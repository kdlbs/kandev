---
id: "01-quick-terminal-ui"
title: "Build the shared Quick Terminal UI"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-terminal.md"
---

# Task 01: Build the shared Quick Terminal UI

## Acceptance

- One global, lazy-loaded launcher opens the existing host shell in an opt-in Quick Terminal
  presentation; current Agents/login dialog dimensions and cleanup behavior remain unchanged.
- With an active workspace, the expanded desktop New Task row and the tablet/phone Home and Tasks
  headers show an accessible Quick Terminal action immediately before Quick Chat; the action is
  absent in collapsed or workspace-less states.
- Tablet/desktop receive a larger contained floating terminal, while phone receives a safe-area-aware
  full-height surface with one flexible xterm body and translated new copy.

## Verification

From the repository worktree:

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- components/quick-terminal/quick-terminal-provider.test.tsx components/app-sidebar/app-sidebar-new-task-item.test.tsx components/kanban/kanban-header-mobile.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web exec eslint components/settings/pty-terminal-dialog.tsx components/settings/host-shell-dialog.tsx components/quick-terminal/quick-terminal-provider.tsx hooks/use-quick-terminal-launcher.ts components/app-sidebar/app-sidebar-new-task-item.tsx components/kanban/kanban-header.tsx components/kanban/kanban-header-mobile.tsx app/layout.tsx src/app-shell.tsx
cd apps && pnpm --filter @kandev/web i18n:ratchet
```

## Files likely touched

- `apps/web/components/settings/pty-terminal-dialog.tsx`
- `apps/web/components/settings/host-shell-dialog.tsx`
- `apps/web/components/quick-terminal/quick-terminal-provider.tsx`
- `apps/web/components/quick-terminal/quick-terminal-provider.test.tsx`
- `apps/web/hooks/use-quick-terminal-launcher.ts`
- `apps/web/app/layout.tsx`
- `apps/web/src/app-shell.tsx`
- `apps/web/components/app-sidebar/app-sidebar-new-task-item.tsx`
- `apps/web/components/app-sidebar/app-sidebar-new-task-item.test.tsx`
- `apps/web/components/kanban/kanban-header.tsx`
- `apps/web/components/kanban/kanban-header-mobile.tsx`
- `apps/web/components/kanban/kanban-header-mobile.test.tsx`
- `apps/web/src/locales/en/sidebar.json`
- `docs/plans/quick-terminal/plan.md` (status/results only)
- `docs/plans/quick-terminal/task-01-quick-terminal-ui.md` (status/results only)

## Dependencies

None.

## Parallelism

Sequential. The provider, shared dialog props, and launcher placements form one frontend contract
and touch shared layout/header files.

## Inputs

- Spec: `What`, `Failure modes`, and all `Scenarios` in
  `docs/specs/ui/requirements/quick-terminal.md`.
- Plan: `Responsive PTY presentation`, `Global launcher ownership`, `Desktop sidebar action`,
  `Tablet and phone actions`, `Mobile design contract`, and `Risks` in `plan.md`.
- Existing patterns: `apps/web/components/settings/host-shell-dialog.tsx`,
  `apps/web/components/settings/pty-terminal-dialog.tsx`,
  `apps/web/components/quick-chat/quick-chat-modal.tsx`, and the Quick Chat actions in the sidebar
  and Kanban headers.

## Implementation notes

- Follow TDD: add the provider/sidebar/mobile-header component assertions and observe them fail before
  changing production components.
- Keep the Quick Terminal presentation opt-in. Do not alter the default layout used by Agents,
  agent-login, installed-agent, profile-status, or chat recovery callers.
- Lazy-load the PTY-bearing host-shell component from the global provider so the initial root bundle
  does not eagerly include xterm.
- Use `useTranslation()` at render time for all new accessible labels/tooltips. Do not translate
  identifiers or call `t()` at module scope.

## Output contract

Report behavior implemented, files changed, exact command outcomes, blockers, residual risks, and
the task/plan status update in this conversation. Set this task to `in_progress` before production or
test changes, then replace `## Results` and mark it `done` only after every acceptance condition and
targeted command passes.

## Results

- `cd apps && pnpm install --frozen-lockfile` — passed; workspace dependencies installed.
- `cd apps && pnpm --filter @kandev/web test -- --run components/quick-terminal/quick-terminal-provider.test.tsx components/app-sidebar/app-sidebar-new-task-item.test.tsx components/kanban/kanban-header-mobile.test.tsx` — passed; 3 files, 23 tests.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps && pnpm --filter @kandev/web exec eslint components/settings/pty-terminal-dialog.tsx components/settings/host-shell-dialog.tsx components/quick-terminal/quick-terminal-provider.tsx hooks/use-quick-terminal-launcher.ts components/app-sidebar/app-sidebar-new-task-item.tsx components/kanban/kanban-header.tsx components/kanban/kanban-header-mobile.tsx app/layout.tsx` — passed with no warnings or errors after extracting tablet actions.
- `cd apps && pnpm --filter @kandev/web i18n:ratchet` — passed; 2 added and 7 modified files clean.
- Security/trust and external side effects: starts and stops the existing host-shell session and
  accepts shell commands; no new backend or authorization policy was added.
