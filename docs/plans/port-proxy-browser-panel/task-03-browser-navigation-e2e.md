---
id: "03-browser-navigation-e2e"
title: "Browser navigation E2E coverage"
status: done
wave: 3
depends_on: ["02-port-dialog-browser-action"]
plan: "plan.md"
spec: "../../specs/port-proxy-browser-panel/spec.md"
---

# Task 03: Browser navigation E2E coverage

- **Acceptance:**
  - Desktop E2E proves a proxy URL opens a new central Browser panel and navigates its address and
    iframe.
  - Desktop E2E proves an existing Browser panel is reused without creating a duplicate.
  - Mobile E2E proves the system-browser fallback remains reachable and the Port Forwarding dialog
    has no viewport or horizontal-overflow regression.
- **Verification:**
  - `cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts e2e/tests/session/port-forward-dialog.spec.ts --project=chromium`
  - `cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts e2e/tests/session/mobile-port-forwarding.spec.ts --project=mobile-chrome`
  - `cd apps && pnpm --filter @kandev/web build:vite`
  - `cd apps/web && pnpm run typecheck`
- **Files likely touched:**
  - `apps/web/e2e/pages/session-page.ts`
  - `apps/web/e2e/tests/session/port-forward-dialog.spec.ts`
  - `apps/web/e2e/tests/session/mobile-port-forwarding.spec.ts`
- **Dependencies:** Task 02.
- **Parallelism:** sequential.
- **Inputs:** Spec scenarios one, two, and five. Use the existing session seed helpers, Dockview
  window test API, proxy URL row test IDs, and mobile viewport assertions. Rebuild the production
  web bundle before E2E execution.
- **Output contract:** Summary, exact files changed, build and E2E commands with pass counts,
  cleanup evidence, blockers, and updated task/plan status.

## Results

- Added desktop coverage for new central Browser panel creation and existing Browser panel reuse.
- Added mobile coverage for the system-browser fallback and absence of the Dockview-only action.
- `cd apps && pnpm --filter @kandev/web build:e2e` — passed.
- `cd apps/web && pnpm e2e:run --host --no-build --project chromium tests/session/port-forward-dialog.spec.ts` — 14 tests passed.
- `cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/session/mobile-port-forwarding.spec.ts` — 1 test passed.
