---
id: "01-restore-selection"
title: "Remember and restore conversation selection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-QUICK-TERMINAL-003
acceptance_criteria:
  - AC-UI-QUICK-TERMINAL-003.1
  - AC-UI-QUICK-TERMINAL-003.2
  - AC-UI-QUICK-TERMINAL-003.3
  - AC-UI-QUICK-TERMINAL-003.4
  - AC-UI-QUICK-TERMINAL-003.5
  - AC-UI-QUICK-TERMINAL-003.6
  - AC-UI-QUICK-TERMINAL-003.7
system_design:
  - ../../specs/ui/system-design/quick-chat-selection.md
---

# Task 01: Remember and restore conversation selection

## Summary

Implement the remembered selection as one complete behavior, from user action
through browser storage to launcher restoration. Prove it on desktop and phone.

## In scope

- Typed selection state, validated storage, user scope, readiness, and race guards.
- All explicit activation paths and authoritative removal paths.
- Launcher, hydration, resync, store, storage, and rendered regression tests.
- Relevant public documentation corrections during implementation, if needed.

## Out of scope

- Agent goal UI and metadata retention belong to Task 02.
- Cross-device selection and transcript scroll restoration.
- New dialog composition, tab-order persistence, and terminal lifecycle changes.

## Acceptance

1. Write the workspace-switch RED test from the plan before production changes.
   Record its expected selected session and actual first-tab fallback.
2. Implement all seven criteria without transiently mounting an unrelated conversation.
   Preserve explicit launches, kind isolation, and existing terminal behavior.
3. Run every required check below. Record commands and results before marking this work order done.

## ASCII UI preview

Use [UI-01 and UI-02](plan.md#ascii-ui-preview), covering `AC-UI-QUICK-TERMINAL-003.1`–`.7`.

```text
Desktop: Archive tasks  [List nova28's open PRs]
                       Previously selected content

Phone:  Quick Chat                      Close
        ... [List nova28's open PRs] ...
        Previously selected content
        Existing composer
```

The phone menu closes before the existing full-height dialog opens. Keep the tab
strip's horizontal scroll and the content's vertical scroll. Loading must not
mount the first conversation while the saved conversation is still unresolved.
Compare the final rendered active content against both previews.

## Verification

Run from the repository root. On a fresh worktree, install once with
`(cd apps && pnpm install --frozen-lockfile)` before package commands.

```bash
(cd apps/web && pnpm exec vitest run hooks/use-quick-chat-launcher.test.ts hooks/use-quick-chat-resync.test.ts lib/quick-chat/selection-storage.test.ts lib/state/slices/ui/quick-chat-selection.test.ts lib/state/slices/ui/quick-chat-selection-actions.test.ts lib/state/slices/ui/quick-chat-actions.test.ts lib/state/slices/ui/quick-chat-sync.test.ts lib/state/slices/auth lib/state/hydration)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/quick-chat.spec.ts tests/chat/quick-chat-cross-device.spec.ts tests/terminal/quick-terminal.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-tabs.spec.ts tests/terminal/mobile-quick-terminal.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Managed E2E builds current sources. Run desktop and mobile separately.
`selection-storage.test.ts` and `quick-chat-selection.test.ts` are new files.
Retain all existing test scenarios and include any additional changed suite in final checks.

## Files likely touched

- New `apps/web/lib/quick-chat/selection-storage.ts` and `.test.ts`.
- New `apps/web/lib/state/slices/ui/quick-chat-selection.ts` and `.test.ts`.
- `apps/web/lib/state/slices/ui/types.ts`, `ui-slice.ts`, `quick-chat-actions.ts`, and `quick-chat-sync.ts`.
- `apps/web/lib/state/slices/ui/quick-terminal-actions.ts` and related tests.
- `apps/web/lib/state/store.ts`, `default-state.ts`, `store-overrides.ts`, and `app-state-types.ts` as needed.
- `apps/web/lib/state/hydration/hydrator.ts` and related tests.
- `apps/web/lib/state/slices/auth/auth-slice.ts` and related tests.
- `apps/web/hooks/use-quick-chat-launcher.ts`, `use-quick-chat-resync.ts`, and their tests.
- `apps/web/components/quick-chat/use-quick-chat-modal.ts` and `quick-chat-modal.tsx` only for pending-open integration.
- `apps/web/components/quick-chat/use-quick-chat-close-actions.ts` for replacement-selection integration.
- Existing E2E files listed in Verification.
- Owning requirement/design and this package for final status and results.

## Dependencies

None. Work remains in the primary session.

## Risks

Readiness and remembered choice must remain independent from list order and agent
activity. Preserve the existing resync revision checks and authoritative deletion semantics.
Do not persist setup IDs, secrets, or conversation text.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/quick-terminal.md), `REQ-UI-QUICK-TERMINAL-003` and existing launcher compatibility.
- [Design](../../specs/ui/system-design/quick-chat-selection.md).
- [Portable order ADR](../../decisions/2026-08-26-quick-chat-tab-order.md).
- [Session resumption](../../specs/tasks/system-design/quick-chat-session-resumption.md).
- `useQuickChatTabOrder`, `orderQuickChatTabs`, and existing launcher tests.

## Results

Implemented remembered Quick Chat selection for each user, workspace, and
conversation kind. The bounded local-storage codec validates entries and keeps
the latest explicit choices. The UI slice now tracks readiness, selection
revisions, and pending restoration. Explicit opens and tab selections update
the remembered choice, while background refreshes, terminal activation, and
fallback selection do not. Close, delete, hydration, resync, and identity
changes clear or restore only the affected scope. Quick Chat waits for an
authoritative list before mounting a remembered or fallback conversation.

Verification passed:

- Focused Quick Chat Vitest: 11 files, 135 tests passed. Review regressions cover hydration restoring the remembered tab and delayed opens carrying persisted mixed-tab order after removal.
- `pnpm run typecheck`, `pnpm run lint`, and `pnpm run i18n:ratchet` passed.
- Desktop Quick Chat E2E: 25 tests passed. Cross-device and terminal E2E: 4 tests passed.
- Mobile Quick Chat and terminal E2E passed with the goal flow: 2 combined tests and 1 mobile terminal test.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.
