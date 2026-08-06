# ADR 0001: SST as the infrastructure candidate

- Status: Proposed
- Date: 2026-08-06

## Context

Kurier needs TypeScript-defined AWS infrastructure for a React static site, Go
services on ECS Fargate, SQS, object storage, and PostgreSQL. AWS CDK was the
earlier baseline. The SST viability spike tested current SST 4 against the
non-database topology.

## Proposed decision

Use SST as the leading infrastructure candidate for the tested VPC, ECS,
Fargate, SQS, IAM-linking, and static-site concerns. Do not mark this decision
accepted or replace the AWS CDK baseline until a database-specific spike and a
least-privilege CI/deployment-role design succeed.

## Evidence

The [SST viability spike](../spikes/sst-viability.md) successfully demonstrated
deployment, Go-to-SQS access, frontend hosting, no-change redeployment, change
detection, state backup, stage isolation, and teardown.

Open concerns are first-run bootstrap side effects, bootstrap cleanup, local
AWS console-login compatibility, default ECS Session Manager permissions, and
untested PostgreSQL operations.

## Consequences if accepted

- Infrastructure stays in TypeScript with high-level SST components.
- SST resource links provide runtime configuration and generated IAM.
- SST/Pulumi state and bootstrap resources become operational dependencies.
- Deployment roles must include tightly reviewed SST bootstrap and application
  permissions.
- Database migrations, backups, recovery, and secret rotation remain explicit
  application/platform responsibilities.
