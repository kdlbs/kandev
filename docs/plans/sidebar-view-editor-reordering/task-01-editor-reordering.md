---
id: "01-editor-reordering"
title: "Implement compact editor reordering"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-VIEW-REORDER-001
acceptance_criteria:
  - AC-UI-SIDEBAR-VIEW-REORDER-001.1
  - AC-UI-SIDEBAR-VIEW-REORDER-001.2
  - AC-UI-SIDEBAR-VIEW-REORDER-001.3
  - AC-UI-SIDEBAR-VIEW-REORDER-001.4
  - AC-UI-SIDEBAR-VIEW-REORDER-001.5
  - AC-UI-SIDEBAR-VIEW-REORDER-001.6
  - AC-UI-SIDEBAR-VIEW-REORDER-001.7
  - AC-UI-SIDEBAR-VIEW-REORDER-001.8
system_design:
  - ../../specs/ui/system-design/sidebar-view-editor-reordering.md
---

# Task 01: Implement compact editor reordering

## Summary

Replace dedicated reorder arrows with sortable grips and compact explicit move menus.
Deliver the shared desktop/phone behavior and targeted regression evidence in one implementation pass.

## In scope

- Stable-ID Sort drag integration, arbitrary insertion, cancellation, context guards, and accessible announcements.
- Shared More menu, automatic-color arrow cleanup, and task-row menu integration.
- Existing selector/removal/visibility behavior and saved draft/settings paths.
- Localized copy, focused desktop/phone tests, and public how-to instructions.
- Update the current sort-chain system design's Editor section after implementation.

## Out of scope

Filter ordering, Group by options, editor section order, Threads direction buttons, task ranking, persistence changes, and new dependencies.

## Acceptance

- Desktop sort/color rows recover field width while grips and explicit menus move the correct complete item to the requested position.
- Phone and keyboard flows retain ordering, focus, input isolation, touch targets, containment, and save/reload behavior across all three editor lists.
- All assigned regression commands pass, with existing arrow-based scenarios migrated and existing assertions retained.

## ASCII UI preview

These excerpts use [UI-01, UI-02, and UI-03 from the full plan](plan.md#ascii-ui-preview), mapping to AC .1, .3, .4, .6, and .8.

```text
UI-01 Desktop:
[::] [Color v] [Red v] [Matching first v] [...] [X]

UI-02 Phone card:
| [::] Rule 2                    [...] [X] |
| [Color                                v]|
| [Red                                  v]|
| [Matching first                       v]|

UI-03 Details:
[::] Relative time                  [...] [On]

More menu: [Move up] [Move down]
```

Phone fields stack below a header of touch-sized actions. The existing drawer body remains the sole content scroller.
Desktop controls use 28px targets. Phone/coarse-pointer controls use at least 44px targets.
At one sort rule, reorder and removal are unavailable. Boundary menu actions are disabled.

## Verification

From the repository root, run these commands sequentially.
In a fresh worktree without dependencies, first run `(cd apps && pnpm install --frozen-lockfile)`.
Use `/tdd` and `/e2e`: run the new failing cases before production changes, then run the complete focused commands.

```bash
(cd apps/web && pnpm exec vitest run components/task/sidebar-filter/sort-chain-editor-model.test.ts components/task/sidebar-filter/sort-chain-editor.test.tsx components/task/sidebar-filter/sidebar-reorder-menu.test.tsx components/task/sidebar-filter/automatic-color-settings.test.tsx components/task/sidebar-filter/task-row-settings.test.tsx components/task/sidebar-filter/sidebar-filter-popover.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/sidebar-filter/)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/task/sidebar-running-first-activity-sort.spec.ts tests/task/sidebar-automatic-colors.spec.ts tests/task/sidebar-filter.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-running-first-activity-sort.spec.ts tests/task/mobile-sidebar-automatic-colors.spec.ts tests/task/mobile-sidebar-views.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Managed E2E rebuilds web/backend/fixtures. Keep runs sequential with the runner's worker limits.
Review captured desktop/phone renders against the previews and record command results and geometry evidence.

## Files likely touched

- `apps/web/components/task/sidebar-filter/sort-chain-editor.tsx`, `sort-chain-editor-model.ts`, `sort-chain-rule-card.tsx`.
- New `apps/web/components/task/sidebar-filter/sidebar-reorder-menu.tsx` and its test.
- `automatic-color-rule-card.tsx`, `automatic-color-rule-list.tsx`, `task-row-settings.tsx`, and `sidebar-filter-popover.tsx` in the same directory.
- Assigned component/model tests and the six E2E specs in the verification block, plus adjacent helpers if needed.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/task.json` (use the existing Traditional Chinese/pseudo generators).
- `docs/public/tasks-and-workflows.md`: short how-to text for grip and explicit menu ordering.
- `docs/specs/ui/system-design/sidebar-running-first-activity-sort.md`: replace its arrow-layout description with the implemented interaction link.
- This plan, work order, and paired requirement/design lifecycle fields.

## Dependencies

None. Existing sort-chain and color-settings implementations are already present.

## Risks

Nonadjacent swaps, unstable rule identity, stale-view drops, focus loss, and touch conflicts with drawer scrolling or dismissal.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/sidebar-view-editor-reordering.md), all acceptance criteria.
- [Design](../../specs/ui/system-design/sidebar-view-editor-reordering.md), all sections.
- Existing `AutomaticColorRuleList` and `SortableDetailRow` dnd implementations and their assigned tests.
- `/mobile-parity`, `/e2e`, `/tdd`, and `apps/web/AGENTS.md`.

## Results

Implemented stable-ID Sort dragging and shared More menus for Sort, automatic colors, and task-row details. Removed the redundant automatic-color arrows, added localized labels, updated the public how-to and linked system-design history, and added focused component plus desktop/mobile regression coverage.

- Focused Vitest: 6 files, 31 tests passed. Typecheck and scoped ESLint passed.
- `i18n:check` and `i18n:ratchet` passed across all seven shipped locales and pseudo locale. The check reported 435 pre-existing orphan catalog entries.
- Chromium E2E: 26/26 passed. Mobile Chrome E2E: 15/15 passed, including 390px fine-pointer and 900px coarse-pointer geometry, containment, and overflow checks.
- Desktop and phone captures were reviewed against UI-01/UI-02; task-row action geometry and ordering were verified by E2E.
- Public-doc tests: 62 passed; 47 pages validated. Specification catalog validation covered 359 decisions and 1,425 specifications; 36 spec-linter tests and the full spec lint passed.
- `git diff --check` passed. The implementation is tracked in PR #4295; the results below include its review follow-up.

### Code-review remediation

- Guarded each pointer/touch reorder with the visible list and scroll-region bounds. The detector keeps nearest-center movement for gaps and keyboard sorting and preserves initial bounds during autoscroll. At drop, each editor validates the latest pointer position against the current visible list bounds, maps rejected drops to cancellation, and announces cancellation.
- Marked each phone drag grip `data-vaul-no-drag`, preventing Vaul from handling the same downward gesture while retaining drawer scrolling and dismissal elsewhere.
- Added a collision-unit regression for a list that moves during a drag, plus mobile E2E coverage for releasing in the stale starting bounds. Existing mobile checks cover outside drops on Sort, automatic colors, and task-row details. The color case asserts no PATCH and unchanged saved settings. Each section also tests a downward touch reorder with a stationary, open drawer.
- Renamed the sidebar-specific scroll helper, corrected the Traditional Chinese translation for “item”, and guaranteed cleanup in the clipped-scroll unit test.
- Final verification: 32 focused component tests passed; typecheck, scoped ESLint, and `build:e2e` passed. Chromium E2E passed 26/26 and mobile Chrome E2E passed 15/15. `git diff --check` passed.
- Review-follow-up verification: 38 focused component tests passed; typecheck, scoped ESLint, `i18n:check`, `i18n:ratchet`, and `git diff --check` passed. Managed Chromium E2E passed 26/26 and managed mobile Chrome E2E passed 15/15, including the stale-bounds regression.

### CI E2E failure remediation

- The failed CI attempt reported nine retry verdicts across normal/container shards and three E2E shard jobs that had no runner or test steps. An exact no-retry replay of normal shard 12 on its CI merge commit passed 273 tests and reproduced the MCP subtask test failure: a persisted collapsed sidebar placed its resize divider over the New Task button. The test now sets that collapsed state deterministically, expands it before clicking, and restores the original layout.
- The Git refresh wait now matches the task session instead of any held request. Mobile file tests wait for the task environment to become ready before opening the Files panel. The repository-set test verifies the option's actual selection instead of hit-testing its pixel edges. The workflow touch helper can continue to the boundary for up to 40 gestures. The Docker source test uses the file-tree helper that reveals virtualized rows. The setup-recovery test waits for the seeded FAILED state after reload before checking its persisted controls.
- Managed E2E checks passed with retries disabled: all four targeted desktop cases passed; the collapsed-sidebar subtask case passed three consecutive runs; the mobile file/HTML cases passed once and then twice each; the touch-scroll case passed once and then three consecutive runs; and the Docker workspace-source case passed in host-mode managed E2E. The container-mode attempt stopped before test setup because this shared worktree's `.git` pointer targets a path outside the runner's mount.
- Final follow-up typecheck, ESLint on all changed E2E files, and `git diff --check` passed. The CI rerun and review disposition are tracked by PR #4295.

### CI rerun follow-up

- The exact-head run on `72a5735e1d2727ac7bb1b8e00e1c8d5adb57bf06` failed in E2E shard 7/14. One test failed on all three Playwright attempts; 239 tests passed and 3 were skipped. The report merge and required E2E aggregate failures were downstream of that shard.
- The failure was in an unchanged desktop session-continuity test. Its downloaded error context showed that the automatic continuation response was already visible after reload, while the test still required the transient retry card. The exact test passed once in the local managed runner before the correction, so that run alone did not reproduce the CI timing.
- The desktop and phone checks now accept either the still-pending retry card or the completed continuation at the reload and second-viewer checkpoints. They retain the initial retry-card assertion and final same-runtime, single-side-effect, and successful-continuation checks.
- After the correction, the focused desktop case passed 3/3 and the matching phone case passed 3/3 in managed E2E with retries disabled. Scoped ESLint, Prettier, and `git diff --check` passed.
- Main advanced to `a8bfce19fc299a4243af4c7be63c32e1a14dc7d5` during the CI rerun and caused import-only conflicts in the desktop and mobile sidebar running-rank E2E specs and their shared helper. The merge keeps both the drag-reorder coverage and main's task-wide running-rank coverage.
- After merging that base, the managed desktop and mobile sidebar running-rank specs both passed locally (2/2 each). Exact-head CI and its retry-summary audit are pending on the new merge commit for PR #4295.

### CI no-flake remediation

- The exact-head CI run on `7c8bb040e6ec2a23611b4a4cbfe50ec76a8e87bf` passed overall but its retry summary reported seven flaky tests. A retries-disabled local replay reproduced the weak sort-grip activation and the task-title/workflow setup timeouts. The replay also confirmed that the slow-fetch test could begin its 19-second retry-budget dwell before Git reached `fetch`.
- The slow-fetch scenario now gates the actual `git fetch`, waits for its start marker, checks the waiting UI through the retry budget, and releases the fetch in cleanup. Session-resume seeds stale activity only after the task session list is connected and settled. Task-title and workflow tests expand navigation when the disclosure is rendered and restore the prior workspace layout.
- Workflow-preview checks now await response-body completion, wait for the first rendered step, and inspect workflow groups in sequence. The sort drag uses a short vertical activation move contained within the grip. Mobile diff swipes wait for finite drawer animation and a visible scroll region. WebSocket response waits can arm immediately but start their timeout after a dependent operation; the runtime-notification test uses this around backend restart.
- Final managed E2E checks with retries disabled passed: desktop affected specs 15/15; the desktop sidebar sort scenario 3/3 across a separate repeat run; the phone Pierre-diff and runtime-reconnect cases 2/2. The causal-waits unit suite passed 29/29.
- Typecheck, ESLint on all changed E2E files, Prettier, `i18n:check`, `i18n:ratchet`, and `build:e2e` passed. The i18n check reported the existing 435 orphan catalog entries. `git diff --check` passed. Exact-head PR CI and the zero-retry audit are pending after this fixup push.

### CI zero-retry follow-up

- The phone downward sort regression now holds the drawer stationary and verifies the saved order after a touch drag from the top rule. The test scopes its target to the source sortable list, waits for list transitions and sync completion, and uses the More menu for longer moves across the scrolled list. The shared touch helper yields a frame between move events so dnd-kit processes each location before release.
- The phone sort scenario passed five consecutive runs with retries disabled, then passed once after the final sort-sync helper refactor. Mobile automatic-color and sidebar-view E2E passed 14/14; desktop sidebar-sort E2E passed 2/2.
- The six focused sidebar component suites passed 31/31. The clean embedded E2E build, plugin fixture package build, web typecheck, changed-file ESLint, `i18n:check`, `i18n:ratchet`, Prettier, and `git diff --check` passed. The i18n check reports the same 435 existing orphan catalog entries.
- Before this update, PR #4295 at `84336672951e35d11aad1036c026e6db41ec8ff5` had 53 passing and 17 skipped checks, with no failed checks or unresolved review threads. The updated head's exact CI and retry-summary audit remain pending after the fixup push.

### CI failure remediation follow-up

- CI at `6d2ac464789b1e91eab568344ea3926eb86235d6` exposed two independent test failures. The lifecycle SSH cleanup test showed that `StopInstance` continued to call the remote stop command after the cleanup backstop had marked the client transport lost. It now rechecks transport loss before sending that command; the regression test asserts zero follow-up stop calls. The test failed before the guard with `stop calls = 1` and passed after it.
- The Quick Chat storage test exposed unstable ordering when `Date.now()` advanced by more than one millisecond between entries. Persistence now captures one timestamp for the ordered batch, so elapsed write time cannot reorder the selections. A deterministic clock-advance regression failed before the change (`workspace-5` first instead of `workspace-204`) and passed after it.
- Local verification after these changes: the lifecycle package passed with `-race` and CI-equivalent atomic coverage (78.6%); the focused SSH regression passed 100 consecutive race-enabled runs; `selection-storage.test.ts` passed 4/4 and then passed in 10 consecutive invocations (40 tests). Frontend typecheck, changed-file ESLint, backend changed-code lint, Prettier, and `git diff --check` passed. The exact-head PR retry audit is pending.

### CI retry-only E2E remediation follow-up

- The exact-head CI run passed overall but reported four tests that passed only after Playwright retry: diff expand-all, hidden-workflow task creation, hidden-directory listing, and mobile selected-first repository filters.
- Diff expansion now waits for the control to report its expanded state before checking the newly visible lines. The task-creation test explicitly seeds the persisted collapsed-sidebar layout, expands it before opening the dialog, and restores the workspace's original layout. The directory-browser helper waits for session and dockview readiness, selects the Files panel, and confirms its content before opening workspace actions.
- The selected-first filter fixture now includes a provider-backed repository and derives expected filter values from the canonical repository slug, matching the value shown by the UI. Assertions verify selected-first grouping without depending on ordering within each group.
- Retries-disabled managed E2E passed: mobile selected-first 3/3, desktop selected-first 3/3, diff expand-all 3/3, collapsed-sidebar task creation 3/3, and the complete hidden-directory browser file 8/8. No local retry was used.
- Typecheck, scoped ESLint on all changed E2E files, Prettier, and `git diff --check` passed. The exact-head PR retry-summary audit remains pending after this fixup push.

### CI zero-retry E2E remediation follow-up

- The exact-head CI run at `c76b79013ba69d0e271dfa379f68382dd3fec915` passed required checks but its retry summary reported five flaky E2E tests: initial Changes refresh, completed-workspace restoration, control sizing, MCP subtask creation, and mobile hidden-directory reveal.
- The Changes assertion now distinguishes a pending first membership snapshot from a valid ready snapshot for that same session. Workspace restoration has a six-minute test budget for slow backend restart and recovery. Control sizing expands navigation only when its disclosure exists and waits for the task dialog rather than an unrelated, potentially preloaded request. The MCP subtask case checks its API parent relationship and nested sidebar row instead of depending on the current Kanban filter, and uses a unique title.
- Both mobile hidden-directory tests wait for the mock session to finish workspace setup and for the session page to load before opening Files. The desktop dockview readiness gate was removed because mobile does not expose that API.
- Retries-disabled managed E2E passed: the four targeted desktop cases passed three consecutive runs each (12/12) with:
  `pnpm e2e:run --host --no-build --shards 1 --project chromium --retries=0 --repeat-each=3 tests/git/changes-panel-refresh-recovery.spec.ts tests/session/completed-workspace-restoration.spec.ts tests/task/control-sizing.spec.ts tests/task/subtask.spec.ts -g "keeps initial pending feedback|restores a cold workspace and recovers a bounded failure|task creation keeps touch sizing until the md breakpoint|agent creates subtask via MCP create_task with parent_id"`.
  Both mobile hidden-directory cases passed three consecutive runs each (6/6) with:
  `pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome --retries=0 --repeat-each=3 tests/task/mobile-directory-browser-hidden-folders.spec.ts`.
- Static checks passed from `apps/web`: `pnpm run typecheck`; `pnpm exec eslint e2e/tests/git/changes-panel-refresh-recovery.spec.ts e2e/tests/git/git-status-refresh-helpers.ts e2e/tests/session/completed-workspace-restoration.spec.ts e2e/tests/task/control-sizing.spec.ts e2e/tests/task/mobile-directory-browser-hidden-folders.spec.ts e2e/tests/task/subtask.spec.ts`; `pnpm exec prettier --check` with the same six paths; and `git diff --check`.
- The exact-head PR CI and retry-summary audit are pending after this remediation push.

### Windows test-fixture cleanup remediation

- Exact-head CI at `0ef5dc7ba742a2d4a74126c4d61aeefdce2c2420` failed in `TestReconcileRepositories_PrunesRemovedTrackerAndPreservesSubscription` on Windows while removing the simulated rolled-back repository. The test deleted the tracker working directory before stopping its Git tracker; Windows can reject removal while a process still uses that directory. The aggregate backend test failure was downstream of this job.
- The fixture now stops the stale tracker before deleting its directory. Reconciliation still performs the pruning and subscription-detachment assertions under test.
- Local verification passed: the focused test with `-race -count=3`, the full process package with `-race`, Windows amd64 test-binary cross-compilation, `golangci-lint run ./... --new-from-rev="a8bfce19fc299a4243af4c7be63c32e1a14dc7d5" --timeout=5m`, and `git diff --check`. The Linux runner does not reproduce Windows directory-lock semantics.
- Exact-head CI and the E2E retry-summary audit are pending after this remediation push.

### Post-merge E2E retry remediation

- PR #4295 merged at `1769b9abafea5c45c0798409e33dbaa6236d0f3b`. Its final head `08d135673deb4a504c8a460bfc079a41b7de8eb9` passed CI, but run `37751538803` recorded nine flaky verdicts: one preview session-tab case, six port-forward cases, one task-create tooltip case, and one mobile file-tree case. These test-only corrections are delivered in a separate follow-up PR based on the merged main branch.
- Port-forward fixtures prepare the session, subscribe the browser, then start the agent, so readiness cannot precede the browser subscription. The remote executor default is reset in `finally`. Preview opening waits for the board's persisted primary-session projection. Tooltip cases expand task navigation and restore the prior sidebar layout. Mobile fixtures push their commit to the remote before task creation, verify the files in the task worktree and reveal the virtualized directory row.
- Local managed E2E uses one worker and no retries. The full port-forward spec passed 17/17. Sidebar customization followed by tooltip coverage passed 9/9; the corrected preview session-tab case passed three consecutive runs. Both mobile file-tree tests passed three consecutive runs each (6/6). The follow-up exact-head CI/retry audit remains pending.
- Web typecheck, scoped ESLint, the managed E2E build, Prettier, and whitespace checks passed. No production behavior or public documentation changes are required.

### Follow-up PR #4339 CI remediation

- At head `fe011f496fe8fbc278844a2d640739f4139ab3de`, CI run `37763747719` passed frontend/backend checks but failed E2E shards 2 and 10 and their aggregate gates. All 20 blob reports were audited with `scripts/playwright-blob-audit`; the retry summary confirmed 3,883 first-attempt passes, three flaky verdicts, two failures, and 47 skips. The original nine retry cases passed on their first attempts.
- New findings: diff expansion lost its control state when equivalent absent comparison targets changed representation; remote-repository creation inherited collapsed navigation; plugin navigation measured a 44px target as 43.99993896484375px; mobile settings recovery measured buttons during a temporary absent projection; delayed entry overlapped mock-agent startup. Fixtures now wait for a settled conversation/turn, expand and restore navigation, and await visible touch targets with direct browser DOM rectangle measurements that retain the strict 44px minimum. Diff and remote-repository suites explicitly disable retries. A constrained replay and pointer/store instrumentation proved that the comparison target changed from `null` to `""` without changing the environment, checkout, branch, HEAD, or base. That changed the serialized display-scope key and remounted the file diff. The shared hook now normalizes absent comparison targets in that key; genuine comparison-target changes still invalidate the scope. The hook regression failed before the fix and passed afterward (3/3 suite tests). This restores the existing [workspace Git status display contract](../../specs/platform/system-design/workspace-git-status.md#frontend-and-mobile), without changing its API or persistence contract.
- Local reproduction: the full session-entry spec passed 4/4 in isolation. Archived normal shard 2 was replayed in `ghcr.io/kdlbs/kandev-ci:runtime-latest` with one worker, two CPUs, 4 GiB memory/swap, and `--retries=0`: 274 passed, three skipped, and one worktree-recovery flaky verdict. That spec's explicit retry setting overrode the command-line value. Its second Resume attempt accepted the previous error before the new response; the test now correlates and awaits that response before replacement, and explicitly disables retries.
- Desktop managed verification passed 24/24 (eight targeted cases repeated three times) with one worker and retries disabled. This includes sidebar collapse immediately before remote-repository loading. Both mobile remediation cases passed three repeats each (6/6) with strict 44px DOM rectangle assertions and retries disabled. After the scope-key fix and a fresh `build:e2e`, constrained-runtime verification passed 9/9 (diff expansion, delayed entry, and worktree recovery each repeated three times) with one worker, two CPUs, 4 GiB memory/swap, and retries disabled. Command: `bash e2e/scripts/run-raw-e2e.sh --project=chromium e2e/tests/session/session-resume-recovery.spec.ts e2e/tests/session/session-entry-recovery.spec.ts e2e/tests/git/diff-expansion.spec.ts --grep="keeps normal resume|completes entry when|expand-all button" --repeat-each=3 --retries=0 --reporter=list` inside the CI runtime container. Web typecheck, scoped ESLint, Prettier, the fresh E2E build, and whitespace checks passed. The new exact-head CI/retry audit remains pending.
- Both automated review threads on the first follow-up head were dispositioned and resolved: a test-local type comment was optional; the proposed `finally` error path was invalid because the initial workspace update occurs outside the `try`. The scope-key fix is shared state normalization with no layout, scrolling, navigation, or touch changes; the focused hook regression satisfies mobile parity for this narrow case. Public documentation does not change.


### Follow-up PR #4339 retry-summary remediation

- Head `1d2e36197687b14a34217c2fbf777375daaae059` passed all 50 checks, with 16 skipped, zero failed/pending checks, and no unresolved or hidden review threads. E2E run `37775716968` nevertheless recorded five retry-only failures. The full 20-report blob audit agreed with the merged retry summary: 3,915 first-attempt passes, five flaky verdicts, zero unexpected outcomes, and 47 skips. The earlier remediation cases passed on their first attempts. Fixup remains open until the next code head's retry audit is clean.
- Cleanup failed while a session transfer held its durable mutation guard. A regression reproduced immediate reset failure on `ErrSessionTransferInProgress`. The test-only reset endpoint now waits for that typed guard to settle within its existing 30-second cleanup budget, preserves unrelated failures, and honors cancellation. The new guard/cancellation regressions failed before the fix and passed afterward. Focused E2E reset tests passed three race-enabled runs; backend build and changed-package lint passed.
- Creation auto-focus reproduced locally after the persisted zero-height navigation test. The helper expands navigation, waits for the visible dialog instead of requiring a new repository HTTP response, retains submit-eligibility assertions, and restores the original layout. The cancel-dialog case opens its seeded task directly, so an unrelated board filter cannot hide the target.
- The idle-session fixture now waits for the normal agent response and terminal turn state in persistence before navigation and follow-up prompts. Shared first-response coverage warms the complete active view before opening the incomplete archived view and counts only that archived query; bootstrap queries for the active view do not belong to this read. The original one-request assertion during three invalidations and two-request assertion after settlement remain intact on desktop and phone.
- Desktop verification passed 18/18 (six targeted cases, including collapse before creation, repeated three times) with one worker and retries disabled. Phone verification passed 9/9: transient retry, shared first-response reads, and creation auto-focus each passed three runs. The CI runtime container (one worker, two CPUs, 4 GiB memory/swap) passed 12/12: collapse before creation, transient retry, and shared first-response coverage repeated three times with retries disabled. Commands: `pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome e2e/tests/session/mobile-transient-retry.spec.ts e2e/tests/task/mobile-sidebar-shared-task-state.spec.ts -g "yellow retry card|phone accepts safe first rows" --repeat-each=3 --retries=0`; `pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome e2e/tests/task/mobile-creation-auto-focus.spec.ts --repeat-each=3 --retries=0`; constrained `bash e2e/scripts/run-raw-e2e.sh --project=chromium e2e/tests/settings/sidebar-direct-customization.spec.ts e2e/tests/session/transient-retry.spec.ts e2e/tests/task/creation-auto-focus.spec.ts e2e/tests/task/sidebar-shared-task-state.spec.ts --grep="collapses every navigation entry|shows the yellow retrying card|saves auto-focus|desktop accepts safe first rows" --repeat-each=3 --retries=0 --reporter=list`. Web typecheck, scoped ESLint, Prettier, and whitespace checks passed. New-head CI and its retry audit follow the push. Source layout, copy, navigation contracts, and public documentation do not change.


### Follow-up PR #4339 chart retry remediation

- E2E run `37796670933` at `0eaf1b61538785942577a88594e6e9f66de5adba` exposed a retry-only rich-output failure. Its line geometry was complete, but the test required an animation-only `stroke-dasharray` attribute. Reduced motion reproduced that exact failure locally. The test now checks the stable SVG path and retains separate animation-opt-out coverage.
- Removing that timing dependency exposed a lazy-plot defect. A real browser observer delivered non-intersecting and intersecting observations in one batch; reading only its first entry lost mount eligibility. The hook now accepts any intersecting entry, preserving the existing visible-near-viewport, mount-once contract. The batched-observation regression failed before the fix and passed afterward. No product contract, layout, copy, or public documentation changes.
- Verification: the chart component and motion suites passed 11/11. Fresh-build desktop E2E passed 6/6, mobile rich-output E2E passed 3/3, and the CI runtime with one worker, two CPUs, and 4 GiB memory/swap passed 6/6, all with retries disabled. Reduced-motion geometry also passed three local runs. Commands: `pnpm exec vitest run components/task/chat/messages/kandev/rich-output/chart-block.test.tsx components/task/chat/messages/kandev/rich-output/chart-motion.test.tsx`; `pnpm e2e:run --host --shards 1 --project chromium e2e/tests/chat/rich-output.spec.ts --repeat-each=3 --retries=0`; `pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome e2e/tests/chat/mobile-rich-output.spec.ts --repeat-each=3 --retries=0`; constrained `bash e2e/scripts/run-raw-e2e.sh --project=chromium e2e/tests/chat/rich-output.spec.ts --repeat-each=3 --retries=0 --reporter=list`. Typecheck, scoped ESLint, Prettier, and whitespace checks passed. The new code head still requires a complete CI and retry audit.


### Follow-up PR #4339 dependency-selector retry remediation

- The completed 20-report audit for head `0eaf1b61538785942577a88594e6e9f66de5adba` and E2E run `37796670933` recorded exactly two retry-only failures: rich output and task-create dependency selection. The merged summary agreed: 3,920 first-attempt passes, two retry passes, zero unexpected outcomes, and 47 skips. All earlier remediated cases passed on their first attempts.
- The dependency selector reproduced after the persisted zero-height sidebar scenario: the navigation divider intercepted the New Task click for the entire test budget. Its dialog-opening cases now expand navigation through the existing page-object helper. Before/after hooks capture and restore the workspace layout for every case, preserving isolation without changing product behavior or increasing timeouts.
- The complete dependency selector file, preceded by the sidebar-collapse case, passed three repetitions: 15/15 with retries disabled. The constrained CI runtime (one worker, two CPUs, 4 GiB memory/swap) passed the reported selector case after collapse three times: 6/6. Commands: `pnpm e2e:run --host --no-build --shards 1 --project chromium e2e/tests/settings/sidebar-direct-customization.spec.ts e2e/tests/task/create-task-dependency-selector.spec.ts -g "collapses every navigation entry|Task-create dependency selector" --repeat-each=3 --retries=0`; constrained `bash e2e/scripts/run-raw-e2e.sh --project=chromium e2e/tests/settings/sidebar-direct-customization.spec.ts e2e/tests/task/create-task-dependency-selector.spec.ts --grep="collapses every navigation entry|selects, clears, and persists multiple" --repeat-each=3 --retries=0 --reporter=list`. Typecheck, scoped lint, formatting, and whitespace checks passed. The new pushed head requires a fresh full CI/retry audit.


### Follow-up PR #4339 workspace readiness and panel recovery

- The full 20-report audit of E2E run `37810847369` at `60b52f04885234ae451e12da05e8621926675b6a` found six retry-only failures. The merged summary agrees: 3,917 first-attempt passes, six retry passes, zero unexpected final outcomes, and 47 skips. Aggregate green checks do not satisfy the requested zero-flake gate.
- The mobile Markdown fixture now waits for its initial response and completed turn before opening the workspace, then uses the Files page object's readiness check. CI showed workspace preparation rather than a ready file tree. Both phone cases passed five repetitions on the host and five in the constrained CI runtime (10/10 each, retries disabled).
- The LSP search fallback now recovers if a late Dockview activation switches away from Files. The backend-restart regression places its target outside the virtualized tree and dispatches that late panel activation after search opens. It reproduced the missing search input before the helper change and passed three repetitions afterward.
- The shared per-test fixture now resets the workspace sidebar layout with its current revision. The sidebar-collapse case followed by local-repository creation reproduced CI's intercepted New Task click before the reset. Both complete suites passed three repetitions afterward (27/27), including layout persistence across reloads within a test. This also covers the same leaked layout identified in the mobile prompt-chip suite.
- Commands: `pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome e2e/tests/chat/mobile-markdown-math.spec.ts --repeat-each=5 --retries=0`; `pnpm e2e:run --host --no-build --shards 1 --project chromium e2e/tests/lsp/lsp-file-intelligence.spec.ts -g "disconnects the old lease and starts fresh analysis after backend restart" --repeat-each=3 --retries=0`; `pnpm e2e:run --host --no-build --shards 1 --project chromium e2e/tests/settings/sidebar-direct-customization.spec.ts e2e/tests/task/create-task-new-local-repository.spec.ts --repeat-each=3 --retries=0`. Scoped ESLint, Prettier, web typecheck, and whitespace checks passed. Mobile branch-picker and transcript-loading findings remain under investigation; the next pushed head still requires a complete retry audit.


### Follow-up PR #4339 silent refresh and overlay settlement

- Integrated main at `56cc19514e20b1c78c366005a9358ba9a8857393` without conflicts to reproduce the newer transcript regression against its actual implementation. The phone test failed all ten baseline runs. A probe showed that monitoring began with `session-history-loading` mounted and the initial history read still in flight. The observer now waits for initialized, idle history and removal of loading rows before monitoring the recovery operation; loading rows during recovery still fail the test.
- The focused background-refresh unit regression also exposed an initial-wait flag being raised during a silent read. That assertion failed before the fix. Silent reads now leave the initial-wait flag off, while reads with no transcript retain visible loading. The complete session-fetch, session-message, diff-scope, and chart suites passed 55/55.
- The mobile repository-set test now waits for the Add Repository popover itself to unmount before opening the branch selector. This settles the prior overlay rather than assuming that disappearance of its selected option proves completion. The original CI dropdown failure did not reproduce in 15 host and 15 constrained baseline runs; the updated full phone batch verifies the explicit transition as well as touch scrolling, contained drawer controls, and branch refresh.
- Final integrated replay: desktop passed 12/12 (collapse before local-repository creation, virtualized LSP search after restart, and silent transcript recovery, each repeated three times). The CI runtime container with one worker, two CPUs, and 4 GiB memory/swap passed 18/18 (phone transcript recovery, all repository-set cases, and both prompt-chip cases, each repeated three times). All runs disabled retries. Phone transcript recovery also passed five host runs, and both prompt-chip cases passed five repetitions (10/10).
- Commands: `pnpm e2e:run --host --no-build --shards 1 --project chromium e2e/tests/settings/sidebar-direct-customization.spec.ts e2e/tests/task/create-task-new-local-repository.spec.ts e2e/tests/lsp/lsp-file-intelligence.spec.ts e2e/tests/chat/turn-end-history-refresh.spec.ts -g "collapses every navigation entry|refreshes and creates from a second repository row|disconnects the old lease and starts fresh analysis after backend restart|conversation gap found" --repeat-each=3 --retries=0`; constrained `bash e2e/scripts/run-raw-e2e.sh --project=mobile-chrome e2e/tests/chat/mobile-turn-end-history-refresh.spec.ts e2e/tests/settings/mobile-workspace-repository-sets.spec.ts e2e/tests/task/mobile-task-create-prompt-chips.spec.ts --repeat-each=3 --retries=0 --reporter=list`. Fresh web/backend E2E builds, web typecheck, scoped ESLint, Prettier, whitespace checks, and the full `make -C apps/backend test` suite passed. These fixes preserve existing product contracts and introduce no public documentation or localization changes. The next pushed head requires a fresh complete CI/retry audit.


### Follow-up PR #4339 quorum fixture isolation

- The full 20-report audit of E2E run `37825281613` at `32f683019011d08a405b5be963e1b00d833b67e9` found one retry-only failure: the advance-on-approve Office quorum case never received its automatic Review participant seat. The merged summary agrees: 3,927 first-attempt passes, one retry pass, zero unexpected final outcomes, and 47 skips. All prior remediated cases passed on their first attempts.
- Before preparing a session, that case had no task-runner fallback and relied on the worker-shared CEO as its only eligible automatic reviewer. Its manually registered reviewer and approver have worker roles, which automatic casting deliberately excludes. Stopping the shared CEO reproduced the exact missing-seat timeout locally. The ordinary focused baseline passed 3/3, and the preceding Office files plus quorum cases passed 36/36 under CI resource limits, so the CI attempt's actual CEO status is not directly established by the available artifacts.
- The case now explicitly stops the shared CEO and creates an eligible specialist before entering Review. Its separate worker still claims the automatically cast seat, and the existing single-seat, one-decision quorum, Approval, and Done assertions remain intact. This isolates the test's casting prerequisite and permanently covers the unavailable shared agent state without altering production eligibility, increasing timeouts, or changing public documentation.
- Verification: the constrained CI runtime (one worker, two CPUs, 4 GiB memory/swap) passed 36/36 with retries disabled: `bash e2e/scripts/run-raw-e2e.sh --project=chromium e2e/tests/office/dashboard-agent-cards.spec.ts e2e/tests/office/regression-fixes.spec.ts e2e/tests/office/runtime-skills.spec.ts e2e/tests/office/workflow-quorum-transitions.spec.ts --repeat-each=3 --retries=0 --reporter=list`. Both complete quorum paths and their preceding suites ran three times. Web typecheck, scoped ESLint, Prettier, and whitespace checks passed. The next pushed head requires a fresh full CI/retry audit.

### Follow-up PR #4339 asynchronous fixture readiness

- All 20 blob reports from E2E run `37835205841` at `5624fb47ae1c3dc1f5fbcfa7409f0af9028d01f9` were audited: 3,928 passing attempts, 47 skips, three retry attempts, two first-attempt failures, and one first-attempt timeout. There were no parse errors or additional failing cases. The Office quorum case passed on its first attempt. Green job conclusions do not satisfy the no-retries delivery gate.
- The mobile source-attachment failure captured `Preparing workspace...` instead of the Files controls. A primary executor binding and an initially idle chat do not prove that the first workspace launch completed. The fixture now waits for its actual mock response and completed session turn before navigating; all touch-target, source-attachment, and persistence assertions remain intact.
- The initial Git toolbar case held a refresh request while an independent ready Git notification could remove its loading indicator between visibility and geometry assertions. The case now uses the existing real Git enrichment gate to keep the producer pending until the toolbar is measured. It retains the request/response checks, strengthens the empty-state assertion, and verifies that releasing enrichment clears the indicator without changing toolbar height.
- Plugin UI registration after reload can follow the built-in status items. The saved-order assertion now polls for the complete original order after reload and re-enable rather than sampling an intermediate DOM. The control isolation, disabled-state removal, saved ordering, and interaction reset assertions remain intact. No production behavior, timeout, public documentation, or localization contract changed.
- Verification: one worker, two CPUs, and 4 GiB memory/swap in the CI runtime image passed all 12 desktop runs of `bash e2e/scripts/run-raw-e2e.sh --project=chromium e2e/tests/git/changes-panel-refresh-recovery.spec.ts e2e/tests/plugins/plugin-action-compatibility.spec.ts --repeat-each=3 --retries=0 --reporter=list`, and all five phone runs of `bash e2e/scripts/run-raw-e2e.sh --project=mobile-chrome e2e/tests/task/mobile-add-workspace-sources.spec.ts --repeat-each=5 --retries=0 --reporter=list`. Web typecheck, scoped ESLint, Prettier, and whitespace checks passed. The new pushed head requires a fresh complete CI and retry audit.

### Follow-up PR #4339 daily scheduler fixture boundary

- Backend run `37852960952` at `1d978d72ed17b7da688d2021441c761fae4a1771` failed `TestCronScheduler_DailyTrigger_DoesNotRefireNextTick` at 23:59 UTC. Its one-minute probe crossed midnight, when `@daily` is correctly due again. A fixed 23:59:30 evaluation reproduced the former assertion failure locally under the race detector.
- The existing integration case still verifies that a concurrency-cap skip persists its evaluation. It now checks that the trigger remains suppressed immediately before the next UTC midnight and becomes due at midnight. These assertions preserve the daily scheduling contract without depending on the test's wall-clock execution time or changing production behavior.
- Verification: `go test -trimpath -race ./internal/automation -count=10` passed the complete automation package ten times. Backend lint against live-base SHA `41dc8f79d304d3255d4448eba6e48c23f6982a5f` reported zero issues; whitespace checks passed. Fresh complete CI and retry evidence remains required after pushing this test repair.

### Follow-up PR #4339 file-editor transition

- The partial 13-report audit of E2E run `37863662121` at `250a003bdaa969ad740c932bc641c74571c33106` found one retry-only timeout in the user edit/save diff case. The row became visible, then a late Changes activation hid Files before the subsequent click. The failure screenshot confirms Changes active; the click log records a hidden, detached row. This is a tab-focus race, not evidence of a virtualization defect.
- Activating the existing Changes panel between the visible-row probe and file click reproduced the hidden-row timeout locally. The former case's explicit describe retry configuration overrode the CLI's zero-retry setting, so that controlled failure ran three failed attempts. The repaired case passed the same one-shot late activation with no retry, then completed all original edit, save, diff-update, and stale-content assertions.
- Tab activation, file click, and visible editor confirmation now form one bounded retried transition. File clicks have a short attempt timeout so the fixture can reopen Files when a late update activates Changes. The file's explicit retry overrides were removed; CI retains its standard configured policy, while local `--retries=0` now applies to the complete file.
- Verification: the CI runtime image with one worker, two CPUs, and 4 GiB memory/swap passed all 27 runs of `bash e2e/scripts/run-raw-e2e.sh --project=chromium e2e/tests/git/diff-update.spec.ts --repeat-each=3 --retries=0 --reporter=list`. Web typecheck, scoped ESLint, Prettier, and whitespace checks passed. Remaining current-head reports must still be audited, and the next pushed head requires fresh complete CI evidence.

### Follow-up PR #4339 command-reveal cue observation

- The 18-report audit of E2E run `37863662121` at `250a003bdaa969ad740c932bc641c74571c33106` found a second retry-only failure: the command-selected task was active and visible, but its transient reveal class was absent when asserted after URL and active-row checks. The cue lasts 1,400 ms. Waiting for the actual cue to appear and clear reproduced the same failed class assertion locally with retries disabled.
- Both direct command-selection cases now install a DOM class observer before selection and retain the observed reveal cue until asserted. The observer records the target task's actual class mutation, including the old class value when addition and removal share a mutation batch. It disconnects on observation and is disposed in a finally block. Navigation, active-row, viewport containment, and document-scroll assertions remain intact; production timing and behavior are unchanged.
- The repaired case passed after deliberately waiting for the same actual cue expiry before assertion. The complete sidebar suite passed all 24 runs in the constrained CI runtime with retries disabled: `bash e2e/scripts/run-raw-e2e.sh --project=chromium e2e/tests/task/sidebar-scroll-preservation.spec.ts --repeat-each=3 --retries=0 --reporter=list`. Web typecheck, scoped ESLint, Prettier, and whitespace checks passed. The complete 20-report audit found 3,928 passing attempts, 47 skips, two retry attempts, one initial failure, one initial timeout, and zero parse errors. Only the two known cases failed. The next pushed head requires a complete fresh audit.
