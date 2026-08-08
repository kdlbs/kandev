---
id: "01-dockview-browser-navigation"
title: "Dockview browser navigation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/port-proxy-browser-panel/spec.md"
---

# Task 01: Dockview browser navigation

- **Acceptance:**
  - `openBrowserPanel(url)` updates and focuses the active Browser panel, or the first Browser
    panel when no Browser panel is active.
  - When no Browser panel exists, the action adds one in the central group with `params.url` set
    to `url` and makes it active.
  - The Browser panel updates its address input and iframe source when Dockview changes `params.url`.
- **Verification:**
  - `cd apps && pnpm --filter @kandev/web test -- lib/state/dockview-panel-actions.test.ts`
  - `cd apps/web && pnpm run typecheck`
- **Files likely touched:**
  - `apps/web/lib/state/dockview-panel-actions.ts`
  - `apps/web/lib/state/dockview-store.ts`
  - `apps/web/lib/state/dockview-panel-actions.test-utils.ts`
  - `apps/web/lib/state/dockview-panel-actions.test.ts`
  - `apps/web/components/task/browser-panel.tsx`
  - `apps/web/components/task/browser-panel.test.tsx`
- **Dependencies:** None.
- **Parallelism:** sequential.
- **Inputs:** Spec state/data ownership, API surface, and first three scenarios. Use
  `focusOrAddPanel`, `api.panels`, `panel.api.component`, `panel.api.updateParameters`, and the
  existing panel portal parameter forwarding.
- **Output contract:** Summary, exact files changed, focused test results, typecheck result,
  blockers, and updated task/plan status.

## Results

- Added `openBrowserPanel(url)` to the Dockview actions and store contract.
- Reuses the active Browser panel, then the first Browser panel, and otherwise creates one in the
  central group with a stale-group fallback.
- Synchronizes Browser panel address state when Dockview changes `params.url`.
- `cd apps && pnpm --filter @kandev/web test -- lib/state/dockview-panel-actions.test.ts` — 31 tests passed.
- `cd apps/web && pnpm run typecheck` — passed.
- Browser parameter synchronization is covered by the desktop E2E scenarios in Task 03.
