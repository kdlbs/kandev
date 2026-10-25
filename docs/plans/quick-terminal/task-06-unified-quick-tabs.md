---
id: "06-unified-quick-tabs"
title: "Unify Quick Chat and terminal tabs"
status: done
wave: 6
depends_on: ["05-detachable-pty-lifecycle"]
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-terminal.md"
---

# Task 06: Unify Quick Chat and terminal tabs

## Acceptance

- One workspace-scoped store and Quick Chat dialog render chat/configuration and terminal tabs;
  terminal launchers reuse the last terminal or create the first, while **New Terminal** always
  appends a distinct tab and conversation launchers retain their own latest selection.
- The plus action exposes a translated, accessible **Agents**/**Terminals** creation menu without
  duplicating existing tabs; conversation and terminal switching remains in the tab strip. Chat
  tabs can be renamed from their context menu while terminal labels stay fixed. Terminal close
  stops only its PTY while modal dismissal, tab switch, boot hydration, and server resync preserve
  local terminal tabs; active-tab removal selects the nearest remaining same-workspace tab or
  closes the empty dialog.
- The standalone Quick Terminal dialog/provider is removed from both app shells; desktop/tablet and
  phone launchers retain visibility, order, hitboxes, focus return, tooltip behavior, and the shared
  dialog's intentional responsive composition.

## Verification

From the repository worktree:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web exec vitest run lib/state/slices/ui/quick-chat-actions.test.ts lib/state/slices/ui/quick-chat-sync.test.ts lib/state/hydration/hydrator.test.ts components/quick-chat/quick-chat-modal.test.ts components/quick-chat/quick-tab-add-menu.test.tsx hooks/use-quick-chat-launcher.test.ts hooks/use-quick-terminal-launcher.test.ts components/app-sidebar/app-sidebar-new-task-item.test.tsx components/kanban/kanban-header-mobile.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web exec eslint components/quick-chat/quick-chat-modal.tsx components/quick-chat/quick-chat-tab-item.tsx components/quick-chat/quick-tab-add-menu.tsx components/quick-chat/quick-chat-provider.tsx hooks/use-quick-chat-launcher.ts hooks/use-quick-terminal-launcher.ts components/app-sidebar/app-sidebar-new-task-item.tsx components/kanban/kanban-header.tsx components/kanban/kanban-header-mobile.tsx lib/state/slices/ui/types.ts lib/state/slices/ui/ui-slice.ts lib/state/slices/ui/quick-chat-sync.ts lib/state/default-state.ts lib/state/hydration/hydrator.ts app/layout.tsx src/app-shell.tsx)
(cd apps && pnpm --filter @kandev/web run i18n:ratchet)
(cd apps && pnpm --filter @kandev/web run i18n:check)
```

## Files likely touched

- `apps/web/lib/state/slices/ui/types.ts`
- `apps/web/lib/state/slices/ui/ui-slice.ts`
- `apps/web/lib/state/store.ts`
- `apps/web/lib/state/slices/ui/quick-chat-actions.test.ts`
- `apps/web/lib/state/slices/ui/quick-chat-sync.ts`
- `apps/web/lib/state/slices/ui/quick-chat-sync.test.ts`
- `apps/web/lib/state/default-state.ts`
- `apps/web/lib/state/hydration/hydrator.ts`
- `apps/web/lib/state/hydration/hydrator.test.ts`
- `apps/web/components/quick-chat/quick-chat-provider.tsx`
- `apps/web/components/quick-chat/quick-chat-provider.test.ts`
- `apps/web/components/quick-chat/quick-chat-modal.tsx`
- `apps/web/components/quick-chat/quick-chat-modal.test.ts`
- `apps/web/components/quick-chat/use-quick-chat-modal.ts`
- `apps/web/components/quick-chat/use-quick-chat-modal.test.ts`
- `apps/web/components/quick-chat/quick-chat-tab-item.tsx`
- `apps/web/components/quick-chat/quick-chat-tab-item.test.tsx`
- `apps/web/components/quick-chat/quick-tab-add-menu.tsx` (new)
- `apps/web/components/quick-chat/quick-tab-add-menu.test.tsx` (new)
- `apps/web/hooks/use-quick-chat-launcher.ts`
- `apps/web/hooks/use-quick-chat-launcher.test.ts`
- `apps/web/hooks/use-quick-terminal-launcher.ts`
- `apps/web/hooks/use-quick-terminal-launcher.test.ts` (new/replacement)
- `apps/web/components/app-sidebar/app-sidebar-new-task-item.tsx`
- `apps/web/components/app-sidebar/app-sidebar-new-task-item.test.tsx`
- `apps/web/components/kanban/kanban-header.tsx`
- `apps/web/components/kanban/kanban-header-mobile.tsx`
- `apps/web/components/kanban/kanban-header-mobile.test.tsx`
- `apps/web/components/quick-terminal/quick-terminal-provider.tsx` (remove)
- `apps/web/components/quick-terminal/quick-terminal-provider.test.tsx` (remove/replace)
- `apps/web/src/app-shell.tsx`
- `apps/web/app/layout.tsx`
- `apps/web/src/locales/en/sidebar.json` and/or the selected Quick Chat namespace
- `docs/plans/quick-terminal/plan.md` (status/results only)
- `docs/plans/quick-terminal/task-06-unified-quick-tabs.md` (status/results only)

## Dependencies

- Task 05 must be done so terminal tab content has a tested attach/detach lifecycle and explicit
  close API.

## Parallelism

Sequential. Store, modal, provider, launchers, and app-shell ownership form one contract and share
files; splitting them would leave an untestable intermediate state.

## Inputs

- Entire feature spec, especially launcher semantics, tab/menu behavior, workspace isolation,
  persistence, and phone scenarios.
- Plan: all frontend sections, the mobile design contract, frontend tests, and frontend risks.
- Existing patterns:
  `QuickChatModal`, `useQuickChatModal`, `QuickChatProvider`,
  `QuickChatTabItem`, `TerminalReopenMenuItems`, `SessionReopenMenuItems`, and the global
  responsive Radix menu styling.

## Implementation notes

- Follow TDD for state and launch policy before changing rendered composition.
- Keep `QuickChatSessionKind` limited to conversation kinds. Model terminals as a separate
  discriminated descriptor so task deletion, rename, boot sync, and message rendering cannot accept
  one accidentally.
- Preserve server authority for conversation membership while explicitly retaining local terminal
  descriptors during hydration/resync.
- Use `useResponsiveBreakpoint` only where composition needs a branch; reuse the existing Quick
  Chat dialog and menu primitives rather than adding a second phone layout.
- Resolve every new user-facing label through `t()` at render time.

## Risks

- The current modal assumes an active session always belongs to
  `quickChat.sessions`; terminal activation must not trigger its stale-session auto-close effect.
- The active conversation and active terminal must remain separately recoverable so one launcher
  does not steal the other's category.
- Removing the provider must preserve lazy xterm loading and focus restoration without causing the
  sidebar tooltip regression to return.

## Output contract

Report behavior implemented, files added/changed/removed, exact test/lint/typecheck/i18n outcomes,
responsive/focus notes, blockers, residual risks, and synchronized task/plan status. Set this task
to `in_progress` before changes and replace `## Results` before marking it `done`.

## Results

- Added browser-local terminal descriptors and actions for reuse-or-create, always-new creation,
  lifecycle updates, activation, workspace-aware fallback, and explicit removal while preserving
  server-owned conversation reconciliation.
- Unified Quick Chat and terminal content in one responsive dialog with a grouped Agents/Terminals
  creation menu, conversation context-menu rename, translated terminal labels/errors,
  terminal-specific close ownership, and shared launcher focus restoration.
- Removed the standalone Quick Terminal provider from both application shells while retaining lazy
  xterm loading and standard Agents/login dialog cleanup semantics.
- Focused frontend suite: 16 files, 126 tests passed.
- `cd apps/web && pnpm run typecheck` — passed.
- Changed-file frontend and E2E ESLint — passed with zero warnings.
- `cd apps/web && pnpm run i18n:ratchet` — passed; `pnpm run i18n:check` passed with the existing
  orphan catalog entries reported by the checker.
- `git diff --check` — passed.
