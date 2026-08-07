# Proposed architecture

Kurier's architecture is a planning baseline. It should evolve when spikes,
security review, or vertical-slice delivery provide better evidence.

## Components

- A React and TypeScript web application presents request authoring, execution
  status, and sanitized evidence.
- A Go API handles authenticated application requests and persists durable
  metadata in PostgreSQL.
- A Go execution worker consumes queued jobs, performs remote API requests, and
  writes immutable, deliberately redacted evidence.
- A Go local agent will execute requests that require access to a developer's
  machine or private network. A future MCP interface will expose only
  deliberately sanitized evidence to coding agents.

The proposed production topology uses CloudFront for web delivery, ECS Fargate
for the API and worker, RDS PostgreSQL for persistence, SQS for execution jobs,
and S3 for larger immutable evidence artifacts. SST 4 with TypeScript is the
selected infrastructure framework following the compute/frontend and database
viability spikes. GitHub Actions is the proposed CI/CD entry point.

## Trust and data boundaries

Request credentials are sensitive by default. Authorization headers, cookies,
credential headers, and user-designated secrets must be redacted before logs,
persistence, evidence artifacts, or MCP responses are produced. Evidence should
be immutable and traceable to an execution without retaining unneeded secrets.

The local agent is a separate trust boundary: it should bind locally by
default, require explicit authorization, and reveal the minimum information
needed by the hosted platform.

## Current state

Only local process foundations and isolated, removable infrastructure-spike
definitions exist today; no Kurier AWS stage remains deployed. PostgreSQL is
available through Docker Compose, while an isolated Fargate spike proved
private RDS PostgreSQL connectivity, secure credential delivery, TLS, and a
versioned migration. Product services are not connected to PostgreSQL yet.
Queueing, evidence storage, authentication, and redaction remain future work.
