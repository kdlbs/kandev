---
id: "09-e2e-and-i18n"
title: "Localized recovery E2E"
status: done
wave: 7
depends_on: ["11-responsive-launch-error-surface"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-launch-failure-recovery.md"
---

# Task 09: Localized recovery E2E

Prove the desktop and phone outcomes against fresh production assets.
Complete all five locale catalogs.

- **Acceptance:**
  1. The desktop spec proves the pre-session gate, reload persistence, terminal move, and zero session launch.
  2. It proves exact-row branch recovery, relaunch, persisted self-heal, and pointer-toast copy.
  3. The old PR missing-branch spec expects the typed card, not a warning message or archive/delete actions.
  4. The mobile spec proves the same branch outcome through `MobilePickerSheet`.
  5. The mobile test checks 44px targets, viewport containment, picker scroll, and no document overflow.
  6. Tests use stable test IDs, UI assertions, causal WS waits, and disposable seeded state.
  7. New copy exists in all five locale catalogs.
  8. Traditional Chinese catalogs come from `pnpm run i18n:zh-hant`.
  9. `pnpm run i18n:check` passes.

- **Verification:**
  `cd apps && pnpm install --frozen-lockfile && cd web && pnpm run i18n:check && pnpm e2e:run tests/pr/pr-watcher-missing-branch.spec.ts tests/task/launch-failure-recovery.spec.ts && pnpm e2e:run --project mobile-chrome tests/task/mobile-launch-failure-recovery.spec.ts`

- **Files likely touched:**
  `apps/web/e2e/tests/task/launch-failure-recovery.spec.ts`,
  `apps/web/e2e/tests/task/mobile-launch-failure-recovery.spec.ts`,
  `apps/web/e2e/tests/pr/pr-watcher-missing-branch.spec.ts`,
  `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`.

- **Dependencies:** Task 11.
- **Parallelism:** sequential.
- **Inputs:** plan "E2E Tests", spec scenarios, `/e2e`, `/mobile-parity`, and root i18n rules.

## Results
Implemented desktop and mobile recovery coverage and updated the deleted-branch watcher cases.
All new task copy is present in the five locale catalogs.

Verification:

- `cd apps/web && pnpm run i18n:check`: passed. The catalogs contain 7,159 referenced keys and complete translations.
- `cd apps/web && pnpm e2e:run --host --no-build tests/task/launch-failure-recovery.spec.ts`: 2 tests passed.
- `cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-launch-failure-recovery.spec.ts`: 1 test passed.
- Desktop and mobile PR watcher specs: 1 test passed in each project.
- Desktop and mobile recovery screenshots were captured and inspected. The picker closes after selection, and no recovery failure toast remains.
