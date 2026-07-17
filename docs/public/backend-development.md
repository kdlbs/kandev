---
title: "Backend Development"
description: "Change Kandev's Go backend across domain, API, event, persistence, migration, and agent-runtime boundaries."
---

# Backend Development

The Go module in `apps/backend/` builds both the unified `kandev` binary and the `agentctl` helper installed in task environments.

## Run focused loops

Use `make dev` for normal full-stack work: it isolates state, starts Vite, and configures the backend proxy. A raw `make dev-backend` uses the normal application home unless you set an isolated `KANDEV_HOME_DIR`; see [Contributing](contributing.md).

From the repository root:

```bash
make test-backend
make lint-backend
```

For a package-sized loop:

```bash
cd apps/backend
CGO_ENABLED=1 go test -tags fts5 ./internal/task/service/...
CGO_ENABLED=1 go test -tags fts5 ./internal/agent/runtime/lifecycle -run TestName
```

The supported local target enables CGO and SQLite FTS5. CI also has race/coverage and PostgreSQL jobs, so a direct package result is not the complete matrix.

## Follow the owning domain

Backend domains live under `apps/backend/internal/`. A persistence-backed HTTP feature often changes:

1. a domain model and transport DTO;
2. a repository or store contract and implementation;
3. a service or controller method that enforces behavior;
4. an HTTP or WebSocket handler;
5. an event publisher and client broadcaster;
6. construction or route registration in `internal/backendapp/`;
7. tests at each changed boundary.

This is a tracing guide, not a required package template. For example, task code uses `models/`, `dto/`, `repository/`, `service/`, and `handlers/`; other domains use `store/` or add a controller/provider/poller. Follow the nearest domain's errors, transactions, and ownership. Do not call another domain's concrete store when its service or interface owns the rule.

## HTTP, WebSocket, and events

HTTP routes are generally registered under `/api/v1` by domain handlers. WebSocket action constants live in `apps/backend/pkg/websocket/`; dispatch and broadcasting are wired through registered handlers and `internal/gateway/websocket/`.

For a wire change:

- update Go DTOs, JSON compatibility, TypeScript types, domain clients, and WebSocket handlers together;
- validate malformed input, workspace/task reachability, and identity server-side;
- preserve additive compatibility when old and new clients can overlap;
- update [WebSocket API](websocket-api.md) manually when its public contract changes;
- test a reconnecting client that missed the incremental event.

The event bus in `internal/events/` is live fan-out, not durable storage. In-memory delivery is the default; NATS is selected when configured. Write and commit durable state before publishing. Do not publish on rollback. Subscribers must tolerate duplicates, reconnects, and startup recovery from the database.

## Persistence and inline migrations

SQLite is the default. PostgreSQL uses the same domain repositories where supported, with `sqlx.Rebind` and helpers in `internal/db/dialect/`. A package name such as `internal/task/repository/sqlite` does not prove SQLite-only behavior.

Kandev has no central migration directory or external migration runner. Each repository/store creates its fresh schema and applies ordered upgrade steps during backend startup. A schema change must therefore:

1. update the fresh-schema definition;
2. add or update the ordered upgrade/replay path;
3. preserve legacy rows, null/default semantics, indexes, and foreign keys;
4. remain safe when initialization or a compatible step runs again;
5. use the writer connection, dialect helpers, and rebinding;
6. return an error for a failure that must block startup.

`db.MigrateLogger.Apply` exists for tolerant legacy changes and does not abort on every unexpected error. Do not use it for a critical rebuild while assuming startup will fail.

SQLite table rebuilds need explicit copy order, index/trigger recreation, and interruption tests. Before a different binary version boots an existing SQLite database, the persistence provider takes a snapshot and retains recent backups. PostgreSQL does not receive that automatic snapshot; operators remain responsible for `pg_dump`.

Test fresh initialization, replay, and upgrade from representative old rows. Useful patterns include `internal/task/repository/sqlite/schema_replay_test.go` and PostgreSQL tests gated by `KANDEV_TEST_POSTGRES_DSN`. Add PostgreSQL coverage when shared SQL or startup ordering changes.

## Agent runtime and agentctl

`internal/agent/runtime/` defines launch, resume, stop, and observation seams. `internal/agent/runtime/lifecycle/` supplies executor backends and environment preparers. Product executor names in `internal/task/models/` map through `internal/agent/executor/`; startup registration is in `internal/backendapp/agents.go`.

The backend does not directly own every agent subprocess. Backend clients in `internal/agent/runtime/agentctl/` reach the sidecar built from `cmd/agentctl/` and implemented in `internal/agentctl/server/`. The sidecar owns the process/ACP adapter, workspace, Git, files, shell, terminal, ports, and MCP relay inside the environment.

Launch or executor changes usually cross both sides. Test command construction, prepare failure, agentctl delivery/readiness, resume/reconnect, cancellation, process-group cleanup, and at least one relevant container or remote-shaped path. Do not infer remote filesystem, signal, credential, or network behavior from a local process test.

## Background work and recovery

Schedulers, integration pollers, automation rules, workflow reactions, and run queues continue after the initiating request. Return durable IDs and status, pass cancellation-aware contexts, and define timeout, retry, and deduplication behavior. User-visible failure needs persisted state or a queryable run record; a goroutine that only logs is not recoverable.

Keep network and agent work outside database transactions. Startup must be able to distinguish queued, preparing, running, failed, and abandoned work without relying on an event replay.

## Configuration, secrets, and input

`internal/common/config/` owns server configuration and `profiles.yaml` owns prod/dev/E2E runtime-profile environment defaults. User credentials belong in the secrets domain or a provider-specific secret adapter, never plaintext rows or logs.

Validate provider hosts and URLs against SSRF, keep archive extraction within its target, pass commands as argument vectors, and constrain Git refs and filesystem paths. Treat repository files, provider payloads, agent output, and MCP arguments as untrusted.

## Review checklist

- Domain behavior is enforced below the handler and has focused tests.
- Transactions are short; events follow successful durable writes.
- Fresh schema, upgrade/replay, legacy rows, and PostgreSQL impact are covered.
- Cancellation, retry, duplicates, recovery, and cleanup are explicit.
- Runtime changes cover backend and agentctl ownership.
- Secrets and untrusted content do not leak through logs, shell construction, or errors.
- Go/TypeScript wire types and public protocol docs agree.
- Focused tests, `make test-backend`, and `make lint-backend` pass.

Related: [Architecture](architecture.md), [Testing](testing.md), and [Extending Kandev](extending-kandev.md).
