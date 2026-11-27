---
id: "02-pr-delivery"
title: "Capture screenshots and deliver through merge"
status: in_progress
wave: 2
depends_on:
  - "01-phone-action-bar"
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-TASK-ACTION-BAR-001
  - REQ-UI-MOBILE-TASK-CHROME-001
acceptance_criteria:
  - AC-UI-MOBILE-TASK-CHROME-001.3
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.1
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.2
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.3
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.8
system_design:
  - ../../specs/ui/system-design/mobile-task-action-bar.md
  - ../../specs/ui/system-design/mobile-task-chrome.md
---

# Task 02: Capture screenshots and deliver through merge

## Summary

Capture the implemented bar from isolated fixture data and publish a PR with
embedded screenshots. Resolve actionable feedback, satisfy required checks,
and merge under the user's existing authorization.

## In scope

- Fresh phone/desktop screenshots from Task 01's managed headless fixtures.
- Screenshot inspection/compression, isolated orphan media ref, template-based
  PR body, Conventional Commit, push, review/CI remediation, merge evidence.
- Plan/work-order delivery evidence and final user report.

## Out of scope

- New UI design, unrelated cleanup, broad local verification, bypassing checks,
  persistent Kandev task/session creation, or delegated implementation.

## Acceptance

- Capture and inspect the real UI-01 and UI-02 common merged-PR state, plus a
  phone closed/dense state. Evidence shows notice separation, touch controls,
  composer, and surrounding phone navigation. Use synthetic data only.
- Publish compressed, nonempty images using `/pr`'s orphan-media procedure;
  verify the live PR body embeds SHA-pinned images and media binaries do not
  appear in the merge diff. Record screenshot file paths and media ref.
- Handle actionable reviews and failing CI in the primary conversation; rerun
  affected Task 01 checks after remediation. Merge only after required checks
  and reviewer gates pass, then record PR URL, merged head, and merge commit.

## ASCII UI preview

UI-04: Existing Tasks picker workspace recovery. See [full preview](plan.md#ui-04-existing-tasks-picker-workspace-recovery).

```text
Tasks sheet (modal owner) > workspace menu (non-modal)
Select workspace B > close surfaces > reopen Tasks picker
```

No visual geometry or navigation policy changes. One modal owner must release
pointer blocking after navigation. The existing mobile shared-context scenario
is the regression boundary for AC-UI-MOBILE-TASK-CHROME-001.3.

## Verification

Screenshot capture uses the already validated Task 01 fixtures. Add optional
capture through `testInfo.outputPath` to the relevant tests and use managed
headless commands below against the final code. If source changed, rebuild.

```bash
export PATH="$HOME/.local/share/mise/shims:$PATH"
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-chat-status-bar.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/chat-status-bar.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-shared-task-state.spec.ts tests/chat/mobile-chat-status-bar.spec.ts)
(cd apps/web && pnpm exec vitest run components/task/mobile/session-task-switcher-sheet.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/workspace-switcher.tsx e2e/tests/task/mobile-sidebar-shared-task-state.spec.ts)
git diff --check
git status --short
```

Load `/commit`, `/push`, `/pr`, and `/pr-fixup` at delivery. Read the full PR
template and screenshot validation/compression steps before publication. Reuse
an existing branch PR if present. Use `scripts/pr-await <PR>`,
`scripts/pr-state --summary <PR>`, and `scripts/pr-resolve list <PR>` for exact
head check/review evidence. Do not poll manually on a timer. Do not spawn a
monitor agent without the applicable explicit user authorization.

Verify the live PR body after writing via `--body-file`; use REST PATCH if
needed. Verify final merge state via `gh pr view <PR> --json
state,mergedAt,mergeCommit,url`. Resolve actual PR number before running
commands containing `<PR>`; record exact executed commands in Results.

## Files likely touched

- `docs/plans/mobile-task-action-bar/plan.md` and this work-order result.
- Task 01 E2E files only for optional screenshot capture if needed.
- Temporary `apps/web/.pr-assets/*.png` (never staged on the feature branch).
- Temporary PR body/evidence files outside the tracked diff.

- `apps/web/components/task/workspace-switcher.tsx`: non-modal nested menu.
- `apps/web/e2e/tests/task/mobile-sidebar-shared-task-state.spec.ts`: existing
  regression retained, with optional capture after successful recovery.
- `docs/specs/ui/system-design/mobile-task-chrome.md`: existing modal boundary.

## Dependencies

Task 01 done with all required targeted verification recorded.

## Risks

Unavailable capture/hosting, incomplete reviews, or required CI failures block
delivery. Name the actual blocker and preserve state. Media refs must survive
branch cleanup so embedded screenshots stay available.

## Parallelism

`sequential`

## Inputs

- [Validated work order](task-01-phone-action-bar.md) and [previews](plan.md#ascii-ui-preview).
- [Requirements](../../specs/ui/requirements/mobile-task-action-bar.md).
- [Design](../../specs/ui/system-design/mobile-task-action-bar.md).
- `.github/pull_request_template.md` and repository publication skills.
- User authorization: lead through PR with screenshots and merge.

## Results

Capture completed on 2026-10-12 using the isolated managed fixtures. The final
four-regression capture run passed; its manifest contains the intended fresh
PNGs: common phone, dense phone, translated closed notice, desktop, and the
coarse-tablet long-label state added during review remediation. All were
visually inspected and contain synthetic fixture data. PNG compression uses
`pnpm dlx pngquant-bin@9.0.0 --quality 65-90 --ext .png --force web/.pr-assets/*.png`
from `apps/`.

Implementation verification is complete in Task 01. Publication, exact-head
CI/review disposition, and merge are external completion gates. Their final
SHAs, PR URL, media ref, and merge evidence are recorded in the Kandev task plan
and user handoff after publication, avoiding a self-referential documentation
commit that restarts checks. No media binary belongs in the feature diff.

### Required-CI baseline repair

At head 0ed984e89f, 47 checks passed and 19 were skipped; only E2E Shard 3/14
and its aggregate failed. The existing phone shared-context reconnect test left
body pointer events disabled after workspace navigation closed the picker and
its nested workspace menu. It reproduced on this PR and on untouched main
acd873d41c. Logs: `/tmp/kandev-run.e2e.ASH6C2r2.log` and
`/tmp/kandev-run.e2e.GMZHBUfi.log`.

In an isolated baseline worktree, changing only the nested workspace dropdown
from a second modal to `modal={false}` passed the unchanged regression.
Log: `/tmp/kandev-run.e2e.hSCFFt3O.log`. This repairs the existing task-drawer
reachability contract, not a new workspace flow; the enclosing picker remains
modal. No assertions were removed or weakened. Final targeted validation and
current-head CI/merge evidence are recorded externally after the repair commit.

The repair passed all 7 mobile scenarios (3 shared-sidebar regressions and 4
action-bar regressions), 7 existing picker component tests, final typecheck,
zero-warning scoped lint, catalog validation, and specification lint. Fresh-build
log: `/tmp/kandev-run.e2e.6cEzy1RT.log`. The final post-commit capture also covers
the recovered Tasks picker, and its media/head evidence is recorded externally.
