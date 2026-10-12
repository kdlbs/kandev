---
id: "03-refresh-delivery"
title: "Deliver and recover scoped snapshots"
status: done
wave: 3
depends_on: ["02-ordered-enrichment"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.20
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.21
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.25
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.26
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.27
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.28
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.32
system_design:
  - ../../specs/platform/system-design/workspace-git-status.md
---

# Task 03: Deliver and recover scoped snapshots

## Summary

Carry accepted snapshot quality and failures through runtime and gateway delivery. Return usable correlated refresh responses and preserve canonical live-source authority.

## In scope

- Keep the ordinary complete source probe at two seconds. Project per-repository failures, summary-only persistence, and transport-level unavailable state.
- Add `fresh`, `recover`, and `replay` modes to `session.git.refresh`, with correlated snapshot/state responses and finite budgets from the design.
- Preserve environment/workspace eligibility, sibling lookup, authorization, and requested-session routing. Capture/revalidate execution and stream generation across asynchronous work.
- Preserve detailed read consumers through `details=wait`: completion/archive snapshots, launch base capture, and server-side Review/log/cumulative-diff fallbacks. Cover pending, superseded, and failed details.
- Reconcile compact snapshot metadata, hashing, task summaries, and completion capture consumers with pending/unknown details. Never persist provisional zeros as confirmed totals.
- Add actual handler-to-client response recovery coverage with notifications deliberately dropped, plus mixed healthy/failed repositories and replaced-source cases.

## Out of scope

UI markup and localization. No database schema migration or persisted live-diff storage.

## Acceptance

- A dropped initial or completed notification can recover through the correlated response/replay without another worktree mutation.
- Failed repositories remain named beside healthy ones. Failed eligible live sources never substitute an old DB snapshot as fresh.
- Shared-environment siblings accept the correct scope. Late source/workspace/session replacement responses are rejected, and compact rows cannot establish clean membership.

## Verification

Run from repository root. Use `/tdd` and run each new regression red before implementation.
If `apps/node_modules` is absent, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -race -tags fts5 ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/gateway/websocket ./internal/orchestrator -count=1)
(cd apps/backend && go test ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agent/handlers -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator/executor -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/git_status_sources.go`
- `apps/backend/internal/backendapp/helpers_git_status_test.go`
- `apps/backend/internal/backendapp/helpers_git_status_materialized_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/streams.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/agent/runtime/agentctl/git.go`
- `apps/backend/internal/agentctl/server/api/git.go`
- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/internal/gateway/websocket/client.go`
- `apps/backend/internal/gateway/websocket/hub.go`
- `apps/backend/internal/gateway/websocket/client_git_refresh_test.go`
- `apps/backend/internal/backendapp/adapters.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/agent/handlers/review_change_source.go`
- `apps/backend/internal/agent/handlers/git_handlers.go`
- `apps/backend/internal/orchestrator/event_handlers_git.go`
- `docs/specs/tasks/system-design/environment-owned-git-status.md`

## Dependencies

Complete Task 02 first. Read its Results before changing the shared contract.

## Risks

- Long recovery must not block socket reading or run outside client teardown ownership.
- Source epochs from separate siblings need validated ownership, not string ordering.
- Read scope can change during a successful HTTP call. Revalidate after the response, before publication.

## Parallelism

`sequential`. This work order does not authorize delegation.

## Inputs

- [Requirement](../../specs/platform/requirements/workspace-git-status.md).
- [System design](../../specs/platform/system-design/workspace-git-status.md).
- [Plan](plan.md), accepted ADR, scoped `AGENTS.md`, and existing tests beside owned code.

## Results

The initial package check exposed two API projection gaps. Cancellation was being reduced to a generic unavailable status, and a failed repository lookup had no repository-specific code. Cancellation now returns `status_canceled`; tracker lookup errors return a local `repository_unavailable` code without exposing raw filesystem details. Healthy repository entries remain successful beside a failed entry.

- `(cd apps/backend && go test -race -tags fts5 ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/gateway/websocket ./internal/orchestrator -count=1)`: passed.
- `(cd apps/backend && go test ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agent/handlers -count=1)`: passed.
- `(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator/executor -count=1)`: passed.
- Focused cancellation and invalid-repository API regressions: passed.
- `git diff --check`: pending final integrated verification.

Work order 03 is done. Work order 04 is in progress.
