---
id: "02-permission-audit-claim"
title: "Permission audit claim"
status: completed
wave: 2
depends_on: ["01-live-permission-contract"]
plan: "plan.md"
spec: "../../specs/agents/external-permission-resolution.md"
---

# Task 02: Permission audit claim

## Acceptance

- Permission messages persist `request_id` and support an atomic first-writer claim bound to task
  session, request ID, pending ID, and claim ID on both SQLite and Postgres dialect paths.
- Audit claims/finalization retain actor kind/user, source, exact option ID/kind, timestamps, and
  honest result without raw action details, credentials, environment, PAT values, or token IDs.
- Competing/in-progress, terminal replay, wrong identity, and finalization-by-wrong-claim are
  distinguishable and publish the existing message update only after a successful write.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/task/repository/sqlite ./internal/task/service ./internal/backendapp
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/message.go`
- `apps/backend/internal/task/repository/sqlite/message_test.go`
- `apps/backend/internal/task/service/service_messages.go`
- `apps/backend/internal/task/service/service_messages_test.go`
- `apps/backend/internal/backendapp/adapters.go`
- `apps/backend/internal/backendapp/adapters_test.go`

## Dependencies

Task 01.

## Parallelism

Sequential. The authorized service must rely on this claim as its pre-delivery replay barrier.

## Inputs

- Spec: `Permission audit projection`, failure/persistence guarantees, and concurrency/replay
  scenarios.
- Existing patterns: `MessageRepository.UpdateMessage`, dialect-aware JSON helpers, message update
  events, and permission status metadata.

## Risks

- JSON predicates must behave identically for absent/null metadata on SQLite and Postgres.
- A finalization failure must preserve the original durable dispatch claim instead of reopening it.

## Output contract

Report CAS outcomes and SQL dialect coverage, audit/redaction fields, exact commands/results, files
changed, blockers/risks, then update this task and the plan checkbox/results.

## Results

- Added typed actor/source/result audit models with no fields for action details, credentials,
  environment, PAT values, or token IDs.
- SQLite and PostgreSQL use a single conditional JSON update as the first-writer claim; exact-claim
  finalization is another conditional update and never reopens a dispatching claim.
- Repository tests cover first claim, concurrent competition, in-progress, terminal replay, wrong
  identities, wrong-claim finalization, accepted finalization, redaction shape, and PostgreSQL JSONB
  expression selection. Empty metadata is guarded before PostgreSQL `jsonb` casts, with an
  always-on expression regression and an environment-gated real PostgreSQL behavior test.
  Missing audit rows return the stable domain representation `(nil, nil)`. Task-service tests
  confirm only successful writes publish `message.updated`.
- `request_id` now flows from agentctl events into the durable permission message metadata.
- Passed: full `./internal/task/repository/sqlite`, full `./internal/backendapp`, and targeted
  `./internal/task/service -run TestPermissionResolutionServicePublishesOnlySuccessfulWrites`.
- The prescribed combined command also exposed pre-existing environment-only failures in unrelated
  local-directory tests: this container's filesystem root is owned by `nobody`, so Kandev's trusted
  parent-chain check correctly rejects every absolute temp path. The permission tests themselves
  pass; no production path-security behavior was weakened.
