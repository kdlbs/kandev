---
id: "02-present-history-choices"
title: "Present branch history choices"
status: done
wave: 2
depends_on: ['01-observe-local-rebase']
plan: "plan.md"
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-003
acceptance_criteria:
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.1
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.2
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.3
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.4
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.5
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.6
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.7
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.8
system_design:
  - ../../specs/tasks/system-design/branch-history-explanations.md
---

# Task 02: Present branch history choices

## Summary

Connect evidence to the existing contribution UI. Deliver neutral wording, readable version choices, and a phone drawer through shared state.

## In scope

- Add the explanation client and hook with complete identity keys and stale-result rejection.
- Wire compare navigation to existing task and expanded PR histories.
- Update header, VCS, phone actions, confirmations, and all locale catalogs.
- Preserve current action policy and resolution handlers.

## Out of scope

Automatic reconciliation, provider writes outside existing confirmed operations,
and unrelated Git or UI refactors.

## Acceptance

- Explanation loading, success, fallback, and head changes produce the defined wording without changing permissions.
- All entry points offer comparison first and clearly describe publication and restoration.
- Desktop and phone follow UI-01 through UI-04 with shared behavior and keyboard/touch accessibility.

## ASCII UI preview

UI-01: Desktop Changes toolbar, diverged state. Current source shows a yellow
warning menu titled “Task and PR histories differ”, with comparison first.
The menu keeps one toolbar entry and makes comparison primary.

```text
Changes                        [histories differ]
                 +----------------------------------------+
                 | Task and PR histories differ           |
                 | The task branch was rebased locally.   |
                 | The PR still has the earlier history.  |
                 | Task: 5   Published: 5   Newer base: 29 |
                 | [Compare versions]                     |
                 | Publish task version...                |
                 | Restore published PR version...        |
                 | Open PR on GitHub                      |
                 +----------------------------------------+
```

UI-02: Phone Changes header entry, same state. A short choice deserves an
inset bottom drawer. Comparison dismisses this drawer and opens Changes.

```text
+-----------------------------------+
| Changes                 [History] |
| Task history                      |
| ...                               |
|  +-----------------------------+  |
|  | Task and PR histories differ|  |
|  | Local rebase detected.      |  |
|  | Task commits: 5             |  |
|  | Published commits: 5        |  |
|  | Newer base commits: 29      |  |
|  | [Compare versions]          |  |
|  | Publish task version...    |  |
|  | Replaces published history.|  |
|  | Restore published version..|  |
|  | Creates a recovery branch. |  |
|  | Open PR on GitHub          |  |
|  | [Close]                    |  |
|  +-----------------------------+  |
+-----------------------------------+
```

UI-03: Both surfaces, evidence states. These replace only the explanation body.

```text
Loading: Task and published histories differ.
         Checking local history...
Unknown: Task and published histories differ.
         Compare them before choosing a version.
Stale:   Discard the old explanation and return to neutral wording.
         Existing current-provider action gates remain in force.
```

UI-04: Compare versions, existing Changes view, desktop and phone.

```text
Task version                    [current local head]
  local commits and existing local actions
PR #3587 version (expanded)      [current published head]
  published commits, read-only
```

The first comparison heading receives focus. The Changes view owns scrolling.
The drawer has one internal scroll region and safe-area padding. Its touch
controls have at least 44px hit targets. Desktop keeps compact controls.
Action order, version identity, and phone composition are required.
Spacing, counts, and abbreviated drawing text are illustrative. Product copy
uses the full localized labels from the design. These views cover AC 003.1,
003.3, 003.4, 003.6, and 003.8.

Full combined preview: [plan](plan.md#ascii-ui-preview).

## Verification

Run from the repository root. Install workspace dependencies once before pnpm.
Add failing tests first, implement, then rerun these exact checks.

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-contribution-history-explanation.test.tsx hooks/domains/session/remote-contribution-relation.test.ts hooks/domains/session/use-remote-contribution-relation.test.tsx components/task/remote-contribution-header-actions.test.tsx components/task/remote-contribution-resolution-dialog.test.tsx components/task/use-remote-contribution-resolution.test.tsx components/task/mobile/mobile-changes-panel.test.tsx components/task/changes-panel-remote.test.ts components/vcs-split-button.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

## Files likely touched

- New `apps/web/hooks/domains/session/use-contribution-history-explanation.ts` and `.test.tsx`
- `apps/web/hooks/domains/session/use-remote-contribution-relation.ts`
- `apps/web/components/task/remote-contribution-header-actions.tsx`
- `apps/web/components/task/remote-contribution-action-items.tsx`
- `apps/web/components/task/remote-contribution-resolution-dialog.tsx`
- `apps/web/components/task/changes-panel-data.tsx` and relevant history/navigation components
- `apps/web/components/task/mobile/mobile-changes-panel.tsx`
- `apps/web/components/vcs-split-button.tsx` and related multi-repository consumers
- New `apps/web/components/task/mobile/mobile-changes-panel.test.tsx`
- Existing WebSocket client helpers and `apps/web/src/locales/` catalogs

## Dependencies

Task 01.

## Risks

Comparison must target the selected repository without resetting desktop preferences. Long translations must wrap inside the phone drawer.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/tasks/requirements/remote-contribution-tasks.md#amendment-branch-history-explanations)
- [System design](../../specs/tasks/system-design/branch-history-explanations.md)
- [Plan](plan.md)
- Existing contribution resolution tests and branch-scoped selection patterns.

## Results

Implemented the comparison-first divergence menu, shared explanation hook,
desktop and phone surfaces, selected-repository comparison navigation, updated
confirmations, and all supported locale catalogs. The explanation cache is
scoped to mounted consumers and exact identity/head keys, and stale responses
are discarded.

Verification passed:

- `(cd apps/web && pnpm exec vitest run hooks/domains/session/use-contribution-history-explanation.test.tsx hooks/domains/session/remote-contribution-relation.test.ts hooks/domains/session/use-remote-contribution-relation.test.tsx components/task/remote-contribution-header-actions.test.tsx components/task/remote-contribution-resolution-dialog.test.tsx components/task/use-remote-contribution-resolution.test.tsx components/task/mobile/mobile-changes-panel.test.tsx components/task/changes-panel-remote.test.ts components/vcs-split-button.test.ts)` (9 files, 75 tests)
- `(cd apps/web && pnpm run typecheck)`
- `(cd apps/web && pnpm run i18n:check)`
- `(cd apps/web && pnpm run i18n:ratchet)`
- `git diff --check`
