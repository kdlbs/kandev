---
id: "01-keep-pending-file-actions-visible"
title: "Keep pending file actions visible"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CHANGES-FILE-ACTION-FEEDBACK-001
acceptance_criteria:
  - AC-UI-CHANGES-FILE-ACTION-FEEDBACK-001.1
  - AC-UI-CHANGES-FILE-ACTION-FEEDBACK-001.2
  - AC-UI-CHANGES-FILE-ACTION-FEEDBACK-001.3
  - AC-UI-CHANGES-FILE-ACTION-FEEDBACK-001.4
  - AC-UI-CHANGES-FILE-ACTION-FEEDBACK-001.5
system_design:
  - ../../specs/ui/system-design/changes-file-action-feedback.md
---

# Task 01: Keep Pending File Actions Visible

## Summary

Keep the tree-mode stage and unstage spinner visible after a fine-pointer user
leaves the acted-on row. Prove the repair by holding each outgoing worktree
request at the browser WebSocket boundary before changing the shared row.

## In scope

- Add a Chromium regression for stage and unstage pending feedback.
- Pause only the armed worktree action frame and forward unrelated WebSocket
  traffic.
- Confirm the regression fails because the spinner becomes visually hidden
  after pointer leave.
- Make `isPending` override the fine-pointer idle hover swap in
  `TreeModeFileActionSlot`.
- Confirm only the acted-on file shows pending feedback and that completion
  moves it to the expected staged or unstaged section.
- Preserve the coarse-pointer branch, its always-visible action, and its 44px
  target.

## Out of scope

- Changes to Git execution, pending keys, status refresh, or error handling.
- Bulk, discard, edit, commit, or provider-file actions.
- New copy, loading primitives, notifications, cancellation, or progress
  estimates.
- Changes-panel layout or responsive composition changes.

## Acceptance

- During an in-flight stage or unstage request, the acted-on row's spinner is
  visibly rendered after the pointer leaves, and its file-type icon does not
  replace the spinner.
- Releasing each request completes the current Git flow and moves the file to
  the section matching its refreshed staged state.
- Idle fine-pointer hover behavior and coarse-pointer action reachability stay
  unchanged.

## Verification

Run from the repository root:

```bash
(cd apps/web && rtk pnpm e2e:run tests/git/git-changes-panel.spec.ts -- --grep "keeps stage and unstage progress visible after pointer leaves" --retries=0)
(cd apps/web && rtk pnpm exec eslint components/task/changes-panel-file-row.tsx e2e/tests/git/git-changes-panel.spec.ts)
(cd apps/web && rtk pnpm run typecheck)
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check -- apps/web docs/specs/ui docs/plans/changes-file-action-feedback
```

Before the production edit, run the exact Playwright scenario and record the
expected spinner-visibility failure. After the edit, rerun the scenario and
then the complete command set above.

## Files likely touched

- `apps/web/components/task/changes-panel-file-row.tsx`
- `apps/web/e2e/tests/git/git-changes-panel.spec.ts`
- `docs/plans/changes-file-action-feedback/plan.md`
- `docs/plans/changes-file-action-feedback/task-01-keep-pending-file-actions-visible.md`

## Dependencies

None.

## Risks

- Newline-delimited WebSocket messages can contain unrelated frames; the pause
  controller must forward them instead of delaying the full message.
- The original row can detach after status refresh, so post-completion
  assertions must use a locator scoped to the destination section.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-CHANGES-FILE-ACTION-FEEDBACK-001` and its system design.
- `TreeModeFileActionSlot`, `StageButton`, and the existing per-file pending
  state in `useSessionGit`.
- The current Chromium Changes-panel scenario and existing WebSocket
  request-pause helpers.

## Results

- RED: `pnpm e2e:run tests/git/git-changes-panel.spec.ts -- --grep "keeps stage and unstage progress visible after pointer leaves" --retries=0` failed on the expected rendered-state assertion. After pointer leave, the pending stage layer had computed opacity `0`.
- GREEN: the same production-build command passed after pending state was given
  precedence over the tree row's hover swap. The final formatted test passed
  again with `--no-build`.
- `changes-panel-file-row.test.tsx` passes 16 tests, including the existing
  coarse-pointer visibility and 44px target coverage.
- Targeted ESLint, `pnpm run typecheck`, `scripts/lint-spec-files.py --all`,
  and the scoped `git diff --check` command pass.
- No Pixel 5 test was added because the changed conditions execute only for
  fine pointers. The shared coarse-pointer branch and existing mobile
  stage/unstage path are unchanged.
