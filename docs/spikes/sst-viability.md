# SST viability spike

## Status and recommendation

Executed on 2026-08-06. SST passed the non-database viability tests: it
deployed a VPC, ECS/Fargate Go service, least-privilege SQS integration, and
React/Vite static site; repeated the deployment cleanly; detected a deliberate
configuration change; exposed recoverable state; and removed the application
resources.

The recommendation is to run one additional database-specific spike before
permanently adopting SST for Kurier. The evidence supports SST for the tested
compute, queue, IAM, frontend, stage, and teardown workflow. PostgreSQL
networking, migrations, recovery, and production cost were deliberately not
deployed or proven here. The related architecture decision therefore remains
proposed.

## Purpose

This experiment evaluated current SST as a possible replacement for the
earlier AWS CDK plan. It did not implement Kurier product behavior or establish
a production architecture.

The acceptance criteria were:

- TypeScript infrastructure using current SST components;
- an isolated VPC and ECS cluster;
- one small Go service with health and harmless SQS test endpoints;
- queue URL injection and narrowly scoped send permission;
- the existing React/Vite application hosted through CloudFront;
- explicit stage isolation;
- preview, repeat deployment, state inspection, and complete teardown.

## Versions and environment

- SST CLI and npm package: `4.17.1`
- Pulumi AWS provider: `7.40.0`
- Pulumi engine used by SST: `3.215.0`
- Pulumi Docker Build provider: `0.0.14`
- Pulumi Command provider: `1.0.1`
- SST Go resource SDK: `github.com/sst/sst/v3 v3.19.3`
- AWS SDK for Go v2 core: `1.43.4`
- AWS SDK for Go v2 SQS: `1.46.4`
- Stage: `viability`
- Region: `us-east-2`
- Node.js: `26.4.0`
- Go: `1.26.5`

SST 4 is the current stable generation. The `/v3/` path in the Go SDK module is
the current official Go resource-linking API; it does not imply use of obsolete
SST v2/CDK constructs.

## Tested architecture

`sst.config.ts` is restricted to the `viability` stage and defines:

- `sst.aws.Vpc` with two Availability Zones and NAT disabled by omission;
- `sst.aws.Cluster`;
- `sst.aws.Service` using one ARM64 Fargate Spot task with 0.25 vCPU and
  0.5 GB memory;
- a public Application Load Balancer forwarding HTTP port 80 to container port
  8080 and checking `GET /health`;
- `sst.aws.Queue`;
- an `sst.Linkable` exposing only the queue URL and granting only
  `sqs:SendMessage` on that queue;
- `sst.aws.StaticSite` building `apps/web` with Vite and serving it through S3
  and CloudFront;
- stage values passed as `KURIER_STAGE` to Go and the intentionally public
  `VITE_KURIER_STAGE` to the frontend;
- one-week CloudWatch log retention and removal behavior for this disposable
  stage.

The isolated service is under `infra/spikes/sst-viability/service`. It has no
dependency on Kurier's product API.

## Resource inventory

The deployment created the following resource groups:

- one VPC, internet gateway, two public subnets, two private subnets, route
  tables, associations, and security groups;
- one Cloud Map private namespace;
- one ECS cluster, Fargate service, task definition, task/execution roles, and
  CloudWatch log group;
- one public Application Load Balancer, listener, and target group;
- one SQS queue;
- one private site asset bucket, bucket policy/configuration, CloudFront
  distribution, CloudFront function, and key-value store;
- SST state and asset buckets, one ECR asset repository, `/sst/bootstrap`, and
  a stage passphrase parameter.

No NAT Gateway, NAT instance, RDS/Aurora database, Route 53 resource, custom
domain, long-lived application secret, production stage, or second stage was
created.

## Commands and results

Authentication was checked before application deployment:

```sh
aws sts get-caller-identity
aws configure list
aws configure get region
```

The configured profile was `default` and region was `us-east-2`. The initial
session had expired; after interactive `aws login` in another terminal, the AWS
CLI identity check passed.

AWS console-login credentials were not recognized directly by SST in this
environment. Commands that needed SST used an in-process export:

```sh
eval "$(aws configure export-credentials --format env)"
```

The values were never printed, written to the repository, or added to shell
configuration. A normal supported CI role, IAM Identity Center profile, or
credential-process profile should replace this interactive workaround.

Installation and preview:

```sh
npm install --save-dev --save-exact sst@4.17.1
npm run sst:install
npx sst version
npm run sst:diff
docker build --tag kurier-sst-viability:local \
  infra/spikes/sst-viability/service
```

All passed. The initial diff built the frontend and enumerated the planned
resources. It contained no NAT or database.

Deployment and runtime verification:

```sh
npm run sst:deploy
curl --fail <site-url>
curl --fail <service-url>/health
curl --fail-with-body --request POST <service-url>/queue-test
aws sqs receive-message ...
aws sqs delete-message ...
npm run sst:deploy
```

Results:

- the first deployment completed successfully;
- the CloudFront URL returned the Kurier application;
- the deployed JavaScript contained the public `viability` stage label and no
  recognizable AWS access-key or session-token material;
- `GET /health` returned HTTP 200 JSON with service `sst-viability`, stage
  `viability`, and status `ok`;
- `POST /queue-test` returned HTTP 202 with generated correlation and SQS
  message IDs;
- SQS returned the same message ID and a body containing only correlation ID,
  UTC timestamp, `viability` stage, and event type `kurier.sst.viability`;
- the message was deleted and both visible and in-flight queue counts returned
  zero;
- IAM inspection showed the task role's queue statement was exactly
  `sqs:SendMessage` on the viability queue;
- SST also added Session Manager channel permissions to the task role for its
  ECS service support. These were unrelated to queue access and should be
  reviewed before production use;
- the second no-change deployment completed in approximately 15 seconds and
  made no persistent cloud changes. SST recreated and deleted only its local
  static-site build command.

Change detection was tested by temporarily changing CloudWatch log retention
from one week to two weeks:

```sh
npm run sst:diff
```

The diff reported only `retentionInDays = 14` plus the transient site builder.
The source was restored to one week without deployment. A final diff showed no
persistent infrastructure change.

Stage isolation was verified without deploying another stage:

```sh
npx sst diff --stage not-allowed --print-logs
```

Configuration evaluation stopped with the expected message that only
`viability` is permitted.

State and teardown:

```sh
npx sst state list --stage viability
aws s3api list-object-versions --bucket <sst-state-bucket>
npm run sst:remove
```

SST listed only `viability`. Local state lived under ignored `.sst/` paths and
was backed up to a versioned `sst-state-*` bucket. The observed backup included
the current app state, update records, event logs, and snapshots. The
encryption passphrase was held in SSM at
`/sst/passphrase/kurier-sst-spike/viability`.

`sst remove` successfully deleted the application VPC, subnets, internet
gateway, load balancer, ECS resources, queue, site bucket, CloudFront resources,
roles, and log group. Direct AWS lookups then returned no VPC, active ECS
cluster, queue, load balancer, role, or application bucket.

SST intentionally retains shared bootstrap state. In this previously
unbootstrapped account, creation timestamps proved that the diff created both
bootstrap buckets, the ECR repository, and both SSM parameters. After
application removal, those exact resources and their versions were deleted
explicitly. The inactive ECS task definition was also submitted for permanent
deletion. Final AWS inventory returned no spike VPC, cluster, queue, load
balancer, bootstrap bucket, ECR repository, or SSM parameter.

## Problems and resolutions

### Preview has cloud side effects

`sst diff` is a resource preview, but on a new account it automatically created
the SST bootstrap state/asset buckets, ECR repository, bootstrap parameter, and
stage passphrase. This happened before application deployment. The preview did
not create the planned VPC/ECS/SQS/site resources.

This is an important operational limitation: treat the first diff in an
unbootstrapped account as a mutating bootstrap operation and require approval.
The bootstrap resources were inventoried by timestamp and removed after the
spike.

### AWS console-login compatibility

The AWS CLI could use the new console-login credentials, but SST initially
reported no credentials. AWS's current compatibility matrix says login
credentials are not supported by AWS SDK for Go v2, while AWS JavaScript SDK v3
does support them. The exact limitation in SST's credential path was not
isolated further.

An ephemeral `aws configure export-credentials` environment resolved SST CLI
authentication without exposing or persisting credentials. The deployed Go
service did not use local credentials; it correctly used the ECS task role.

### Identity scope

The available interactive identity was the AWS account root. Deployment
proceeded only after this was disclosed in the approval prompt. This is
unacceptable for routine Kurier development or CI. A least-privilege deployment
role with short-lived credentials is required before adopting the workflow.

### Type-checking SST internals

Running TypeScript directly against `.sst/platform/tsconfig.json` exposed an
upstream strictness error in SST's generated platform source. This is not the
supported validation path. `sst install`, repeated `sst diff`, and deployments
all evaluated the actual configuration successfully.

## Cost assessment

The hourly cost was dominated by:

- the public ALB;
- one minimal Fargate Spot task and public IPv4 address;
- Cloud Map namespace;
- small S3, CloudFront, SQS, ECR, and state-storage usage.

SST's published estimate for a continuously running minimal Fargate Spot
service including public IPv4 is about $6/month. A low-traffic ALB is roughly
$16/month before meaningful LCU usage. Together this is approximately
$0.03/hour for this short test, plus negligible request/storage charges. The
deployment existed for substantially less than one hour, so expected direct
cost is well below $0.10, though billing data was not yet available to verify an
observed charge.

The deliberate absence of NAT avoided SST's documented estimate of roughly
$65/month for two managed NAT Gateways. Bootstrap storage would accrue small
ongoing storage charges if retained; it was deleted.

## Resource linking behavior

The custom `QueueAccess` link supplied a generated `SST_RESOURCE_QueueAccess`
environment value to the SST-managed container. The current official Go SDK
read `QueueAccess.url` through `resource.Get`; no queue URL, ARN, account ID,
credential, or secret appeared in source.

The custom link included only an `sqs:SendMessage` permission for the queue ARN.
AWS inspection confirmed that exact statement. The service handled missing or
invalid link configuration by logging a configuration error and returning HTTP
503 from `/queue-test`, rather than crashing or attempting a hardcoded
fallback.

## PostgreSQL assessment

No database was deployed.

### Verified from current SST documentation

`sst.aws.Postgres` creates standard Amazon RDS for PostgreSQL. It best matches
Kurier's currently proposed RDS PostgreSQL architecture. Defaults include a
Single-AZ `db.t4g.micro`, PostgreSQL 17, and gp3 storage with a 20 GB minimum.
SST estimates roughly $14/month in `us-east-1`; Multi-AZ approximately doubles
database cost. RDS Proxy is optional and costs extra.

The component requires a VPC and places the database in private subnets. It
generates a random master password by default or accepts an `sst.Secret`.
Linking exposes host, port, database, username, and password to the linked
runtime. The `dev` option can point links at local PostgreSQL and skip deploying
RDS during `sst dev`.

`sst.aws.Aurora` instead creates Aurora Serverless v2. Current defaults can
scale from zero to four ACUs and pause after five idle minutes on supported
versions. SST documents $0.12 per ACU-hour and approximately $0.01/GB-month
storage. Aurora exposes a Secrets Manager secret ARN and can optionally use the
Data API or RDS Proxy.

Sources:

- [SST Postgres component](https://sst.dev/docs/component/aws/postgres/)
- [SST Aurora component](https://sst.dev/docs/component/aws/aurora)
- [SST VPC component](https://sst.dev/docs/component/aws/vpc/)

### Inference and migration considerations

For Kurier's initial steady API/worker workload, standard RDS PostgreSQL is the
closer architectural match and has more predictable behavior. Aurora
Serverless v2 may reduce idle development cost, but introduces Aurora-specific
operational and pricing behavior and pause/resume latency.

SST does not provide Kurier's schema-migration workflow. The application still
needs a versioned migration tool, one-writer deployment ordering, backup and
restore testing, connection-pool sizing, credential rotation, TLS enforcement,
and a strategy for local-to-hosted data parity. Database credentials are
present in SST state/resource links and require tighter state and runtime-access
review than the harmless queue link tested here.

A separate database spike is necessary. It should compare standard RDS and
Aurora Serverless v2 with representative connections from ECS, test migrations
and rollback, inspect backups/restores and secret rotation, and measure idle and
steady-state cost. It must use a non-root deployment role.

## Developer and security observations

Positive observations:

- components composed cleanly in one TypeScript file;
- the current `Service`, `Queue`, `StaticSite`, and `Linkable` APIs covered the
  required topology without CDK escape hatches;
- Go resource linking and generated IAM were straightforward;
- stage prefixes and the explicit stage guard prevented accidental extra
  environments;
- diffs were readable and repeat deployment was fast;
- teardown handled the application graph successfully.

Risks and limitations:

- first preview bootstrap is mutating and not obvious from `sst diff` output;
- `sst remove` does not remove bootstrap resources or state by default;
- local console-login authentication required a workaround;
- the available root identity was overprivileged;
- ECS services include Session Manager permissions by default;
- CloudFront and ECS teardown each took several minutes;
- static-site rebuilds appear as transient command changes even when deployed
  assets are unchanged;
- PostgreSQL and CI role behavior remain untested.

## Decision

The tested SST path is technically viable, but adoption remains proposed.
Complete the database-specific and least-privilege CI/deployment-role spike
before replacing AWS CDK in Kurier's architecture baseline.
