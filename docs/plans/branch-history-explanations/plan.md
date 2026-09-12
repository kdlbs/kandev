---
created: 2026-09-12
status: completed
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-003
system_design:
  - ../../specs/tasks/system-design/branch-history-explanations.md
legacy_specs: []
---

# Implementation Plan: Branch History Explanations

## Overview

Explain task/PR divergence accurately, including a completed local rebase that
is not yet published. Implement bounded evidence first, responsive UI second,
and rendered regression coverage with public documentation third.

The task system owns checkout preservation and contribution version choices.
The [requirement amendment](../../specs/tasks/requirements/remote-contribution-tasks.md#amendment-branch-history-explanations)
is implemented. Existing requirements 001 and 002 remain active.

## Scope

### In scope

- Neutral divergence wording and optional evidence of a completed local rebase.
- Separate task, published, and base commit counts when graph evidence is complete.
- Comparison-first actions and existing safe resolution operations.
- Desktop menus, phone drawers, all supported locales, and focused regression tests.

### Out of scope

- Automatic fetch, rebase, reset, publication, or agent coordination.
- Determining whether CI or local validation passed.
- Patch-based equality, commit deduplication, and a new comparison editor.
- New providers, GitLab Changes presentation, persistence, and feature flags.

## Technical approach

Add the read-only request described in the [system design](../../specs/tasks/system-design/branch-history-explanations.md).
Use `GitHandlers`, the runtime agentctl client, and `GitOperator` in the current
executor. Add the WS action constant in `apps/backend/pkg/websocket/actions.go`.
The implementation must use existing session authorization and repository routing.

Keep `classifyRemoteContribution` and `remoteContributionActionPolicy` separate
from the explanation. Add a shared, cancellable explanation hook scoped to the
selected repository, branch, PR, and heads. Discard stale results.
Reuse existing Changes histories for comparison. Replace the external-link-only
view action with comparison navigation and retain an explicit provider link.

Update `RemoteContributionHeaderActions`, shared action items, confirmations,
`VcsSplitButton`, and phone Git controls together. A shared view model supplies
both responsive surfaces. Localized labels describe replacement consequences.

## Related delivery records

The completed packages remain historical evidence:

- [Head drift](../remote-contribution-head-drift/plan.md)
- [Local-first choices](../remote-contribution-local-first/plan.md), including task 06
- [History reconciliation](../remote-contribution-history-reconciliation/plan.md)
- [Resume recovery](../contribution-resume-recovery/plan.md)

Do not reopen their completed tasks or replace their recorded test counts.
This package supersedes their warning-copy and action-order expectations only.
During implementation, update affected existing tests as part of Task 02 and 03.

## ASCII UI preview

UI-01: Desktop Changes toolbar, diverged state. The source shows a yellow
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

## Tests

| Criteria | Targeted evidence |
| --- | --- |
| 003.2, 003.3, 003.7 | New `git_contribution_history_test.go`: `TestContributionHistoryLocalRebase`, `TestContributionHistoryUnknown`, `TestContributionHistoryCounts`, `TestContributionHistoryBounds` |
| 003.7, 003.8 | API, client, and handler `TestContributionHistory` tests for routing, authorization, validation, stale heads, and unavailable executors |
| 003.2, 003.5, 003.7, 003.8 | New `use-contribution-history-explanation.test.tsx`: shared reads, stale responses, cancellation, identity isolation, and policy independence |
| 003.1, 003.3, 003.4, 003.5, 003.6 | Existing header, resolution, and Changes suites, plus new `mobile-changes-panel.test.tsx` |
| 003.8 | Locale completeness, plural rules, pseudo catalog, and typecheck |

Test names for new suites are proposed. Existing paths are listed in work orders.
Task 01 uses real temporary Git repositories, including a linked worktree.
It reproduces a five-commit rebase onto 29 newer base commits with conflict resolution.
Assertions distinguish operation evidence from patch equality.
Include expired reflog, shallow history, missing objects, changed provider head,
post-rebase commit, branch switch, merges, and process cancellation.

## E2E tests

Add scenarios to `e2e/tests/git/git-changes-panel.spec.ts` (`chromium`) and
`e2e/tests/git/mobile-pr-checkout-drift.spec.ts` (`mobile-chrome`).
Use a real disposable repository and the existing mock-provider fixture.
The evidence request traverses the production WS and agentctl path.
Mock provider data describes actual original commit SHAs.

| Flow | Criteria |
| --- | --- |
| Local rebase shows neutral title, correct cause and separate counts | 003.1, 003.2, 003.3 |
| Compare expands both histories without Git writes | 003.4, 003.7 |
| Publish confirmation and stale lease rejection preserve remote history | 003.5 |
| Restore requires clean files and reports a recovery branch | 003.5 |
| Missing explanation evidence leaves neutral usable UI | 003.2, 003.8 |
| Phone drawer, inline consequences, comparison, focus return and containment | 003.4, 003.6 |
| Provider moves or repository switches during evidence request | 003.7 |
| Existing aligned and linear-history behavior remains intact | 003.8 |

## Work orders

- [x] [Task 01: Observe local rebase evidence](task-01-observe-local-rebase.md)
- [x] [Task 02: Present branch history choices](task-02-present-history-choices.md)
- [x] [Task 03: Prove rendered history flows](task-03-prove-history-flows.md)

Execute sequentially: 01 -> 02 -> 03. No subagents are authorized.
Each work order follows TDD and records its exact checks after implementation.
Install dependencies once with `(cd apps && pnpm install --frozen-lockfile)`.
Production and permanent test changes belong in the work orders.

## Verification results

Implementation and design-package validation on 2026-09-12:

- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local Markdown links and work-order acceptance references: all resolve.
- `git diff --check -- docs/specs docs/plans/branch-history-explanations`: passed.
- `git status --short -- docs/specs docs/plans/branch-history-explanations`: all four package files and specification edits accounted for.

Task 01 and Task 02 results record their focused backend and frontend checks.
Task 03 rendered-flow checks passed for desktop and phone, and public Git
documentation was updated.

## Risks

- Git reflog formats and expiration limit local-rebase detection. Neutral wording is the required fallback.
- Conflict resolution changes patches. Rebase detection cannot claim identical content.
- Concurrent agent work can invalidate a snapshot. Exact head checks and browser ownership keys prevent stale explanations.
- A new diagnostic endpoint must work through remote executors without backend-local filesystem assumptions.
- Existing mobile menu and Changes tests contain old labels and action order.
- A publication label can hide destructive effects unless confirmation and inline consequences remain explicit.

## Documentation impact

This implementation updates `docs/public/git-operations.md` with the neutral
fallback, comparison-first path, and replacement consequences. No ADR change is
required because mutation policy and commit identity remain unchanged.
