---
id: "01-acceptance-harness"
title: "Lifecycle acceptance harness"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/lsp-file-intelligence/spec.md"
---

# Task 01: Lifecycle Acceptance Harness

## Acceptance

- The fake Kotlin server records process PID, initialize/import invocation, shutdown/exit/signal,
  configuration, progress, and optional controller generation without relying on UI state.
- A new production-build desktop spec expresses Start-without-editor, active-panel/idle
  independence, same-task multi-session/browser deduplication, reload reattachment, exactly-one
  Restart generation, Stop suppression, task-stop cleanup, progress-away-from-file, and disabled
  status-bar fallback.
- The focused contract is observed failing against the current session/browser-owned product for
  the intended missing behavior before production implementation starts.

## TDD sequence

1. Extend the fake server and helpers with deterministic event-count/generation assertions and
   cleanup that never reads another worker's files.
2. Add `task-lsp-lifecycle.spec.ts`, using task controls and UI assertions rather than API-only
   assertions. Use browser clock advancement for the old two-minute timer; do not wait two real
   minutes.
3. Run the smallest Start-without-editor case against a fresh production build and record the
   expected missing-control failure. Confirm Playwright discovers every new test.
4. Keep the test contract fixed while Tasks 02–09 implement it; adjust only selectors/helpers whose
   final accessible contract is recorded in the spec.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium --list e2e/tests/lsp/task-lsp-lifecycle.spec.ts
cd apps/web && pnpm e2e:run tests/lsp/task-lsp-lifecycle.spec.ts -- --grep "starts Kotlin before opening a file"
```

The second command must list the intended count. The managed-runner command is RED evidence in this
task, not a pass requirement; record the exact expected assertion that fails.

## Files likely touched

- `apps/web/e2e/fixtures/fake-lsp-server.mjs`
- `apps/web/e2e/tests/lsp/lsp-e2e-helpers.ts`
- `apps/web/e2e/tests/lsp/task-lsp-lifecycle.spec.ts`
- `apps/web/e2e/helpers/api-client.ts` only if a missing same-task session/stop helper is required

## Dependencies

None.

## Parallelism

Sequential. This executable contract must be fixed before production changes.

## Inputs

- Spec scenarios: Ownership and controls; Status and progress; Discovery, environments, and
  recovery.
- Existing `fake-lsp-server.mjs`, `lsp-e2e-helpers.ts`, and task/session E2E fixtures.
- E2E skill requirements: production build, UI-visible assertions, stable test IDs, no timeout
  inflation, and worker-scoped cleanup.

## Output contract

Report files changed, discovered test count, exact RED command/failure, fixture cleanup behavior,
and any contract ambiguity. Update this task and `plan.md` in the same conversation.

## Results

- Extended `fake-lsp-server.mjs` with controller-generation, explicit initialize, and explicit
  project-import evidence while retaining raw protocol, configuration, exit, signal, PID, and
  timestamp records in the worker-owned backend temp directory.
- Added eight production-build desktop scenarios in `task-lsp-lifecycle.spec.ts`; Playwright listed
  all eight under the `chromium` project.
- RED command:
  `cd apps/web && pnpm e2e:run tests/lsp/task-lsp-lifecycle.spec.ts -- --grep "starts Kotlin before opening a file"`
- Expected RED result: failed at `openTaskLspControl` because
  `getByTestId("task-lsp-control")` was absent (`element(s) not found`, 5-second locator timeout).
  This is the missing start-before-editor task surface, not an infrastructure or compile failure.
