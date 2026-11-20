---
id: "09-maintainer-review-scope-and-safety"
title: "Legacy-routing scope, switch authority, targeted parked-run release, and replay authorship"
status: done
wave: 8
depends_on:
  - "08-seat-write-integrity"
plan: "plan.md"
requirements:
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-002
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-005
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-006
acceptance_criteria:
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-002.6
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-002.7
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.2
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-006.8
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-006.11
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-006.12
system_design:
  - ../../specs/office/system-design/review-author-provider-exclusion.md
---

# Task 09: Legacy-routing scope, switch authority, targeted parked-run release, and replay authorship

## Summary

Address maintainer review of the PR. The setting is declared to cover only
agents on workspace provider routing, with a setup warning for each seat-pool
agent bound to its own execution profile, until the Office routing handoff
carries it into the shared resolver. Only users who may manage the workspace
can change the switches. Turning a switch off re-queues only the reviews it
parked, never a session-ceiling hold. A work run launched by the ceiling's
replay records its provider. A committed save is reported as saved even when
the read-back fails.

## Scope

1. `agent_bound_profile` setup warning in the evaluator, the import advisor's
   sync text, and the web warning list, with copy in every locale; the
   switch description says profile-bound agents are not covered.
2. `requiresOfficeWorkspaceManage` covers `PUT .../routing/author-exclusion`;
   the handler refuses agent callers.
3. `ReleaseAuthorExclusionParkedRuns` (repository and routing provider)
   replaces the workspace-wide release for the switch endpoint.
4. `SettleCeilingDeferredRun` copies the deferred attempt's route onto the
   run.
5. The PUT handler answers 200 after the write; null steps make the client
   reload.
6. The [Office handoff design](../../specs/agents/system-design/dynamic-agent-routing-01.md#use-in-office)
   carries the exclusion into the shared resolver.

## Exclusions

Enforcing the exclusion for profile-bound agents (dynamic-agent-routing task
10). Changing the workspace-wide release used by routing settings changes.

## Implementation acceptance

- A viewer and an Office agent both get 403 from the switch endpoint and
  nothing is written.
- Releasing after a switch turns off re-queues only current-cycle
  author-skip parks at that step; ceiling holds, session-recovery parks,
  other steps, other workspaces, and earlier-cycle skips stay parked, on
  SQLite and PostgreSQL.
- A ceiling-replayed run records the deferred attempt's provider, model, and
  execution profile, on SQLite and PostgreSQL.

## Verification

```bash
cd apps/backend
go test -trimpath -count=1 ./internal/office/routing/... ./internal/office/dashboard/... \
  ./internal/office/repository/sqlite/... ./internal/office/scheduler/... \
  ./internal/office/service/... ./internal/workflow/... ./internal/backendapp/...
KANDEV_TEST_POSTGRES_DSN=<dsn> go test -trimpath -count=1 -run 'Postgres' \
  ./internal/office/repository/sqlite/ ./internal/workflow/repository/
cd ../..
bash scripts/lint-go-changed <changed-go-files>
cd apps/web
pnpm exec vitest run components/author-exclusion-warning-list.test.tsx hooks/domains/office lib/author-exclusion-warning-groups.test.ts
pnpm run i18n:check && pnpm run lint && pnpm run typecheck
```

## Likely files

- `apps/backend/internal/office/routing/author_exclusion_setup.go`, `seat_steps.go`, `provider.go` (+ tests)
- `apps/backend/internal/office/repository/sqlite/run_routing.go`, `run_routing_author_release.go` (+ tests)
- `apps/backend/internal/office/dashboard/handler_routing_seat.go`, `service.go`, `routing_dto.go` (+ tests)
- `apps/backend/internal/backendapp/office_scope.go` (+ test)
- `apps/backend/internal/office/models/enums.go`
- `apps/backend/internal/workflow/service/import_exclusion_advisor.go` (+ test)
- `apps/web/components/author-exclusion-warning-list.tsx`, `lib/types/author-exclusion.ts`,
  `lib/api/domains/office-routing-api.ts`, `hooks/domains/office/use-workspace-routing.ts` (+ tests)
- `apps/web/src/locales/*/{workflows,office}.json`

## Risks

- A run released at a step whose switch is still on elsewhere re-parks with
  the same verdict; only runs at the turned-off steps are selected.

## Results

Implemented with tests for each behavior. Results of the verification
commands are recorded in the PR.
