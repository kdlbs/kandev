---
id: "03-fix-quick-terminal-startup-race"
title: "Fix Quick Terminal startup race"
status: done
wave: 3
depends_on: ["01-quick-terminal-ui", "02-quick-terminal-e2e"]
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-terminal.md"
---

# Task 03: Fix Quick Terminal startup race

## Acceptance

- StrictMode effect replay cannot stop the session owned by the current terminal mount.
- A live Quick Terminal accepts input after startup and resize requests remain successful.
- A natural PTY exit clears the client session identity and stops further resize requests.
- Restored launcher focus does not reopen its tooltip, while pointer hover still shows it.
- Existing Agents-page terminal cleanup remains unchanged outside the shared lifecycle fix.

## Files

- `apps/web/components/settings/pty-terminal-dialog.tsx`
- `apps/web/components/app-sidebar/app-sidebar-new-task-item.tsx`
- `apps/web/components/app-sidebar/app-sidebar-new-task-item.test.tsx`
- `apps/web/e2e/tests/terminal/quick-terminal.spec.ts`
- `apps/web/e2e/tests/terminal/mobile-quick-terminal.spec.ts`
- `docs/specs/ui/requirements/quick-terminal.md`
- `docs/plans/quick-terminal/plan.md`

## Verification

```bash
cd apps/web && pnpm exec vitest run components/quick-terminal/quick-terminal-provider.test.tsx components/app-sidebar/app-sidebar-new-task-item.test.tsx components/kanban/kanban-header-mobile.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint components/app-sidebar/app-sidebar-new-task-item.tsx components/app-sidebar/app-sidebar-new-task-item.test.tsx e2e/tests/terminal/quick-terminal.spec.ts
cd apps && pnpm --filter @kandev/web run i18n:ratchet
cd apps && pnpm --filter @kandev/web run i18n:check
cd apps/web && pnpm e2e:run tests/terminal/quick-terminal.spec.ts tests/settings/host-shell-pty.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/terminal/mobile-quick-terminal.spec.ts
cd apps/backend && go test ./internal/agent/loginpty
```

## Root-cause reproduction

An isolated dev browser reproduced two `/host-shell/start` requests followed by a stop for the
shared session, `Session ended (exit -1)`, and resize 404s. After the generation guard, the same
surface accepted `echo quick-terminal-fixed` and the resize request returned 200.

## Results

- Added mount-generation ownership so StrictMode replay cannot stop the active session.
- Cleared the client session identity on PTY exit/close to stop stale resize requests.
- Added browser coverage for shell readiness and command input/output.
- Made the sidebar action tooltip pointer-driven and covered focus/hover behavior.
- Passed focused component tests, typecheck, targeted lint, i18n checks, Chromium and Pixel 5 E2E,
  and backend PTY tests.
