# SST viability spike

## Status and recommendation

Executed on 2026-08-06. SST passed both the original non-database viability
tests and the database/least-privilege follow-up. The combined tests deployed
and observed the planned compute, queue, frontend, private PostgreSQL,
resource-linking, migration, state, repeat-deployment, change-detection, and
teardown paths.

The recommendation is to adopt SST 4 as Kurier's infrastructure framework.
This is evidence for the framework, not approval to promote the spike topology
unchanged into production. Production database recovery, credential rotation,
high availability, and network-egress design remain separate engineering work.
ADR 0001 is accepted based on the combined evidence.

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
- Database stage: `db-viability`
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

## Pre-deployment PostgreSQL assessment

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

A separate database spike was necessary. The follow-up below completed the
connectivity, migration, IAM, repeat-deployment, and teardown portions. Restore
testing and secret rotation remain production-readiness work.

## Database and least-privilege follow-up

### Purpose and identity

The follow-up tested whether SST could manage Kurier's likely PostgreSQL path
without root credentials or `AdministratorAccess`. Both the unprofiled AWS CLI
and the `kurier-admin` profile resolved to the non-root IAM user
`arn:aws:iam::747336059622:user/davian-admin`. That user created and assumed
`kurier-db-viability-deployer`; every SST diff, deployment, inspection, repeat
deployment, and removal ran as:

```text
arn:aws:sts::747336059622:assumed-role/kurier-db-viability-deployer/kurier-db-viability
```

The role session used temporary process environment credentials. No credential
file, permanent access key, password, MFA value, or root credential was read,
written, or used.

### Database comparison and selection

Verified from the current [SST Postgres component
documentation](https://sst.dev/docs/component/aws/postgres), `sst.aws.Postgres`
creates standard RDS PostgreSQL. Its documented baseline is a Single-AZ
`db.t4g.micro` at about `$0.016/hour` plus 20 GB gp3 at about
`$0.115/GB-month`, or roughly `$14/month` before backup and transfer costs.
It requires VPC subnets, can link connection properties to runtimes, and
supports a local `dev` override that avoids deploying RDS.

Verified from the current [SST Aurora component
documentation](https://sst.dev/docs/component/aws/aurora),
`sst.aws.Aurora` creates Aurora Serverless v2. SST documents a default zero-to-
four-ACU range, pause after five idle minutes, roughly 15-second resume latency,
`$0.12/ACU-hour`, and `$0.01/GB-month` storage. Aurora can expose a Data API and
Secrets Manager secret ARN. It is attractive for intermittent development but
adds Aurora-specific scaling, pause, connection, and pricing behavior.

Both candidates use private VPC networking and support encrypted storage,
backups, and point-in-time recovery. Neither requires a NAT Gateway for
database traffic itself. Application tasks still need a route to ECR,
CloudWatch Logs, Secrets Manager, and other required AWS APIs. Neither SST
component supplies Kurier's schema migration workflow.

Standard RDS PostgreSQL was selected because it matches Kurier's proposed
architecture, is sufficient for the capstone vertical slice, is simpler to
reason about, and has a lower predictable continuously-running baseline than
an unpaused Aurora database. Aurora remains a future commercial option if
measured scale, availability, or intermittent-stage economics justify its
additional behavior.

AWS documents that RDS billing is per second with a ten-minute minimum after a
billable state change, and that new-account free-tier or credits may apply.
Eligibility was not inferred from the account and no billing claim is made.
See [AWS RDS for PostgreSQL pricing](https://aws.amazon.com/rds/postgresql/pricing/).

### Exact deployed configuration

- Application: `kurier-sst-db-spike`
- Stage: `db-viability`
- Region: `us-east-2`
- SST: `4.17.1`
- Pulumi AWS provider: `7.40.0`
- PostgreSQL engine: `17.10`
- Instance: Single-AZ `db.t4g.micro`
- Storage: 20 GB gp3, encrypted, with storage autoscaling disabled
- Database name: `kurier_db_viability`
- Public access: disabled
- Database subnets: two private subnets with only the VPC-local route
- Backup retention: one day
- Final snapshot: skipped for the disposable stage
- Automated backups on deletion: not retained
- Deletion protection and Performance Insights: disabled
- RDS Proxy: disabled
- `rds.force_ssl`: `1`

The spike used a raw Pulumi VPC graph inside the SST configuration: one VPC,
two public task/ALB subnets, two private database subnets, one Internet Gateway,
public and private route tables, and dedicated ALB, task, and database security
groups. It created no NAT Gateway, VPC endpoint, Route 53 resource, custom
domain, Aurora resource, public database address, or second database.

The temporary ARM64 Fargate Spot task used 0.25 vCPU and 0.5 GB memory. It was
assigned a public IPv4 address only because the approved no-NAT/no-interface-
endpoint topology still needed ECR, Secrets Manager, CloudWatch Logs, and AWS
API access. Its public subnets had an active
`0.0.0.0/0 -> Internet Gateway` route. Task ingress allowed TCP 8080 only from
the ALB security group. Task egress allowed:

- TCP 443 to the internet for required AWS HTTPS endpoints and image pulls;
- TCP and UDP 53 to the VPC resolver at `10.0.0.2/32`;
- TCP 5432 to the VPC CIDR for the private database;
- TCP 80 to `169.254.170.2/32` for the ECS task credential endpoint.

The database security group accepted TCP 5432 only from the task security
group and had no egress rule. AWS inspection confirmed the database was not
publicly accessible and its private subnet route table had no Internet Gateway
or NAT route.

### Secure linking and Go behavior

The isolated service lives under
`infra/spikes/sst-db-viability/service`. SST's generated database secret stayed
in Secrets Manager. A custom `DatabaseAccess` link exposed only database name,
host, port, username, and the generated secret ARN. The linked environment did
not contain the password. The task role had one inline statement:
`secretsmanager:GetSecretValue` on the exact generated secret ARN. The ECS
execution role had only AWS's standard `AmazonECSTaskExecutionRolePolicy`.

The service fetched and decoded the secret at runtime, built a PostgreSQL
connection without printing it, required TLS 1.2 or newer with
`sslmode=verify-full`, and validated the RDS hostname against AWS's official CA
bundle. Missing links, missing secrets, malformed values, transport failures,
and database failures produce fixed diagnostic categories or redacted HTTP
responses. Credentials, connection strings, ARNs, AWS tokens, and internal
errors are not logged or returned.

The service exposed:

- `GET /service-health` for process/ALB health;
- `GET /health` with distinct service and database status;
- `GET /migration-status`;
- `POST /migrate`;
- `POST /database-test`.

Migration `0001_create_spike_records.sql` created a spike-only table and a
version-tracking table. The migration ran transactionally at startup.
`POST /migrate` twice returned `{"status":"current"}`, proving idempotency.
`POST /database-test` generated harmless correlation metadata, inserted it,
retrieved it, compared it, and deleted it in one observed operation.

Observed runtime results:

```text
GET  /service-health   200, service ok
GET  /health           200, service ok and database ok
GET  /migration-status 200, currentVersion 1
POST /migrate          200, current
POST /migrate          200, current
POST /database-test    200, inserted-retrieved-deleted
```

### Deployment-role design

The committed policy template is
`infra/spikes/sst-db-viability/iam/deployment-policy.json`; account IDs remain
placeholders. Its trust policy permits assumption only by the non-root
`davian-admin` user. The deployment policy does not include
`AdministratorAccess`, unrestricted IAM administration, user/group/access-key
management, or permission to pass arbitrary roles.

Write permissions are limited where AWS and generated names permit:

- exact shared SST state and asset buckets and their objects;
- `/sst/bootstrap` and the exact app/stage passphrase parameter;
- the shared `sst-asset` ECR repository;
- stage-prefixed RDS, Secrets Manager, ECS, log-group, and IAM role ARNs;
- `iam:PassRole` only for stage-prefixed roles and only to
  `ecs-tasks.amazonaws.com`;
- the RDS service-linked role only through `iam:CreateServiceLinkedRole` with
  `iam:AWSServiceName = rds.amazonaws.com`.

Some read and relationship APIs necessarily remained `Resource: "*"`,
including `Describe*`, `GetAuthorizationToken`, random-password generation,
ECS task-definition registration, EC2 relationship/tag operations,
load-balancer graph operations, and Application Auto Scaling graph operations.
Their service APIs either do not support useful creation-time resource scoping
or the final ARN does not exist when SST calls them. The stage guard, SST
default tags, tightly named dependent resources, lack of unrelated delete APIs,
and short role lifetime reduce but do not eliminate this risk. A production
role should add organization-level tag conditions or separate bootstrap and
application roles where AWS reliably supports them.

The policy was validated with IAM Access Analyzer before use. Access-denied
events drove only evidence-backed additions or scope corrections:

- exact generated role prefixes and RDS parameter/subnet-group ARNs;
- Secrets Manager `GetResourcePolicy`;
- Application Auto Scaling create/delete/tag actions and
  `ListTagsForResource`;
- stage-log-group `logs:FilterLogEvents` for redacted diagnosis;
- EC2 child-resource creation in the spike VPC;
- `iam:ListInstanceProfilesForRole` on only the stage-generated role prefixes,
  required by Pulumi's IAM role deleter.

The final IAM policy simulation allowed
`ListInstanceProfilesForRole` on the exact generated task role. A deliberate
account-wide `iam:ListRoles` call under the deployment role was denied, which
confirmed it could not enumerate or administer unrelated IAM roles.

SST's default ECS task role includes Session Manager channel actions when ECS
Exec is enabled. Kurier did not require interactive container access for this
spike. The service explicitly set `enableExecuteCommand = false`, removed the
default task-role statements, and added only exact-secret read access.

### Commands and verification

The controlled workflow used:

```sh
aws sts get-caller-identity
aws accessanalyzer validate-policy ...
aws sts assume-role ...
npm run sst:install:db
BUILDX_BUILDER=kurier-sst-builder npm run sst:diff:db -- --print-logs
BUILDX_BUILDER=kurier-sst-builder npm run sst:deploy:db -- --print-logs
curl --fail-with-body <service-url>/health
curl --fail-with-body --request POST <service-url>/migrate
curl --fail-with-body --request POST <service-url>/database-test
npm run sst:state:list:db
npm run sst:remove:db -- --print-logs
```

Installation/configuration evaluation and preview passed under the assumed
role. The deployment completed successfully. An unchanged second deployment
reported all 44 managed cloud resources skipped. A temporary one-week to
two-week log-retention edit produced exactly one
`retentionInDays = 14` diff; the edit was restored and a final diff reported no
changes.

SST state used the pre-existing, versioned `sst-state-moshrdrrbeba` S3 bucket,
with local generated state under ignored `.sst/`. `sst state list` showed only
`db-viability` for this application before removal and `db-viability (not
deployed)` afterward. The shared state bucket, asset bucket, `sst-asset` ECR
repository, and `/sst/bootstrap` parameter were preserved.

### Problems and resolutions

- SST's high-level VPC component brought Cloud Map/private-DNS behavior that
  was unnecessary for this database-only test. A small raw Pulumi VPC graph
  kept the test explicit and avoided Route 53 and NAT.
- With no NAT or VPC endpoints, private-subnet Fargate could not reach ECR and
  required AWS APIs. After explicit approval, only the disposable task moved
  to public subnets with `assignPublicIp: true`; RDS remained private.
- Local BuildKit DNS failed while fetching build dependencies. A temporary
  host-network Buildx builder isolated the workaround without changing the
  default builder.
- The scratch container initially contained the RDS CA bundle but no general
  web PKI roots, causing a redacted `secret-fetch-transport` failure against
  Secrets Manager. Adding the maintained Mozilla CA bundle fixed the HTTPS
  path while retaining the separate RDS CA bundle.
- The first removal attempt exposed the missing
  `iam:ListInstanceProfilesForRole` permission after AWS's normal ECS drain
  delay. That single action was added to the existing stage-role ARN scope,
  validated, and the same assumed role completed teardown.

### Teardown and residual audit

SST removal reported zero managed resources. Independent AWS queries found no
spike RDS instance or cluster, secret, ECS cluster or task, load balancer,
tagged VPC, subnet, security group, ENI, Internet Gateway, NAT Gateway, or
CloudWatch log group. The task and its public IPv4 association disappeared
with ECS deletion.

One already-created automated RDS backup snapshot remained temporarily visible
immediately after instance deletion even though
`deleteAutomatedBackups = true`; the retained-automated-backup API returned
`DBInstanceAutomatedBackupNotFound`. AWS documents that automated backups are
deleted when they are not retained. This was not a manual or final snapshot
and cannot be deleted through the manual snapshot API. A later recheck returned
an empty snapshot inventory, confirming the eventual cleanup completed.

The shared SST bootstrap buckets, ECR repository, SSM bootstrap parameter, and
their uncertain shared objects were deliberately not removed. The account-level
`AWSServiceRoleForRDS` service-linked role was also preserved. After cloud
audit, the temporary `kurier-db-viability-deployer` role and its inline policy
were deleted using the non-root administrator identity.

### Cost and limitations

The standard RDS baseline is approximately `$0.016/hour` plus prorated 20 GB
gp3 storage (`$2.30/month` at SST's documented us-east-1 estimate). The public
ALB, 0.25-vCPU/0.5-GB Fargate Spot task, public IPv4, logs, secrets, ECR, and S3
added short-lived usage. The experiment ran for hours, not a month; expected
direct cost is low single-digit dollars or less, but billing data was not yet
available and account credits were not assumed.

For a capstone vertical slice, Single-AZ RDS is economical and operationally
simple. A commercial deployment needs an explicit Multi-AZ and recovery
decision, longer retention, restore testing, credential rotation, application
users rather than the master user, connection-pool sizing, migration
serialization, and monitoring. The temporary public-IP task design is not a
production recommendation; production should compare NAT, VPC endpoints,
public tasks behind an ALB, and their cost/security tradeoffs.

## Developer and security observations

Positive observations:

- components composed cleanly in one TypeScript file;
- the current `Service`, `Queue`, `StaticSite`, and `Linkable` APIs covered the
  required topology without CDK escape hatches;
- Go resource linking and generated IAM were straightforward;
- stage prefixes and the explicit stage guard prevented accidental extra
  environments;
- diffs were readable and repeat deployment was fast;
- teardown handled both application graphs successfully;
- private RDS, secure secret delivery, Go migrations, and least-privilege role
  operation were observable through standard AWS tooling.

Risks and limitations:

- first preview bootstrap is mutating and not obvious from `sst diff` output;
- `sst remove` does not remove bootstrap resources or state by default;
- local console-login authentication required a workaround;
- the first phase used a root identity; the follow-up corrected this and proved
  a non-root assumed deployment role;
- ECS services can include Session Manager permissions by default, so Kurier
  must keep ECS Exec opt-in;
- CloudFront, ECS, and RDS teardown each took several minutes;
- static-site rebuilds appear as transient command changes even when deployed
  assets are unchanged;
- the tested deployment policy still needs production hardening around AWS APIs
  that require wildcard resources;
- database restore, rotation, Multi-AZ behavior, and production CI federation
  remain untested.

## Decision

Adopt SST 4 as Kurier's infrastructure framework. The combined evidence covers
the required compute, queue, static frontend, private PostgreSQL, secure
linking, non-root deployment role, repeat deployment, diff, state, and teardown
paths. ADR 0001 records the accepted choice.

The next infrastructure task should convert the experimental graph into a
small, environment-oriented foundation and design GitHub Actions OIDC
federation. It must not copy the spike's public-IP task, master-database-user,
or broad creation-time permissions into production without explicit review.
