---
id: "03-progress-e2e"
title: "Desktop and tablet progress E2E"
status: completed
wave: 3
depends_on: ["02-progress-disclosure"]
plan: "plan.md"
spec: "../../specs/platform/requirements/lsp-file-intelligence.md"
---

# Task 03: Desktop and Tablet Progress E2E

## Acceptance

- The deterministic fake Kotlin server can hold initialize work, report determinate progress, and finish on a test-controlled signal.
- Desktop E2E proves reported progress, the no-report fallback, completion copy, and lifecycle actions through the UI.
- Coarse-pointer tablet E2E proves drawer presentation, touch geometry, viewport containment, and no horizontal overflow while existing phone no-LSP coverage stays green.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/lsp/lsp-file-intelligence.spec.ts
cd apps/web && pnpm e2e:run --no-build -- --project=mobile-chrome tests/lsp/mobile-lsp-file-intelligence.spec.ts
```

## Files Likely Touched

- `apps/web/e2e/fixtures/fake-lsp-server.mjs`
- `apps/web/e2e/tests/lsp/lsp-e2e-helpers.ts`
- `apps/web/e2e/tests/lsp/lsp-file-intelligence.spec.ts`
- `apps/web/e2e/tests/lsp/mobile-lsp-file-intelligence.spec.ts`
- Existing Docker/SSH LSP specs where one-click lifecycle assumptions require migration.

## Dependencies

Task 02.

## Parallelism

Sequential.

## Inputs

- Spec progress and responsive scenarios.
- Existing LSP fake-server event log and task seeding helpers.

## Output Contract

Record RED/GREEN E2E evidence, exact commands and outcomes, migrated selectors/actions, screenshots or rendered geometry evidence, and update this task plus `plan.md`.

## Result

- RED: desktop tests first observed no project-work disclosure; the initial tablet test also exposed the tablet file tree's different panel root. Container coverage then caught a lifecycle-helper race with the popover's closing animation.
- GREEN: the fake server can hold initialization and emit controlled begin/report/end frames; desktop proves reported and no-report states, tablet proves the shared drawer and touch geometry, and the lifecycle helper now keys off the trigger's authoritative expanded state.
- Review hardening: desktop crash recovery now proves the close reason, error state, and Retry action before reconnecting, while cross-language crash isolation retains the surviving connection.
- Review hardening: elapsed-time E2E assertions follow the localized English unit catalog (`sec`) instead of the removed hard-coded suffix.
- Review hardening: the secondary-repository task-host scenario pins a target opened from the Files tree, navigates back to it through LSP, and proves the existing task-root-relative tab is reused instead of duplicated.
- Review hardening: the full Kotlin protocol scenario accepts the fake server's completion item without `textEdit`, verifies Monaco inserts it through the current-word fallback range, and undoes the edit before continuing navigation and save coverage.
- Review hardening: the fake server marks its completion list incomplete, and the full Kotlin protocol scenario proves continued typing produces a trigger-kind-3 refresh before accepting the item.
- CI hardening: the SSH boundary scenario asserts the localized unsupported-executor category instead of the removed backend transport sentence.
- Verified:
  - `pnpm e2e:run --no-build -- --project=chromium tests/lsp/lsp-file-intelligence.spec.ts` — 13 passed
  - `pnpm e2e:run tests/lsp/lsp-file-intelligence.spec.ts -- --grep "runs Kotlin intelligence through the task host"` — 1 production-build Chromium scenario passed with incomplete completion retriggering
  - `pnpm e2e:run --no-build -- --project=mobile-chrome tests/lsp/mobile-lsp-file-intelligence.spec.ts` — 3 passed
  - `KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --no-build -- --project=containers tests/docker/lsp-file-intelligence.spec.ts` — 3 passed
  - `KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --no-build -- --project=containers tests/ssh/lsp-unsupported-executor.spec.ts` — 1 passed
