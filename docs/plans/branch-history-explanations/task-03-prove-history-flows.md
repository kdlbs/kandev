---
id: "03-prove-history-flows"
title: "Prove rendered history flows"
status: done
wave: 3
depends_on: ['02-present-history-choices']
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

# Task 03: Prove rendered history flows

## Summary

Prove the integrated divergence flows in disposable desktop and phone sessions. Document the shipped behavior after the focused scenarios pass.

## In scope

- Add the scenario matrix from the plan to existing desktop and mobile Git suites.
- Use real local rebase evidence and mock-provider original SHAs.
- Verify no Git writes on comparison, stale lease protection, clean-tree restoration, and recovery results.
- Add the public Git explanation and synchronize specification delivery status after all work orders pass.

## Out of scope

Automatic reconciliation, provider writes outside existing confirmed operations,
and unrelated Git or UI refactors.

## Acceptance

- Both projects pass the planned scenarios with fresh production builds and correct repository identity.
- Phone screenshots and geometry checks match UI-02, with touch targets, focus return, safe areas, and no horizontal overflow.
- Public docs explain the neutral fallback and destructive choices. All package results record actual commands and outcomes.

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

Full combined preview: [plan](plan.md#ascii-ui-preview).

## Verification

Run from the repository root. Install workspace dependencies once before pnpm.
Add failing tests first, implement, then rerun these exact checks.

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/git/git-changes-panel.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-pr-checkout-drift.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/e2e/tests/git/git-changes-panel.spec.ts`
- `apps/web/e2e/tests/git/mobile-pr-checkout-drift.spec.ts`
- Shared Git fixtures/helpers used by those suites
- `docs/public/git-operations.md`
- Requirements, paired designs, and this plan package for completed delivery records

## Dependencies

Task 02.

## Risks

Run projects sequentially with managed runners. Do not use the live diagnostic task or overlap full suites. Rebase fixtures must exercise production evidence rather than a fabricated explanation response.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/tasks/requirements/remote-contribution-tasks.md#amendment-branch-history-explanations)
- [System design](../../specs/tasks/system-design/branch-history-explanations.md)
- [Plan](plan.md)
- Existing contribution resolution tests and branch-scoped selection patterns.

## Results

Implemented the rendered desktop and phone divergence flows with real disposable
Git history evidence. The desktop scenario verifies local-rebase explanation and
separate counts, comparison without Git writes, stale-lease rejection, and the
existing confirmation lease. The phone scenarios verify the inset Drawer, inline
consequences, a single contained scroll owner, touch target sizing, comparison,
stale-lease rejection, clean-tree restoration rejection, and recovery-branch
results. Updated the public Git operations guide and synchronized the requirement
and system-design delivery status.

Verification passed:

- `(cd apps/web && E2E_DEBUG=1 pnpm e2e:run --host --no-build --project chromium tests/git/git-changes-panel.spec.ts)` (25 passed; the preceding E2E run completed the fresh backend and Vite build)
- `(cd apps/web && E2E_DEBUG=1 pnpm e2e:run --host --project mobile-chrome tests/git/mobile-pr-checkout-drift.spec.ts)` (fresh backend and Vite build, 2 passed)
- `node --test scripts/validate-public-docs.test.mjs` (62 passed)
- `node scripts/validate-public-docs.mjs` (46 published docs pages validated)
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
