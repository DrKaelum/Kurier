# ADR 0001: Use SST for Kurier infrastructure

- Status: Accepted
- Date: 2026-08-06

## Context

Kurier needs TypeScript-defined AWS infrastructure for a React static site, Go
services on ECS Fargate, SQS, object storage, and PostgreSQL. AWS CDK was the
earlier baseline. Two SST viability phases tested current SST 4 against the
non-database topology and a private RDS PostgreSQL deployment operated through
a temporary least-privilege deployment role.

## Decision

Use SST 4 with TypeScript as Kurier's infrastructure framework for the planned
AWS architecture. Keep the spike definitions isolated from production
infrastructure and translate their evidence into deliberately designed
environment components rather than treating the spike as production code.

## Evidence

The [SST viability spike](../spikes/sst-viability.md) successfully demonstrated
deployment, Go-to-SQS access, frontend hosting, no-change redeployment, change
detection, state backup, stage isolation, and teardown. Its database follow-up
then demonstrated:

- a private, encrypted Single-AZ RDS PostgreSQL 17.10 instance;
- SST-linked connection metadata and exact-secret runtime permission;
- TLS-verified Go connectivity, an idempotent versioned migration, and a
  complete insert/read/delete transaction;
- deployment, no-change redeployment, diff detection, and teardown using only
  an assumed temporary deployment role;
- evidence-driven tightening and adjustment of that role without
  `AdministratorAccess` or unrestricted IAM administration;
- complete removal of application resources while preserving shared SST
  bootstrap infrastructure and the account-level RDS service-linked role.

## Consequences

- Infrastructure stays in TypeScript with high-level SST components.
- SST resource links provide runtime configuration and generated IAM.
- SST/Pulumi state and bootstrap resources become operational dependencies.
- Deployment roles must include tightly reviewed SST bootstrap and application
  permissions.
- Database migrations, backups, recovery, and secret rotation remain explicit
  application/platform responsibilities.
- The production network design must decide between public-IP Fargate tasks,
  NAT, or VPC endpoints; the spike's temporary public-IP task is not an
  automatic production choice.
- ECS Session Manager permissions remain disabled unless a documented
  operational requirement justifies them.
- First-run `sst diff` may bootstrap an account and is therefore treated as a
  mutating operation requiring review.
