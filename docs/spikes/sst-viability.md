# SST viability spike

## Question

Can current SST support Kurier's proposed AWS architecture more simply and
reliably than the earlier AWS CDK plan?

This spike is evaluative. SST is not permanently adopted until the results are
reviewed.

## Scope

Build the smallest disposable TypeScript definition that can demonstrate:

- an ECS Fargate API and worker;
- RDS PostgreSQL connectivity and secret handling;
- an SQS queue between the API and worker;
- an S3 evidence bucket with appropriate retention and access controls;
- CloudFront delivery for the web application;
- per-environment configuration, local developer ergonomics, and CI suitability;
- clean teardown with no orphaned billable resources.

The spike must use a sandbox AWS environment, explicit cost limits, and
non-sensitive sample data. It must not be mixed into product feature work.

## Evaluation criteria

- Required AWS services are supported without fragile escape hatches.
- IAM, networking, secrets, deployment previews, and rollback behavior are
  understandable and reviewable.
- CI can deploy without interactive credentials.
- Local workflows and generated state are clear to contributors.
- Operational ownership, upgrade risk, and migration options are acceptable.

## Deliverable

Record the tested SST version, sample topology, deployment and teardown results,
cost observations, limitations, and a recommendation: adopt SST, retain AWS CDK,
or investigate another option. Remove all spike resources after collecting the
results.
