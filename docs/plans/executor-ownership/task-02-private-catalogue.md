---
id: "02-private-catalogue"
title: "Protect configuration and catalogue"
status: done
wave: 2
depends_on:
  - "01-ownership-policy"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-OWNERSHIP-001
  - REQ-EXECUTORS-OWNERSHIP-002
  - REQ-EXECUTORS-OWNERSHIP-003
acceptance_criteria:
  - AC-EXECUTORS-OWNERSHIP-001.2
  - AC-EXECUTORS-OWNERSHIP-002.1
  - AC-EXECUTORS-OWNERSHIP-002.2
  - AC-EXECUTORS-OWNERSHIP-002.3
  - AC-EXECUTORS-OWNERSHIP-003.1
  - AC-EXECUTORS-OWNERSHIP-003.2
  - AC-EXECUTORS-OWNERSHIP-003.3
system_design:
  - ../../specs/executors/system-design/per-user-ownership.md
---

# Task 02: Protect configuration and catalogue

## Summary

Make profile/executor configuration and catalogue data private across every exposed transport, including live ownership changes.

## In scope

- Guard service CRUD, connection tests, build/prepare helpers, clone and secret-reference operations; preserve provider-specific config gates.
- Extend HTTP/WS/MCP and typed settings operations with presence-aware ownership inputs and authorized projections.
- Filter boot/nested catalogue reads and recipient events; invalidate/refetch on transfer without private payloads and fence stale responses.

## Out of scope

Other work orders and the package exclusions. Do not add UI before the underlying authorization contract exists.

## Acceptance

- A/B/admin transport tests prove foreign configuration is unavailable while shared resources retain existing behavior.
- Ownership changes emit only committed, recipient-authorized data; former owners lose cached entries after transfer.
- Owner-only edits work without bypassing protected provider config permissions, and omitted fields preserve ownership.

## Verification

Run from repository root. On a fresh worktree, run
`(cd apps && pnpm install --frozen-lockfile)` before the first pnpm command.
Add the planned tests with TDD; test names must contain `ExecutorOwnership`
where the Go commands filter by that pattern. Ensure new focused web tests are
added to the exact Vitest command before recording completion.

```bash
(cd apps/backend && go test ./internal/task/service ./internal/task/handlers ./internal/mcp/handlers ./internal/backendapp ./internal/gateway/websocket -run 'ExecutorOwnership|ExecutorProfile.*Script' -count=1)
(cd apps/web && pnpm exec vitest run lib/ws/handlers/executor-ownership.test.ts)
(cd apps/web && pnpm run typecheck)
```

Also run `bash scripts/lint-go-changed` with the explicit changed Go files,
then `git diff --check`. Run the ownership conformance scenario against both
SQLite and the repository-configured PostgreSQL test database; a skipped
PostgreSQL test is not evidence of parity. Record the database prerequisite
and exact command/output when that scenario is available.

## Files likely touched

- `apps/backend/internal/task/service/service_resources.go`
- `apps/backend/internal/task/service/executor_provider_profiles.go`
- `apps/backend/internal/task/service/service_events.go`
- `apps/backend/internal/task/handlers/executor_handlers.go`
- `apps/backend/internal/task/handlers/executor_profile_handlers.go`
- `apps/backend/internal/mcp/handlers/config_executor_handlers.go`
- `apps/backend/internal/backendapp/settings_*.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/gateway/websocket/task_notifications.go`
- `apps/web/lib/ws/handlers/executor-profiles.ts`
- `apps/web/lib/ws/handlers/executors.ts`
- `apps/web/lib/ws/handlers/executor-ownership.test.ts (new)`

## Dependencies

Task 01; execute sequentially after its acceptance and targeted checks pass.

## Risks

Filtering only GET endpoints misses boot payloads, settings snapshots, and events. Never repurpose internal inventory readers as user-filtered readers.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/per-user-ownership.md).
- [System design](../../specs/executors/system-design/per-user-ownership.md).
- [Ownership ADR](../../decisions/2026-10-15-private-executor-admission.md).
- [Plan evidence and existing code patterns](plan.md#technical-approach).
- Read scoped AGENTS.md, `/tdd`, and applicable `/mobile-parity` and `/e2e`
  instructions before implementation.

## Results

Implemented authorized catalogues/getters/mutations across HTTP, WebSocket, MCP, settings descriptors, and boot projections. Configuration notifications check each recipient independently; payload-free invalidation evicts lost access and fences stale catalogue responses. Logout, identity changes, role changes, and account status changes clear client configuration. SSH reachability and SSH/Kubernetes inventory use the same configuration policy.

HTTP/MCP/notification regressions, 104 focused frontend tests, and the API/boot/live-eviction browser scenarios pass. Changed-package Go lint is recorded in the plan.

PR remediation: catalogue invalidation clears selectable data atomically while the identity-scoped editor remains mounted, hidden, and inert until access is revalidated. A confirmed removal still discards the form. Regenerated the settings contract snapshot and added the required global invalidation justification.
