# Kurier

Kurier is an agent-ready API testing and debugging platform. Its core goal is
to produce immutable, sanitized API-execution evidence that developers and
coding agents can inspect safely.

This repository currently contains a minimal foundation, not a product
implementation.

## Repository layout

- `apps/web` — React and TypeScript web application
- `services/api` — Go HTTP API
- `services/worker` — Go request-execution worker process
- `services/local-agent` — Go local execution agent and future MCP server
- `infra` — isolated infrastructure spikes and future infrastructure
- `docs` — architecture, decisions, spikes, and planning
- `scripts` — future repository-level development scripts

## Prerequisites

- Node.js 22 or newer and npm
- Go 1.26 or newer
- Docker with Docker Compose

## Setup and development

```sh
npm install
cp .env.example .env
docker compose up -d postgres
npm run dev
```

Run a Go service from another terminal:

```sh
cd services/api
go run .
```

The API defaults to `http://localhost:8080`, the worker health server to
`http://localhost:8081`, and the local agent to `http://localhost:8082`.
Each exposes `GET /healthz`.

## Verification

```sh
npm run format
npm run lint
npm test
npm run build
npm run verify:go
docker compose config --quiet
```

`npm run verify` runs all formatting checks, linting, tests, builds, Go tests,
and Go vet checks in one command.

## SST viability configuration

The repository contains an isolated, non-production SST configuration for the
completed viability experiment. It accepts only the temporary `viability`
stage:

```sh
npm run sst:install
npm run sst:diff
npm run sst:deploy
npm run sst:state:list
npm run sst:remove
```

These commands can create AWS resources and charges. Review
[`docs/spikes/sst-viability.md`](docs/spikes/sst-viability.md), authenticate
with a least-privilege short-lived AWS identity, inspect `npm run sst:diff`,
and obtain approval before deployment. SST is not yet Kurier's adopted
infrastructure framework.

The PostgreSQL credentials in `.env.example` and `compose.yml` are explicitly
for local development only. Never store real credentials in the repository.

## Architecture

See [the architecture overview](docs/architecture/README.md) and
[the SST viability spike](docs/spikes/sst-viability.md). The proposed design
is a planning baseline and may change as the team gathers evidence.
