---
id: "01-preserve-pr-details"
title: "Preserve negative-projection PR details"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PR-TASK-STATUS-SUMMARY-001
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003
acceptance_criteria:
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.2
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.3
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.6
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.8
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.15
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.17
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.24
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.4
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.8
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.4
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.6
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.7
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.8
system_design:
  - ../../specs/ui/system-design/pr-task-status-summary.md
  - ../../specs/integrations/system-design/github-workflow-attention.md
---

# Task 01: Preserve negative-projection PR details

## Summary

Keep linked PR content after a newer task summary clears approval.
Suppress stale approval without a blank or automation-only disclosure.
Prove the same outcome in desktop hover/focus and the existing phone drawer.

## In scope

- A negative-projection disclosure helper and its integration into the PR task view model.
- Full identity retention, independent status rows, terminal lifecycle, counts, and conflict attribution.
- Projection/component regressions and two focused rendered scenarios.

## Out of scope

- Provider/API/store schema changes, polling, merge authorization, and automation mutations.
- Tooltip geometry, scroll mechanics, positive-projection redesign, new copy, and new mobile surfaces.

## Acceptance

1. Every cached linked PR retains its number, title, author, and applicable status after a newer explicit negative.
2. No cleared approval row or stale approval note survives the negative, and freshness/positive behavior keeps its existing guards.
3. Desktop hover/focus and phone tap show that content, preserve counts and conflict attribution, and require no extra provider reads.

These conditions map to the complete acceptance IDs in frontmatter.

## ASCII UI preview

See [UI-01 and UI-02](plan.md#ascii-ui-preview).

```text
UI-01 Desktop hover          UI-02 Phone drawer
+------------------------+   +------------------------+
| PR #42 / Test PR       |   | PR #42 status          | fixed
| by alice               |   |------------------------|
| State   Merged         |   | PR #42 / Test PR       | scroll
+-----------v------------+   | by alice / Merged      |
Task [PR]                    +------------------------+
```

A newer negative removes approval text, not the PR entry.
Open PR automation follows the retained summary inside the existing body.
The phone control opens its existing drawer without task navigation.
Summary AC 001.2/.3/.17/.24 and workflow AC 003.4/.8 define these outcomes.

## Implementation sequence

1. Mark this work order `in_progress`.
2. Extend the existing newer-negative component test to require title, author, and status content.
3. Add merged, closed, open-automation, mixed-sibling, and repository-collision cases.
4. Run the regression and record its expected missing-content failure before production changes.
5. Add the focused projection helper and negative-path view-model integration.
6. Preserve the existing positive projection and freshness behavior.
7. Add deterministic desktop and phone response-fixture regressions from the plan.
8. Run the commands below sequentially and record actual results.
9. Mark this work order `done` and update the plan after all checks pass.

The temporary investigation test is removed. Recreate its behavior in permanent tests during TDD.

## Verification

Run from the repository root.
If the worktree lacks dependencies, install them first:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

The managed E2E runner rebuilds runtime and web assets.
Run the two E2E commands sequentially with one shard and the guarded worker budget.

```bash
(cd apps/web && pnpm exec vitest run components/github/pr-task-workflow-projection.test.ts components/github/pr-task-icon.workflow-approval.test.tsx components/github/pr-task-icon.negative-projection.test.tsx components/github/pr-task-icon.render.test.tsx components/github/pr-task-status-summary.test.ts hooks/domains/github/use-task-pr-tooltip-hydration.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/github/pr-task-workflow-projection.ts components/github/pr-task-icon.tsx components/github/pr-task-workflow-projection.test.ts components/github/pr-task-icon.workflow-approval.test.tsx components/github/pr-task-icon.negative-projection.test.tsx e2e/helpers/pr-negative-projection-fixture.ts e2e/tests/pr/pr-sidebar-hover-hydration.spec.ts e2e/tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium -- e2e/tests/pr/pr-sidebar-hover-hydration.spec.ts --workers=1 --retries=0)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome -- e2e/tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts --workers=1 --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Capture the desktop merged hover and phone merged drawer through the existing `prCapture` fixture.
Require actual visible PR content inside the active overlay, not hidden tooltip description text.

## Files likely touched

Owned production files:

- `apps/web/components/github/pr-task-workflow-projection.ts`
- `apps/web/components/github/pr-task-icon.tsx`

Owned regression files:

- `apps/web/components/github/pr-task-workflow-projection.test.ts` (new)
- `apps/web/components/github/pr-task-icon.workflow-approval.test.tsx`
- `apps/web/components/github/pr-task-icon.negative-projection.test.tsx` (new)
- `apps/web/e2e/helpers/pr-negative-projection-fixture.ts` (new)
- `apps/web/e2e/tests/pr/pr-sidebar-hover-hydration.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts`

## Dependencies

None. The existing summary, hover-hydration, and workflow-approval packages are complete.
Their completed results remain historical evidence.

## Risks

The helper must clear approval across every open sibling without changing independent provider statuses.
Same-number PRs require repository identity for conflict attribution.
A single compact lifecycle cannot replace every sibling lifecycle.
E2E response fixtures must preserve the real workspace and association scope.

## Parallelism

`sequential`

## Inputs

- [PR task summary requirements](../../specs/ui/requirements/pr-task-status-summary.md).
- [Workflow attention requirements](../../specs/integrations/requirements/github-workflow-attention.md).
- [Shared summary design](../../specs/ui/system-design/pr-task-status-summary.md).
- [Negative approval disclosure design](../../specs/integrations/system-design/github-workflow-attention.md#negative-approval-disclosure).
- The existing newer positive/negative cases in `pr-task-icon.workflow-approval.test.tsx`.
- The existing desktop inactive-task and phone automation E2E fixtures.

## Results

The original regressions passed after implementation. Before the fix, both new browser scenarios reached the newer-negative fixture branch and failed because the PR status number was absent.

Review follow-up on 2026-10-02 found and fixed two stale merge-row combinations. Before the follow-up production edit, the new pure-helper and component assertions failed on terminal merge rows and compact conflicts paired with `Mergeable`.

- Focused unit/component suite: 6 files and 83 tests passed, including merged/closed/queued terminal rows, targeted conflict reconciliation, and queue-row retention.
- `pnpm run typecheck`: passed.
- Targeted ESLint across the changed production, regression, helper, and E2E files: passed without warnings.
- Prettier check for the changed projection and regression files: passed.
- `pnpm run i18n:ratchet`: passed with zero added or modified copy violations; guard allowlist intact.
- Desktop Chromium sidebar spec: 3 tests passed, including merged details and keyboard reopen without a fetch.
- Mobile Chrome sidebar spec: 3 tests passed, including drawer content, focus return, and viewport checks.
- `python3 scripts/list-docs.py validate`: 339 decisions and 1,281 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed after recording results.
