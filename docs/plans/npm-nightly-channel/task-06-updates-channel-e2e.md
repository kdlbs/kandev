---
id: "06-updates-channel-e2e"
title: "Desktop and mobile E2E"
status: completed
wave: 5
depends_on: ["05-updates-channel-ui"]
plan: "plan.md"
spec: "../../specs/release/requirements/npm-nightly-channel.md"
---

# Task 06: Desktop and mobile E2E

- **Acceptance:** Desktop UI selects Nightly, saves, reloads, and applies the registry-resolved exact
  target through a managed npm fixture.
- **Acceptance:** Pixel 5 completes selection/save/reload with 44px rows and no horizontal overflow.
- **Acceptance:** an unsupported install renders Stable plus the server reason without a Nightly
  mutation path.
- **Verification:** `cd apps/web && pnpm e2e:run --project chromium tests/system/updates-channel.spec.ts`
- **Verification:** `cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-updates-page.spec.ts`
- **Files likely touched:** `apps/web/e2e/tests/system/updates-channel.spec.ts`,
  `mobile-updates-page.spec.ts`, fixture/API helpers only if the real UI precondition cannot be
  expressed today.
- **Dependencies:** Task 05 and production build from Tasks 03–05.
- **Parallelism:** sequential.
- **Inputs:** `/e2e` fixture rules and the plan mobile design contract.
- **Risks:** E2E must use production Go-served assets and worker-scoped backend URLs.

## Verification results

- `cd apps/web && pnpm e2e:run --project chromium tests/system/updates-channel.spec.ts` — passed,
  2 tests, including exact target confirmation, target-version observation, automatic completion
  reload, and cleared progress state.
- `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome
  tests/system/mobile-updates-page.spec.ts` — passed, 2 tests on the Pixel 5 project against the
  desktop run's production assets, including a long canonical Nightly target and the
  unsupported-channel reason copy.
- The production Go-served E2E build used a loopback npm metadata fixture behind the strict
  `KANDEV_E2E_MOCK=true` gate; selection, real PATCH persistence, reload, unsupported capability,
  exact target display, 44px rows, and document overflow were exercised.
