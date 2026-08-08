---
id: "02-port-dialog-browser-action"
title: "Port dialog browser action"
status: done
wave: 2
depends_on: ["01-dockview-browser-navigation"]
plan: "plan.md"
spec: "../../specs/port-proxy-browser-panel/spec.md"
---

# Task 02: Port dialog browser action

- **Acceptance:**
  - Proxy URL rows show an accessible **Open in browser panel** action when Dockview is available;
    tunnel URL rows do not show it.
  - Clicking the action navigates through the Dockview action and closes the Port Forwarding dialog.
  - Copy and system-browser actions remain available, all new copy is translated, and coarse-pointer
    URL controls have a touch-safe hit area.
- **Verification:**
  - `cd apps && pnpm --filter @kandev/web test -- components/task/port-forward-dialog.test.tsx`
  - `cd apps && pnpm --filter @kandev/web lint`
  - `cd apps && pnpm run i18n:check`
  - `cd apps && pnpm run i18n:ratchet`
  - `cd apps/web && pnpm run typecheck`
- **Files likely touched:**
  - `apps/web/components/task/port-forward-dialog.tsx`
  - `apps/web/components/task/port-forward-dialog.test.tsx`
  - `apps/web/src/locales/en/task.json`
  - `apps/web/src/locales/pt-pt/task.json`
  - `apps/web/src/locales/zh-cn/task.json`
  - `apps/web/src/locales/pseudo/task.json`
- **Dependencies:** Task 01.
- **Parallelism:** sequential.
- **Inputs:** Spec What, failure modes, mobile fallback, and scenarios four through six. Reuse the
  existing Port Forwarding dialog and `useDockviewStore`; do not add a backend or port API call.
- **Output contract:** Summary, exact files changed, focused test/lint/i18n/typecheck results,
  blockers, and updated task/plan status.

## Results

- Added the translated Browser-panel action to proxy URL rows only.
- Kept copy and system-browser actions on proxy and tunnel rows, and made URL controls touch-safe
  on coarse pointers.
- The action closes the dialog after invoking the Dockview navigation action.
- `cd apps && pnpm --filter @kandev/web test -- components/task/port-forward-dialog.test.tsx` — 2 tests passed.
- `cd apps && pnpm --filter @kandev/web lint` — passed with zero warnings.
- `cd apps && pnpm run i18n:check` — passed; existing real-locale parity warnings remain advisory.
- `cd apps && pnpm run i18n:ratchet` — passed.
- `cd apps/web && pnpm run typecheck` — passed.
