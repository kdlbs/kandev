---
id: "03-palette-results"
title: "Conversation results on desktop and phone"
status: done
wave: 3
depends_on: ['02-search-service']
plan: "plan.md"
requirements:
  - REQ-TASKS-CONVERSATION-SEARCH-001
  - REQ-TASKS-CONVERSATION-SEARCH-002
  - REQ-TASKS-CONVERSATION-SEARCH-003
acceptance_criteria:
  - AC-TASKS-CONVERSATION-SEARCH-001.1
  - AC-TASKS-CONVERSATION-SEARCH-001.2
  - AC-TASKS-CONVERSATION-SEARCH-001.3
  - AC-TASKS-CONVERSATION-SEARCH-001.4
  - AC-TASKS-CONVERSATION-SEARCH-001.5
  - AC-TASKS-CONVERSATION-SEARCH-002.4
  - AC-TASKS-CONVERSATION-SEARCH-003.2
  - AC-TASKS-CONVERSATION-SEARCH-003.3
system_design:
  - ../../specs/tasks/system-design/conversation-search.md
---

# Task 03: Conversation results on desktop and phone

## Summary

Show conversation matches through the existing Commands and Tasks scopes. Share query state and navigation across the desktop palette and a touch-accessible phone search surface.

## In scope

Typed client, debounced independent reads, active-first grouping, metadata deduplication, excerpts, matching-session navigation, clear states, phone entry/shell, seven-language localization and pseudo.

## Out of scope

A new Contents mode, exact message scrolling, board-filter redesign, and changes to task lifecycle.

## Acceptance

- Tests prove metadata remains usable during conversation failure and obsolete workspace/query/scope responses cannot replace current rows. Explicit keyboard selection survives late results.
- Desktop E2E finds tasks from user and assistant text in non-primary sessions, deduplicates metadata matches, and opens the matching session without starting agents.
- Phone E2E proves touch-only entry, readable excerpt and archive cues, focus return, 44px targets and no horizontal overflow. Shared command confirmations still work.

## UI previews

Use [UI-01 through UI-04 in the plan](plan.md#ui-previews). The user requested
rich previews instead of ASCII. Preserve these structural requirements:

- UI-01: desktop field and tabs above title/state rows with secondary excerpts.
- UI-02: phone inset drawer with fixed input/close, a single scrolling result
  body, safe-area padding, and full-row tap navigation. This uses the shipped
  mobile task-switcher composition, not a narrow desktop side sheet.
- UI-03/UI-04: metadata rows remain while indexing or failures are disclosed.
  Empty and limited searches have different text and status and refine-query guidance.

These views implement AC-TASKS-CONVERSATION-SEARCH-001.4 and
AC-TASKS-CONVERSATION-SEARCH-001.5. New result arrival has no animation.

## Verification

Run from the repository root. PostgreSQL uses a disposable test database.
The commands below use the delivered test names.

```bash
pnpm --dir apps install --frozen-lockfile
(cd apps/web && pnpm exec vitest run hooks/use-command-panel-task-results.test.ts hooks/use-command-panel-task-navigation.test.ts components/command-panel-dialog.test.tsx components/command-panel-confirmation.test.tsx lib/api/domains/conversation-search-api.test.ts components/navigation/mobile-task-search-action.test.tsx components/navigation/app-nav-sheet.test.tsx components/kanban/kanban-header-mobile.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/conversation-search.spec.ts tests/command-panel.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-conversation-search.spec.ts tests/task/mobile-command-palette-task-actions.spec.ts tests/chat/mobile-quick-chat-cancel-palette.spec.ts
git diff --check
```

## Files likely touched

- `apps/web/lib/api/domains/conversation-search-api.ts (new)`
- `apps/web/hooks/use-command-panel-task-results.ts`
- `apps/web/hooks/use-command-panel-task-navigation.ts`
- `apps/web/components/command-panel.tsx`
- `apps/web/components/command-panel-results.tsx`
- `apps/web/components/command-panel-dialog.tsx`
- `apps/web/components/command-panel-footer.tsx`
- `apps/web/components/navigation/app-nav-sheet.tsx`
- `apps/web/components/command-panel-dialog.test.tsx (new)`
- `apps/web/e2e/tests/task/conversation-search.spec.ts (new)`
- `apps/web/e2e/tests/task/mobile-conversation-search.spec.ts (new)`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-tw,zh-hk,ja,ko,pseudo}/*.json`

## Dependencies

Task 02, `02-search-service`.

## Risks

A late result must not change an explicitly selected task. Use Unicode code-point highlighting, plain text rendering, and authorization-aware stale-result handling. A phone wrapper must not remount independent state or stack search over an open menu.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/conversation-search.md).
- [System design](../../specs/tasks/system-design/conversation-search.md).
- [Plan](plan.md), scoped AGENTS.md, and the exact source/test patterns named there.

## Results

Implemented and validated. All 55 targeted frontend tests pass. Desktop search and all three phone browser scenarios pass, including existing move/archive and change-workflow actions. TypeScript, ESLint, localization checks, and the production web build pass. The phone drawer regression was reproduced with a failing test before the fix. See [performance results](performance-results.md) for commands and evidence limits.

PR CI exposed four phone-header test failures because their shared fixture omitted the command registry provider used by the application shell. The failures reproduced locally. The fixture now renders through the real provider, and all 55 search/navigation tests pass with `NODE_ENV=production`. This changes test composition only; no product behavior or public documentation changes.

The phone cancellation palette regression was reproduced before the click-boundary fix. All five phone search, task-action, and Quick Chat cancellation scenarios pass together afterward. The 55 targeted unit tests and TypeScript also pass.
