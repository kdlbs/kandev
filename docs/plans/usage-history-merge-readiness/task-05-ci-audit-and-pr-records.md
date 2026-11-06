---
id: "05-ci-audit-and-pr-records"
title: "Audit CI and refresh PR descriptions"
status: blocked
wave: 5
depends_on:
  - "04-database-gate"
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-007
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-007.7
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 05: Audit CI and refresh PR descriptions

## Summary

Confirm exact-head CI has no flake in usage history code, then make both PR descriptions match the final state. The #3660 description currently names an old head and says CI is pending.

## In scope

- Run `scripts/pr-await 3660` after the last push. Download the `e2e-flake-history` artifact from the E2E run for that head, and read `flakyTests` for the entry whose `runId` matches.
- Apply this rule: fail if a flaky test touches `stats`, `usage`, plugins, or task deletion. List others in the PR body with the `main` baseline from the same file.
- Replace the marked validation sections in both PR bodies using `gh pr edit --body-file`, and verify the live body afterwards. Only change content between `kandev-usage-history-validation` markers in core and `kandev-g4-plugin-body` markers in the plugin. Add a short `Also included` heading in the core section that lists the unrelated fixes retained in #3660.
- State plainly: toggle off in `prod`; plugin 0.5.0 isn't to be published until the toggle is on; and forked or transferred native usage is removed on deletion with a gap notice.
- Mark work orders done and set this plan's status.

## Out of scope

- Fixing unrelated flaky tests.
- Merging. Report readiness to the user.

## Acceptance

- CI is terminal and green on the exact head, with zero unresolved threads from `scripts/pr-resolve list 3660`.
- The flake rule above passes, or the failing test is reported.
- Each PR body names its current head and has no stale pending claim.

## Verification

```bash
scripts/pr-state --summary 3660
scripts/pr-resolve list 3660
gh pr view 3660 --repo kdlbs/kandev --json headRefOid,mergeable,mergeStateStatus
gh pr view 10 --repo kdlbs/kandev-plugin-session-cost --json headRefOid,mergeable,mergeStateStatus
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `docs/plans/usage-history-merge-readiness/plan.md`
- `docs/plans/usage-history-merge-readiness/task-*.md`
- `docs/plans/usage-history-import/plan.md`

## Dependencies

Task 04.

## Risks

- A PR body edit re-triggers checks. Refresh the evidence afterwards.
- If `main` has moved, run `git merge-tree --write-tree origin/main HEAD` and report conflicts.

## Parallelism

`sequential`

## Inputs

- `.agents/skills/pr-fixup/SKILL.md`.

## Results

Blocked by Task 04: the `a39942a` SQLite fixture-projection run timed out, and later report SQL changed. No full-scale result for the updated source is recorded.

At core head `bf843362ec6124d9cb61941fe002dc412d5110e5`, `scripts/pr-await 3660` reported 91 passed, 13 skipped, and no failed or pending checks; `scripts/pr-state --compact 3660` and `scripts/pr-resolve list 3660` reported no unresolved threads. The `e2e-flake-history` artifact for run `41439159801` recorded one unrelated Threads composer disclosure flake in 4,579 executions. Its latest `main` baseline, run `41436181885`, recorded two flakes and one unexpected result in 4,565 executions.

The core PR's marked validation section was updated through REST after `gh pr edit --body-file` hit the Projects Classic warning. The live body was checked, including the `Also included` list and unchanged text outside the markers. The plugin PR body and final database-gate audit remain open. Refresh CI and review evidence after the final push.
