---
title: "Backend Development"
description: "Work in Kandev's Go backend using its handler, service, repository, event, and runtime patterns."
---

# Backend Development

The backend is a Go module rooted at `apps/backend`. It builds the unified `kandev` binary and the `agentctl` helper used inside local, container, and remote task environments.

## Run and test the backend

From the repository root:

```bash
make dev-backend
make test-backend
make lint-backend
```

For a package-sized loop:

```bash
cd apps/backend
go test -tags fts5 ./internal/task/...
go test -tags fts5 ./internal/mcp/server -run TestName
```

The `fts5` build tag and CGO settings used by the Makefile matter for SQLite search behavior. Reproduce CI through the root targets before concluding that a direct `go test` result is sufficient.

## Follow domain boundaries

A normal backend feature flows through:

1. domain model or request DTO;
2. repository interface and implementation when persistence changes;
3. service method that enforces behavior;
4. HTTP or WebSocket handler;
5. event publication and gateway notification when clients need real-time state;
6. startup wiring in `internal/backendapp/`;
7. unit/integration tests at each changed boundary.

Use the nearest domain's naming and error conventions. Avoid reaching directly into another domain's SQLite repository when a service or interface exists.

## HTTP and WebSocket APIs

HTTP routes generally live with a domain's handlers/controllers under `/api/v1`. Real-time task and agent behavior also uses typed actions from `apps/backend/pkg/websocket` and gateway broadcasters under `internal/gateway/websocket`.

When adding or changing a wire field:

- update the Go DTO and web TypeScript type/client together;
- preserve JSON compatibility unless the change is intentionally breaking;
- test malformed input and authorization/workspace scope;
- publish an event only after persistence succeeds;
- consider a reconnecting client that missed the event.

The public WebSocket protocol reference is generated from maintained behavior in [WebSocket API](websocket-api.md).

## Persistence and migrations

Domain repositories under `internal/*/repository` and `internal/db` own storage. SQLite is the primary local path; PostgreSQL compatibility uses dialect helpers and CI coverage.

- Make migrations forward-safe and deterministic.
- Add repository tests for both new writes and old-row reads.
- Keep transaction scope small.
- Never log secret values.
- Consider startup recovery for statuses that represent in-flight work.

## Agent runtime boundary

The backend does not run every agent command directly. `internal/agent/runtime/lifecycle` coordinates execution, while agentctl owns the process in the task environment. Control DTOs and streams cross that boundary.

Changes to agent launch, MCP injection, terminals, Git operations, or remote behavior often require tests on both sides. Check local, worktree, Docker, and at least one remote-shaped path rather than assuming local process behavior generalizes.

## Events and background work

Schedulers, integration watches, automations, workflow reactions, and run queues can execute after the initiating request returns. Make handlers return durable IDs/status, use cancellation-aware contexts, and define deduplication or idempotency where external events can repeat.

Background failures must be observable in persisted status or logs. A goroutine that only logs an error is usually insufficient for user-facing work.

## Configuration and secrets

`internal/common/config` loads server configuration. User-managed credentials go through the secrets domain or provider-specific secret adapter rather than plaintext config rows or logs.

Validate URLs, filesystem paths, shell input, and provider payloads at the boundary. Pay particular attention to prepare scripts, Git refs, archive extraction, MCP destinations, and remote command construction.

## Review checklist

- Domain rule lives in a service, not only a handler.
- Repository behavior and migration are tested.
- Events follow durable writes.
- Cancellation, timeout, retry, and duplicate-event behavior are explicit.
- No secrets or agent content leak into logs unexpectedly.
- Web types and public docs match the API.
- Relevant focused tests plus `make test-backend` and `make lint-backend` pass.

Related: [Architecture](architecture.md), [Testing](testing.md), and [Extending Kandev](extending-kandev.md).
