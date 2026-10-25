---
id: "03-terminal-close-e2e"
title: "Optimistic terminal close E2E"
status: done
wave: 3
depends_on: ["01-nonblocking-close-feedback", "02-responsive-close-feedback"]
plan: "plan.md"
spec: "../../specs/ui/requirements/terminal-close-feedback.md"
---

# Task 03: Optimistic terminal close E2E

## Intent

Prove desktop and phone remove a confirmed terminal while backend teardown is still unresolved.

## Acceptance

- A shared WebSocket route pauses the next `user_shell.destroy` request and forwards unrelated
  frames unchanged.
- Desktop uses a compact anchored popover with no `alertdialog`, then removes the target Dockview tab
  while paused; a sibling accepts input.
- `mobile-chrome` morphs the target row action into inline confirmation with no `alertdialog`, then
  removes the row and mounted terminal while paused; a sibling remains selectable and interactive
  without horizontal overflow.
- The exact paused request is released after each assertion so fixture cleanup sees normal backend
  state.

## Files

- `apps/web/e2e/tests/terminal/terminal-close-pause.ts`
- `apps/web/e2e/tests/terminal/terminal-dockview-ui.spec.ts`
- `apps/web/e2e/tests/terminal/mobile-terminal-close.spec.ts`

## Verification

```bash
cd apps/web
pnpm e2e:run tests/terminal/terminal-dockview-ui.spec.ts -- --grep "confirmed busy close removes the terminal before teardown settles"
pnpm e2e:run --project mobile-chrome tests/terminal/mobile-terminal-close.spec.ts
```

## Results

- Deterministic paused-transport assertions replace observation of transient teardown progress.
- Desktop Chromium: 1 delayed-destroy test passed.
- Mobile Chrome: 1 delayed-destroy test passed with the same production web/backend build.
- The E2E sleep ratchet passed with no new fixed sleeps.
