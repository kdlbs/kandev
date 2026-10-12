---
id: "04-transport"
title: "Carry both inputs across the push transport"
status: completed
wave: 2
depends_on: ["01-resolution-primitives"]
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-001
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-002
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-004
acceptance_criteria:
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.1
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.1
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-004.8
system_design:
  - ../../specs/workspaces/system-design/configured-push-target.md
---

# Task 04: Carry both inputs across the push transport

## Summary

Carry the explicit push target and the expected branch from the backend
workspace Git action handler, through the runtime client, to the agentctl HTTP
surface, and add the five result fields to the client-side result struct. Every
layer passes both values through without interpreting them; all destination
logic stays in the operator.

## In scope

- `remote` and `expected_branch` on `GitPushRequest` and
  `GitPushPreflightRequest` in `internal/agentctl/server/api/git.go`, bound and
  forwarded into `PushOptions`.
- The five result fields on `client.GitOperationResult` in
  `internal/agent/runtime/agentctl/git.go`, matching the server struct's JSON
  keys.
- Both inputs on `Client.GitPush` and `Client.GitPushPreflight`, carried in the
  request payloads.
- Both fields on the `worktree.push` payload in
  `internal/agent/handlers/git_handlers.go`, forwarded to the client.
- Keeping the startup preflight caller in
  `internal/agent/runtime/lifecycle/manager_startup.go` passing neither.

## Out of scope

- Any validation, resolution, or refusal decision at any transport layer. The
  HTTP surface performs no destination logic of its own.
- Any web, desktop, or mobile control. The existing push control keeps sending
  neither input.
- Operator behavior, which Tasks 02 and 03 own.

## Acceptance

- A push request carrying `remote` and `expected_branch` reaches
  `GitOperator.Push` with both values intact, and a request carrying neither
  produces a payload and behavior identical to today's.
- Both values are scoped to the repository the request already selects via
  `repo` and affect no other repository in a multi-repository task.
- The client result struct deserializes all five new fields.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/agentctl/server/api/... ./internal/agent/runtime/agentctl/... ./internal/agent/handlers/... ./internal/agent/runtime/lifecycle/...
cd apps/backend && make lint
```

## Files likely touched

- `apps/backend/internal/agentctl/server/api/git.go`
- `apps/backend/internal/agentctl/server/api/git_handlers_test.go`
- `apps/backend/internal/agent/runtime/agentctl/git.go`
- `apps/backend/internal/agent/runtime/agentctl/git_test.go`
- `apps/backend/internal/agent/handlers/git_handlers.go`
- `apps/backend/internal/agent/handlers/git_handlers_test.go`

## Dependencies

Task 01, for `PushOptions` and the server-side result fields.

## Risks

- `Client.GitPush` already takes four positional arguments. Adding two more
  makes the call site unreadable; carry a small options struct instead, and
  update the single existing caller in `internal/agent/handlers/git_handlers.go`.
- The client and server result structs are duplicated by design and drift
  silently. The JSON keys of all five fields must match exactly.

## Parallelism

`parallel-safe`

## Inputs

- The system design's [Components and responsibilities](../../specs/workspaces/system-design/configured-push-target.md)
  and [Data and contracts](../../specs/workspaces/system-design/configured-push-target.md) sections.
- The existing `repo` field convention across the git request structs.
- `GitContributionRequest` as the pattern for an optional string field carried
  end to end.

## Results

Implemented. `GitPushRequest` and `GitPushPreflightRequest` carry `remote` and
`expected_branch` through the agentctl HTTP surface; the runtime client gained a
`PushOptions` struct plus the five result fields; and the backend `worktree.push`
payload forwards both. The startup remote-contribution preflight passes an empty
`PushOptions`, so it is unchanged.

Two call sites shadowed the client package identifier with a local variable named
`client`, which the package's misleading `Package agentctl` doc comment hides. The
handler's local was renamed to `agentClient` to match its siblings, and the
lifecycle call site imports the package under an explicit `agentctlclient` alias.

The new API-surface tests were placed in
`internal/agentctl/server/api/git_push_target_test.go` and
`internal/agent/handlers/git_push_options_test.go` rather than appended to the
existing `git_handlers_test.go` files, which were already close enough to
revive's 800-line file-length limit that appending crossed it.

Verified with `go test -tags fts5 ./internal/agentctl/server/api/...
./internal/agent/runtime/agentctl/ ./internal/agent/handlers/...
./internal/agent/runtime/lifecycle/...` and `make -C apps/backend lint`.
