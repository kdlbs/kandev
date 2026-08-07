---
id: "29-enhancement-validation"
title: "Azure enhancement validation"
status: completed
wave: 14
depends_on:
  [
    "18-immediate-activation",
    "27-work-item-detail-ui",
    "28-automation-settings",
  ]
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 29: Azure Enhancement Validation

## Acceptance

- The Azure mock/API helper can deterministically seed paginated comments,
  comment failure/retry, unsafe description HTML, PAT identity, revision
  conflict, watcher matches, duplicate polls, reset previews, and terminal
  provider state without external Azure credentials.
- Desktop Playwright proves: core detail plus effort/discussion; Assign to me;
  board column move; unsafe HTML exclusion; quick-action task creation and a
  visible linked task; work-item watcher create/enable/Run now/reset/delete;
  and pull-request watcher creation.
- Mobile Playwright proves direct card-to-detail navigation, full-height
  containment with one internal scroll owner, 44px actions, assignment, board
  move, task launch/link, watcher create/edit/Run now/reset, safe-area
  clearance, and no document horizontal overflow. Radix owns focus return for
  the Dialog/Drawer surfaces.
- E2E asserts visible outcomes and persisted API state, not private React state
  or timing-only sleeps. Watcher generation/dedup and terminal cleanup are
  covered by the Go service/orchestrator tests.
- Public docs describe remembered filters, read-only detail fields, allowed
  assignment/status changes, PAT identity semantics, quick actions/default
  queries, watchers, and immediate activation.

## TDD Sequence

1. For each missing desktop behavior, add the Playwright assertion against the
   current production build and record the expected failure before extending
   mock routes/UI.
2. Add the minimum deterministic mock seed/state transition and product code
   required for the desktop flow to pass; remove no assertion to obtain green.
3. Repeat RED/GREEN for phone-specific drawer, geometry, watcher cards, and
   safe-area/no-overflow assertions in `mobile-azure-devops.spec.ts`.
4. Run both focused specs together, then docs validation and the repository
   verification sequence. Record exact commands/results in Task 29 and the
   parent plan before marking complete.

## Verification

- `pnpm e2e:run --host --no-build tests/integrations/azure-devops.spec.ts -- --project=chromium` from `apps/web` — passed.
- `pnpm e2e:run --host --no-build tests/integrations/mobile-azure-devops.spec.ts -- --project=mobile-chrome` from `apps/web` — passed (2 tests).
- Managed `pnpm e2e:run tests/integrations/azure-devops.spec.ts -- --project=chromium` and the mobile equivalent — passed after the production build.
- `node --test scripts/validate-public-docs.test.mjs` and `node scripts/validate-public-docs.mjs` from the repository root — passed (58 tests; 41 published pages).
- `make fmt`, then `make typecheck test lint` from the repository root — passed.

## Files Likely Touched

- `apps/backend/internal/azuredevops/mock_client.go`
- `apps/backend/internal/azuredevops/mock_controller.go`
- `apps/backend/internal/azuredevops/mock_client_test.go`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/tests/integrations/azure-devops.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-azure-devops.spec.ts`
- `docs/public/integrations.md`

## Dependencies

Tasks 18, 27, and 28, transitively all enhancement tasks.

## Parallelism

Sequential. E2E fixtures and documentation assert the final integrated contract.

## Inputs

- All new scenarios in the Azure DevOps spec.
- Required skills during implementation: `/e2e`, `/mobile-parity`,
  `/docs-maintainer`.
- Existing Azure mock controller and desktop/mobile integration specs.

## Risks

- Run through the managed E2E command so both Go and production Vite artifacts
  are rebuilt; `--no-build` can silently validate stale code.
- Keep each spec isolated with mock reset/seed in setup; watcher reservations
  and task links from one scenario must not make another pass accidentally.

## Output Contract

Report desktop/mobile outcomes, geometry assertions, screenshots/visual
inspection, docs validation, exact commands/results, files changed, residual
risks, and update task/plan status.
