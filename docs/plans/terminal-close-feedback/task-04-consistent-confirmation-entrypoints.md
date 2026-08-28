---
id: "04-consistent-confirmation-entrypoints"
title: "Consistent localized terminal confirmation"
status: done
wave: 4
depends_on: ["03-terminal-close-e2e"]
plan: "plan.md"
spec: "../../specs/ui/requirements/terminal-close-feedback.md"
parallelism: sequential
---

# Task 04: Consistent localized terminal confirmation

## Root cause

The add-panel terminal row retained `window.confirm`, context-menu Terminate bypassed the guarded
tab-close handler, and tab/tablet/phone close confirmation was conditional on client-only activity
tracking. Those independent paths produced a browser dialog in one place and no confirmation in
others.

## Intent

Make destructive terminal confirmation predictable at every entry point while keeping it local to
the initiating control and preserving immediate UI dismissal after confirmation.

## Acceptance

- Dockview tab × always opens the compact close-anchored popover.
- Dockview context-menu Terminate opens that same popover and does not destroy before confirmation.
- Tablet close uses the same anchored popover.
- Phone close always morphs the row into touch-sized Cancel and Close actions.
- Add-panel rows open the shared close-anchored popover; parked-terminal rows morph inline into
  Cancel and Close actions.
- No task-terminal path calls browser-native `confirm()`.
- Confirmation still removes local UI before background teardown settles; failure alone toasts.

## Files

- `apps/web/components/task/terminal-tab.tsx`
- `apps/web/components/task/terminal-reopen-menu.tsx`
- `apps/web/components/task/parked-terminals-menu.tsx`
- `apps/web/components/task/task-right-panel.tsx`
- `apps/web/components/task/mobile/mobile-terminals-section.tsx`
- Focused tests and terminal E2E specs

## Verification

```bash
cd apps
pnpm --filter @kandev/web exec vitest run components/task/terminal-tab.test.tsx components/task/mobile/mobile-terminal-row.test.tsx
cd web
pnpm e2e:run tests/terminal/terminal-dockview-ui.spec.ts -- --grep "localized confirmation"
pnpm e2e:run --project mobile-chrome tests/terminal/mobile-terminal-close.spec.ts
```

## Results

- Focused Vitest: 4 files, 12 tests passed.
- Frontend typecheck and targeted ESLint: passed.
- Desktop Chromium: tab ×, context-menu Terminate, and add-panel anchored confirmation passed.
- Mobile Chrome: inline confirmation and immediate removal passed.
- i18n checks, i18n new-code ratchet, and E2E sleep ratchet: passed.
