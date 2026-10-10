---
id: "06-firefox-task-paint"
title: "Remove unnecessary work before task-switch paint"
status: done
wave: 6
depends_on:
  - "05-immediate-task-route"
plan: "plan.md"
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
acceptance_criteria:
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.6
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.7
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.15
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
  - ../../specs/ui/system-design/sidebar-archived-filter.md
---

# Task 06: Firefox task-switch paint

Continue the explicitly authorized responsiveness repair after the user reported
a remaining URL-to-content pause on deployed revision `d7280234a`. An isolated
production build with fictional conversations reproduces it: eight warm switches
have median click-to-content frame readiness of 468 ms in Firefox and 188 ms in
Chromium with profiling enabled. This is a DOM/animation-frame observation, not
compositor paint or certification of the user's browser.

Firefox samples in the click-to-content windows identify synchronous virtual
file-tree geometry reads, spinner computed-style reads, repeated responsive
viewport queries, and closed task-dialog setup. Remove proven avoidable work
incrementally and repeat the same measurement before expanding the fix.

## Scope and regression sequence

1. RED/GREEN: file rows use positive cached/estimated geometry until their
   ResizeObserver measurement arrives, without synchronous layout on mount.
   Preserve hidden-row protection, later measured sizes, and mounted-row bounds.
2. RED/GREEN: responsive consumers share a stable, event-updated viewport
   snapshot. Ordinary task rerenders must not read layout or create media
   queries; breakpoint/pointer transitions and final unsubscribe remain correct.
3. RED/GREEN: delay spinner animation promotion until after the first frame,
   cancel deferred work on unmount, and retain visibility/reduced-motion rules.
   Defer unopened task-create form initialization while retaining its later
   close/focus/draft lifecycle.
4. RED/GREEN: retain only context-free default Markdown rendering in a bounded
   LRU; prove current file links, diagram ownership, motion, custom renderer and
   eviction behavior. Keep unrelated sidebar rows stable during selection.
5. RED/GREEN: measure panel width once before constraint writes and use browser
   ResizeObserver entries for file-tree viewport geometry, with the existing
   synchronous fallback when that observer is unavailable.
6. Verify warm/cold desktop and phone navigation, file-tree geometry and actions,
   rapid switching, and any changed lifecycle. Record matched Firefox/Chromium
   measurements without a machine-dependent CI latency threshold.

Own the affected rendering helpers/hooks, their regression tests, focused browser
coverage, and this plan. Keep task/session authority, read tracking, composer
drafts, scroll ownership, hidden animations, and responsive composition intact.
Do not change backend APIs, pagination, dependencies, or the personal instance.

## UI and mobile contract

Reuse UI-03 from the plan and the shipped phone task picker. No new controls or
loading overlays. Desktop retains sidebar, header, chat and optional workbench
panes. Phone retains its task picker sheet and focused chat/Files view, 44px
touch targets, dynamic viewport and safe-area handling.

```text
Desktop                             Phone
| task sidebar | selected header |  | selected task / picker |
|              | chat | files    |  | focused chat or Files  |
|              | composer        |  | composer / navigation  |
```

## Verification

Run each changed helper/hook suite after the last production edit, web typecheck,
changed-file lint/format, localization checks, and specification validation.
Use the managed desktop/mobile task-route and file-tree regressions; add focused
lifecycle coverage for any additional measured fix. Compare the same fictional
fixture/build settings in Firefox and Chromium and preserve raw profiles locally.

## Results

Implemented the seven measured browser-work reductions above on base
`ccaa7c0a8d80ef4f2f089b1f416bd1230417da0a`. The interrupted validation was rerun
on that base. No backend API, dependency, task/session authority, or mounted-chat
retention change was needed.

- 116 focused unit tests passed across the responsive hook, spinner, Markdown
  rendering/context/motion, task dialog, file-tree measurements, sidebar render
  stability, and Dockview measurement/scroll-target suites. The spinner suite
  passed again after splitting an oversized test group for lint.
- `pnpm run typecheck`, changed-file ESLint with `--max-warnings 0`, Prettier,
  `pnpm run i18n:check`, `git diff --check`, `python3 scripts/list-docs.py validate`,
  and `python3 scripts/lint-spec-files.py --all` passed.
- Managed Chromium: 26 tests passed in task route responsiveness, large file-tree
  virtualization, persistent animation motion, chat motion, dialog body lock,
  and task creation. Managed mobile Chrome: nine tests passed in the corresponding
  phone routes/tree/motion/dialog suites plus creation focus and Escape handling.
- Firefox: eight scenarios passed across route responsiveness, large-tree
  geometry, chat motion, dialog lock, and settled sidebar spinner coverage.
  The existing motion test initially waited indefinitely for `scrollend` despite
  a completed wheel scroll. Its synchronization now observes a settled scroll
  position after asserting real input moved away from the bottom; animation,
  reduced-motion, history and reader-ownership assertions remain intact.
  The changed scenario passed again in Firefox, Chromium and mobile Chrome.

Browser commands ran from `apps/web`: `pnpm e2e:run --host --project chromium`
with `tests/task/{task-route-responsiveness,large-file-tree-virtualization,dialog-body-lock,create-task}.spec.ts`
and `tests/chat/{persistent-animation-motion,chat-motion}.spec.ts`; then
`pnpm e2e:run --host --no-build --project mobile-chrome` with
`tests/task/{mobile-task-route-responsiveness,mobile-large-file-tree-virtualization,mobile-dialog-body-lock,mobile-task-create-escape,mobile-creation-auto-focus}.spec.ts`
and `tests/chat/{mobile-persistent-animation-motion,mobile-chat-motion}.spec.ts`.
Firefox used the same one-worker fixtures and a temporary project with Playwright's
`Desktop Firefox` device through `pnpm e2e:raw`; its final changed motion scenario
also ran with `--grep 'live prose'` in all three projects. WebSocket accounting was
strict. Diagnostic configuration, logs and raw captures remain locally ignored.

### Matched production measurements

Both builds used `pnpm exec vite build --sourcemap`, the same isolated backend
and fictional data, and fresh browser contexts. Each browser/build performed two
interleaved batches of ten warm switches in baseline/fixed/fixed/baseline order,
without a CPU profiler. The served entry script was checked against each build.
The fixture contains 18 tasks, two 150+ message conversations (newest 100 loaded),
and 633 repository files at a 1868x1040 viewport.

| Browser | Baseline median | Fixed median | Baseline p90 | Fixed p90 |
| --- | --- | --- | --- | --- |
| Firefox 151 | 347.0 ms | 278.5 ms | 384.0 ms | 329.0 ms |
| Chromium 149 | 181.1 ms | 141.6 ms | 194.1 ms | 158.6 ms |

This is click capture to the first animation-frame observation of the selected,
visible cached chat without a task loader or inert ancestor. It is not compositor
paint, a universal latency guarantee, or a measurement of the user's personal
Firefox profile. Firefox still has a noticeable delay; this change removes
avoidable work without claiming instant rendering. All benchmark page-error
lists were empty.

The retained preview at `http://localhost:48629` owns separate data, ports,
executables, worktrees and mocked providers under
`~/.local/share/kandev-demos/firefox-task-paint-e3nbc8rp/`. Its manifest records
source/build identity; `start.py` and `stop.py` operate only on its owned runtime.
No private transcript content enters fixtures or public assets. Public docs need
no change: controls, terminology, API/configuration and installation behavior
remain unchanged; the owning system design records the rendering/cache bounds.

### Review follow-up

PR #4062 identified a missing synchronous row fallback when ResizeObserver is
unavailable. A new regression first failed with the stale 44px cached height
instead of the actual 52px height. Rows now read actual height in that environment
and preserve the positive cache only while hidden; observer-backed browsers keep
the deferred measurement path. The fallback and Markdown rendering/context/motion
suites passed all 63 tests after this change; changed-file lint/format and web
typecheck also passed.

The static renderer now documents its required synchronous, hook-free dependency
contract. Its tests reload the module before each case, including a repeated
content string in consecutive cases, so cache state cannot depend on test order.
No production-only test reset API is needed. The performance table above records
`449491c73`; the follow-up preserves its observer-backed browser path.

Post-follow-up browser validation rebuilt through
`pnpm e2e:run --host --project chromium tests/task/large-file-tree-virtualization.spec.ts`
(three passed), then ran the same fresh assets with strict WebSocket accounting
through `pnpm e2e:raw` for the Firefox desktop suite and the mobile Chrome
`mobile-large-file-tree-virtualization.spec.ts` suite (four passed). Specification
catalog/lint checks and normal commit hooks also validate the follow-up.

### Background refresh layout follow-up

The user reported that the `Updating tasks...` label moved the task list down.
The shared query-status presenter added a normal-flow row above retained tasks
on each background read. A causal browser regression held that read open and
measured a 40px downward shift before the fix. The same presenter now uses the
existing screen-reader-only utility for its polite refresh announcement. Initial
loading and actionable errors/Retry retain their existing presentation.

The desktop sidebar and the phone `SessionTaskSwitcherSheet` keep their current
view controls, scroll owner, task actions and focus behavior. The shared phone
app-navigation outlet inherits the same presenter. The retained-view regression
compares task-row geometry before and during refresh and after recovery, retaining
the existing query rejection, Retry, conversation and phone touch-target checks.
This change needs no public documentation update: no control, configuration or
API changes. The owning sidebar requirement/design record the stable layout.

Before this follow-up, the branch was rebased without conflict onto the landed
session-refresh/navigation work, `320f050e12071342522a7ba0730af03e6ce1b386`.
Earlier benchmark and CI results above remain evidence for their named revisions;
verification of the combined revision is recorded separately.

Combined-revision verification recorded 148 passing unit tests across 14 files,
passing typecheck, changed-file lint/format, i18n and specification checks. The
held-refresh regression passed in Chromium, Firefox and mobile Chrome. The
seven-scenario phone navigation/sidebar suite passed. Two combined desktop
attempts lost their isolated backend without a diagnosed cause; a final
seven-scenario desktop run passed with exit tracing showing normal fixture
shutdown. These earlier failures remain recorded rather than attributed to a
source fix. At the user's request, further test execution is delegated to CI;
no additional local test runs are required for delivery.
