---
id: "08-prove-and-document-coordinators"
title: "Validate coordinator automations"
status: completed
wave: 4
depends_on:
  - "01-persist-continuation-policy"
  - "02-dispatch-reusable-turns"
  - "03-bound-fallback-resume-history"
  - "04-define-automation-mcp-surface"
  - "05-maintain-shared-run-lifecycle"
  - "06-add-continuation-control"
  - "07-anchor-and-control-shared-runs"
plan: "plan.md"
spec: "../../specs/office/requirements/automation-continuity.md"
---

# Task 08: Validate Coordinator Automations

## Acceptance

- Desktop/mobile E2E show the continuity help descriptions, create a reusable automation, fire it
  twice, observe distinct titled turns in one task/session/worktree, and stop an exact open run.
- Backend integration proves the fixed catalog, central principal, self/foreign denial,
  target-profile spawning, bounded fallback history, stale-run repair, and restart-safe deletion
  cleanup.
- Public/API docs accurately describe configuration, agent compaction, the newest-50 message
  fallback, tool-event exclusion, recovery, security boundaries, and non-destructive worktree
  behavior.

## TDD scenarios

1. RED: Add stable mock/backend fixtures for shared task/session, distinct turns/titles, and a live
   stoppable run.
2. RED: Add desktop help-copy, editor/detail flow, and exact-run stop assertions.
3. RED: Add mobile help-copy and create/select/stop parity, 44 px actions, drawer containment, and
   overflow checks.
4. RED: Add integration matrices for principal scope, self/foreign targets, spawned profiles,
   history bound, stale-run recovery, and cleanup-job retry after restart.
5. GREEN: Complete integration wiring and public documentation.
6. REFACTOR: Remove obsolete execution-mode, `dispatching`, and handler-local scope wording.

## Verification

- `cd apps/backend && go test -tags fts5 ./internal/automation ./internal/orchestrator ./internal/mcp/profile ./internal/mcp/scope ./internal/mcp/handlers ./internal/mcp/server && go test ./internal/agent/runtime/lifecycle`
- `cd apps && pnpm install --frozen-lockfile`
- `cd apps/web && pnpm e2e:run tests/automations-settings.spec.ts tests/automations-run-detail.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/mobile-automations-scroll.spec.ts tests/mobile-automation-detail.spec.ts`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm run i18n:check`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `git diff --check`

## Files likely touched

- `apps/web/e2e/tests/automations-settings.spec.ts`
- `apps/web/e2e/tests/automations-run-detail.spec.ts`
- `apps/web/e2e/tests/mobile-automations-scroll.spec.ts`
- `apps/web/e2e/tests/mobile-automation-detail.spec.ts`
- `docs/public/automation-and-mcp.md`
- `docs/public/agents-and-profiles.md`
- `docs/public/websocket-api.md`
- `docs/public/feature-status.md`

## Dependencies

All contract, runtime, security, cleanup, and UI tasks must finish before end-to-end evidence.

## Inputs

- Completed Tasks 01 through 07 and their targeted evidence.
- Public automation, agent/profile, WebSocket API, and feature-status pages.

## Parallelism

Sequential final integration, E2E, and documentation task.

## Output contract

Report desktop/mobile traces, exact identities/titles, catalog/scope matrices, history bounds,
cleanup evidence, docs validation, files changed, and every command result.

## Risks

- Mock fixtures can hide real resume, compaction, scope, cancellation, or cleanup integration gaps.

## Results

Added coordinator security/catalog tests, continuity and exact-stop desktop/mobile E2E coverage, and updated the four public documentation pages. Public-doc tests and validation passed; all 34 focused desktop/mobile E2E tests passed.
