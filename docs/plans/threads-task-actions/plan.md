---
created: 2026-09-10
status: done
requirements:
  - REQ-TASKS-THREADS-ACTIONS-001
  - REQ-TASKS-THREADS-ACTIONS-002
  - REQ-TASKS-THREADS-ACTIONS-003
  - REQ-TASKS-THREADS-ACTIONS-004
system_design:
  - ../../specs/tasks/system-design/threads-task-actions.md
legacy_specs: []
---

# Implementation Plan: Threads Task Actions

## Overview

Deliver the existing six task operations from each Threads header using shared
task actions, explicit target identity, and an inset phone drawer. Implement
the shared action boundary first, integrate desktop and deck recovery second,
complete phone interaction third, and publish accurate usage guidance last.

The `tasks` system owns this vertical capability because task identity,
eligibility, mutation results, and recovery are its contract. Existing UI
specifications continue to own the deck, shared header, saved-view query, and
viewport-driven session activity.

The user authorized implementation with "go for it" after the design-package
checkpoint. All four work orders are complete in the primary session. No
subagents or new platform tasks/sessions were used. Implementation and design
files were uncommitted at that implementation handoff; publication follows in
the user-requested Open PR workflow step.

## Parent prerequisite

This workspace started clean at `21f24eb8b` on
`feature/add-task-actions-to-548`. During this design turn, the parent committed
its completed mobile implementation as
`0b4253fa71ba0fe85188bff8cd27c2860425c516`
(`fix(ui): improve mobile threads and shared listing headers`) on
`feature/improve-mobile-threa-f2f`. The parent's working tree is clean.
Its committed `docs/plans/mobile-threads/plan.md` contains the execution results.

The branches diverge from `b7a71e64a`, so integration requires a normal merge,
not a fast-forward. The pinned parent commit was merged successfully at the
implementation start as `facf9aef7`, preserving this package. Its ancestry is verified and
the workspace dependencies are installed with the frozen lockfile.

Read-only inspection identified these parent integration points:

- `components/threads/mobile-thread-column-header.tsx`: two-line title picker,
  Open task, status and session control.
- `components/threads/threads-board.tsx`: board-owned picker and
  `renderHeader(activeMobileTaskId)` composition.
- `components/threads/use-mobile-thread-position.ts` and
  `thread-viewport-geometry.ts`: scroll-derived position independent of chat
  hydration; `use-thread-column-activation.ts` consumes it.
- `components/kanban/mobile-listing-*`: compact shared page header/menu, with
  `mobile-picker-sheet.tsx` gaining `onCloseAutoFocus` support.
- `e2e/tests/task/mobile-threads-swipe.spec.ts` and its swipe helper:
  held-touch midpoint, reversal, and paging evidence.

These paths are under `apps/web/`. The parent work orders and scoped guidance
were re-read after the normal merge and before feature edits. No individual
parent files were copied or recreated.

The separate **Choose Threads as default home view** subtask is not a dependency
and contributes no work to this package.

## Scope

### In scope

- Header-only desktop context entry and an always discoverable overflow button.
- Priority, workflow-step move, cross-workflow send, supported links, archive,
  and confirmed delete through existing task behavior.
- Captured task/workspace identity, current eligibility, shared-state updates,
  failure recovery, deterministic thread fallback, and surviving-trigger focus.
- One inset phone drawer, in-surface nested navigation, localization, desktop
  density, touch sizing, containment, and unchanged native swiping.
- Focused TDD and desktop/mobile Playwright outcomes, rendered phone inspection,
  and usage documentation.

### Out of scope

- Rebuilding the parent header, picker, inline pagination, or swipe logic.
- Default Home selection, saved-view behavior changes, or session management.
- New operations, bulk commands, providers, permissions, backend APIs, storage,
  plugin SDK changes, feature flags, or broad UI menu migration.

## Technical approach

### Shared task action boundary

`hooks/use-task-menu-actions.ts` shares archive/delete lifecycle and successful
removal cleanup with the sidebar. `useTaskManagementFlow` owns captured identity
and current eligibility. `TaskManagementSurface` composes those owners with
`useTaskWorkflowMove`, `useUpdateTaskPriority` and existing link hooks. Preserve current menu option
derivation in `task-priority-context-menu.tsx`, `task-move-context-menu.tsx`, and
`task-switcher-link-menu.tsx`. Extract a focused
`components/task/task-management-menu.tsx` composition for the requested six
groups. Keep the existing full switcher menu as the default consumer.

Factor the existing destructive lifecycle glue into the shared task boundary
and consume it from an existing menu caller as well as Threads. Give
`useTaskRemoval` a caller-selected stay-on-listing behavior with its current
navigation as the default. Test that option against stale global task selection
and HTTP/WS ordering. Thread adapters contain rendering, target selection, and
deck focus, never API transport or task business rules.

The shared host captures `{ taskId, workspaceId }` on open and lives above the
column list. It resolves current task facts from the scoped snapshots by that
ID and retains the target through menu-to-dialog transitions. Reuse
`TaskArchiveConfirmation`, `TaskDeleteConfirmDialog`, and `SidebarLinkDialogs`.
Archive uses the shared preference and classification, including inline phone
confirmation when supported; complex confirmation and link dialogs replace the
menu before opening.

### Threads entry and recovery

Compose the shared host at `ThreadsBoard`, leaving `ThreadsPageClient` unchanged. Add a thin
`thread-task-actions.tsx` header adapter. Extend `TaskMenuButton` for a
controlled opener/ref while preserving existing callers. Restrict the context
trigger to the task header and exclude session controls, transcripts, and
editors.

Add `lib/threads/thread-selection-fallback.ts` with the pure
`resolveRemainingThreadId` algorithm. Use the parent's viewport position and
last committed admitted order for reconciliation. Preserve current surviving
tasks and their scroll offset; otherwise use successor, predecessor, first
newly admitted task, or the existing empty state. Shared lifecycle events drive
admission. Late responses cannot reset newer focus, resurrect removed columns,
or rewrite `/threads` to task detail.

### Phone composition

Add a shared task-domain `task-management-drawer.tsx` over the same option
sources and callbacks as desktop. Its page state contains root, priority,
steps, workflows, selected-workflow steps, and links. Use one Drawer instance,
one body scroller, visible Back, fixed header, `dvh` bounds, and safe-area
clearance. Compare the integrated parent versions of `mobile-menu-sheet.tsx`
and `mobile-picker-sheet.tsx`; do not reproduce the global nested-popper CSS.

Put the visible 44px overflow beside Open task in the parent's task header.
Keep the page topbar and inline swiper as shipped by the parent. Phone and
coarse-pointer entries use the drawer; fine-pointer controls retain compact
sizes. Cover 320px, 360px, 700px phone widths and a coarse-pointer tablet without
introducing a second horizontal gesture region.

## Tests

All code work orders use `/tdd`. Record the failing behavior before production
changes, then the passing targeted command. Proposed new test names describe
outcomes; adapt names only when the same outcome remains explicitly mapped.

| Test file and named cases | Acceptance coverage |
| --- | --- |
| `hooks/use-task-menu-actions.test.ts`, `use-task-management-flow.test.ts`, `lib/tasks/task-menu-target.test.ts`, `components/task/task-management-surface.test.tsx`: captured A, unresolved/cross-workspace targets, duplicate submits, removal failures/retry, obsolete link completion, filtered archive anchor | `002.1`-`002.6` |
| `components/task/task-management-menu.test.tsx`, existing link/confirmation tests and both browser suites: current-step eligibility/priority dispatch, ordered groups, provider choices, archive preference and deletion consent | `001.1`-`001.6`, `002.3`, `002.6` |
| Existing `use-task-removal.test.ts`, `use-task-actions.test.ts`, `use-task-workflow-move.test.ts`, `use-update-task-priority.test.ts`, link/confirmation tests | Existing mutation, cleanup, dirty-worktree, provider, preference, and error behavior under extraction |
| `lib/threads/thread-selection-fallback.test.ts`: seven survivor/successor/predecessor/replacement/empty cases | `003.1`-`003.3` |
| `components/threads/thread-task-actions.test.tsx`, `threads-board.test.tsx`, page/activation tests and both browser suites: named opener, removed-column focus, route preservation, native composer context, newer-menu focus, drafts and bounded activation | `002.1`, `002.2`, `002.5`, `003.1`-`003.6`, `004.1`, `004.6` |
| `components/task/task-management-drawer.test.tsx`, archive/surface tests and mobile browser suite: one nested surface, Back row focus, confirmation handoff and surviving focus | `001.1`-`001.6`, `004.2`-`004.7` |

Numeric shorthand in these tables expands to
`AC-TASKS-THREADS-ACTIONS-<number>`. Work-order frontmatter lists full IDs.

## E2E tests

New files are `apps/web/e2e/tests/task/threads-task-actions.spec.ts` in
`chromium` and `mobile-threads-task-actions.spec.ts` in `mobile-chrome`.
Extract common seed/action helpers to sibling `threads-task-actions-helpers.ts`.
Follow `threads-view.spec.ts`: run a real mock-agent turn to establish a primary
session, or use the existing eligible review/clarification fixture. Arbitrary
seeded RUNNING sessions are not primary and cannot prove the feature.

Each action row below is exercised on **both** projects. The action fixtures
use unrelated tasks A and B with real primary sessions. Captured-flow tests and
the existing session-switching regressions also protect global and sibling
session selection. Check visible results, query persistence, then reload and
verify the task metadata remains correct.

| Action / browser test | Required outcome | AC |
| --- | --- | --- |
| `changes the opened task priority` | Submenu current value changes for A, B stays unchanged; saved-view filtering can remove A | `001.2`, `002.2`, `002.4`, `003.1`-`003.4` |
| `moves the opened task to a workflow step` | A reaches the selected step, existing current/auto-start markers match, membership recovery works | `001.3`, `002.4`, `003.1`-`003.5` |
| `sends the opened task to another workflow` | Select workflow then step; A persists there, no-step workflow is disabled, route and B are preserved | `001.3`, `002.2`, `002.4`, `003.1`-`003.5` |
| `links the opened task through the existing form` | Supported provider rows use existing availability; submit a mocked GitHub issue/PR link to A, verify association and B unchanged | `001.4`, `002.2`-`002.5`, `004.3` |
| `archives the opened task` | Preference on: cancel causes no request, then confirm succeeds; preference off: direct non-cascade archive; survivor/empty state is correct | `001.5`, `002.4`, `003.1`-`003.5` |
| `deletes only the confirmed task` | Destructive row follows separator; cancel preserves task/sessions; required consent precedes one delete; survivor/empty state is correct | `001.1`, `001.6`, `002.2`, `002.6`, `003.1`-`003.5` |

Cross-action scenarios also run in the owning project:

| Scenario | Evidence | AC |
| --- | --- | --- |
| Desktop context and keyboard overflow equivalence | Same six groups; Enter/Space and keyboard submenu traversal work; chat/editor context stays native | `001.1`, `004.1`, `004.6` |
| Selection changes while flow is open | A remains target after independent selection/WS changes, B and its chosen session stay unchanged | `002.1`-`002.3`, `002.5` |
| Rejection and retry | Inject a move or archive failure and invalid link, assert error plus unchanged confirmed state; replay a later valid UI action | `002.5`, `003.4` |
| Nested dismissal and surface handoff | Back/Escape/outside at root and nested pages; cancelled archive/delete/link; original, replaced, removed, and offscreen trigger focus | `001.5`, `001.6`, `004.3`, `004.6` |
| Narrow phone, long lists and native swipe | Bound every active page/dialog; assert 44px hitboxes, hit testing, one scroller, safe-area clearance, and no document overflow; swipe before and after menu use | `003.6`, `004.2`-`004.7` |

For phone identity testing, use live updates or an independent client while the
modal is open; do not force clicks through an inert backdrop. Exercise actual
touch swipes with the parent's helper after dismissal. Use mocked providers
and isolated E2E storage, not production credentials or a developer instance.
Component tests cover all first-party and registered plugin choice dispatch;
the browser link outcome uses supported existing integration fixtures.

Managed runs build current production assets and use one worker per shard.
Run desktop and mobile sequentially with one project per command and retries
disabled. Use causal HTTP/WS waits or persisted-state polling, never sleeps.
Restore any modified user settings in `afterEach`.

## Work orders

- [x] [Task 01: Share the task action flow](task-01-shared-task-action-flow.md)
- [x] [Task 02: Integrate desktop Threads task actions](task-02-desktop-threads-actions.md)
- [x] [Task 03: Make task choices touch accessible](task-03-phone-task-actions.md)
- [x] [Task 04: Document Threads task actions](task-04-document-task-actions.md)

Execute 01 → 02 → 03 → 04 sequentially after the parent prerequisite. No work
order is parallel-safe because menu types, action state, and test helpers overlap.
Each code work order owns its RED/GREEN and exact verification commands.

## Verification results

- Parent `0b4253fa7` merged as `facf9aef7`; ancestry verified. Frozen-lockfile
  install, `make -C apps/backend build`, and `pnpm run build:e2e` passed.
- Focused unit/component checks: **217 passed in 25 files**, two workers.
- Desktop: **6 new tests passed** with retries disabled. All **17 existing**
  Threads/workflow regressions passed. A first combined run found a test-driver
  submenu pointer-grace issue; explicit submenu activation fixed the driver,
  and the final six-test rerun passed against the same production build.
- Mobile: the **29-test suite passed**, including all five new tests and 24
  parent/shared-menu cases, with retries disabled. Test discovery
  confirmed all 29 cases; the final run reported no failed tests.
- Typecheck, affected-file ESLint with zero warnings, formatting, `i18n:check`
  and `i18n:ratchet` passed. All copy reuses existing keys; no locale generation
  or catalog changes were necessary. Existing orphan-key warnings are unchanged.
- New 320px/360px phone header, root/deep drawer and delete-confirmation renders
  inspected. The menu is inset, long labels remain bounded, the last step is
  reachable, and the original compact topbar/swiper remain intact. Task 03
  records image paths and geometric assertions.
- Public-doc validator tests passed; the live validator passed for 46 pages.
- `python3 scripts/lint-spec-files.test.py`: passed, 30 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Package link/traceability check: all seven new artifacts resolve their links;
  all 25 acceptance criteria are assigned to work orders.

The final combined unit command, from `apps/web`, was:

```bash
pnpm exec vitest run --maxWorkers=2 \
  hooks/use-task-menu-actions.test.ts hooks/use-task-management-flow.test.ts \
  hooks/use-task-removal.test.ts hooks/use-task-removal-session-loading.test.ts \
  hooks/use-task-actions.test.ts hooks/use-task-workflow-move.test.ts \
  hooks/use-update-task-priority.test.ts lib/tasks/task-menu-target.test.ts \
  lib/threads/thread-selection-fallback.test.ts \
  components/task/task-management-menu.test.tsx \
  components/task/task-management-surface.test.tsx \
  components/task/task-management-drawer.test.tsx \
  components/task/task-switcher-context-menu.test.tsx \
  components/task/task-archive-confirmation.test.tsx \
  components/task/task-delete-confirm-dialog.test.tsx \
  components/task/task-session-sidebar-link-actions.test.ts \
  components/task/task-session-sidebar-move.test.ts \
  components/threads/thread-task-actions.test.tsx \
  components/threads/threads-board.test.tsx \
  components/threads/thread-column-activation.test.tsx \
  components/kanban/kanban-header-mobile.test.tsx \
  app/threads/threads-page-client.test.tsx lib/threads/stable-order.test.ts \
  lib/threads/thread-view-query.test.ts \
  components/threads/thread-session-switcher.test.tsx
```

Managed E2E commands used `--host --no-build` after explicitly building current
production assets. They ran sequentially with one worker, against isolated
mock-agent/provider fixtures. Task 02 and Task 03 record the exact commands.

Design reference inspection used the parent's existing rendered 360px captures
`/tmp/kandev-mobile-threads-PuEDK5/mobile-shared-threads-360.png` and
`mobile-shared-threads-menu-360.png`. They show the compact inline topbar, the
existing task header, and inset menu geometry. They are historical parent
evidence only; they do not verify the proposed task menu.

## Header alignment follow-up (2026-09-10)

Completed the user's screenshot-driven styling correction within the existing
package. Desktop Open task and overflow now share a vertical center; mobile
picker, Open task and overflow have equal 48px center-to-center spacing. Title
placement, header height, touch targets and task/swipe behavior are preserved.

Both new bounding-box assertions failed on the previous build (desktop 2px
offset; mobile 38px versus 48px gaps) and passed after the fresh E2E build. The
focused desktop context/keyboard and mobile long-label/nested/swipe tests each
passed with one worker and no retries. Typecheck, affected-file ESLint,
formatting, specification lint and whitespace checks passed. Task 02 and Task
03 contain exact commands and evidence.

Fresh dark desktop and 360px touch-phone rendering was inspected in the existing
isolated Tailscale preview. Screenshots are retained under
`/tmp/kandev-threads-seed-scLDiX/{desktop,phone}-header-{before,after}.png`.
Only its generated static assets/index were refreshed; prior assets/index are
retained, and its processes and user test data were not restarted or changed.
The preview remains on :19468, with the same identity-checked shutdown command.
The main :9998 instance was not queried, modified or stopped.

After approving the visual correction, the user requested shutdown. The
identity-checked sandbox wrapper stopped its backend and agent descendants;
ports 19468, 59468 and 59568-59667 no longer listen. Disposable data and captures
are retained. The main :9998 runtime was not stopped or reconfigured.

## Ready PR integration (2026-09-10)

The user requested a concise, template-based ready PR. Feature commit
`046832b6d` passed the normal commit hooks. Parent PR #3570 remained open;
its reviewed tip `8b508fdb307f4449edc2c2fbe23a5dad4d87a54e` was integrated
through merge `4c68ac5c5`. The PR targets `feature/improve-mobile-threa-f2f`
so its diff contains only this follow-up, not the parent's changes.

The combined board exceeded the function-size lint by one line. Extracting
its existing picker-focus helper preserved behavior; all 26 board tests
passed before and after extraction, followed by successful normal merge
hooks. No parent behavior or unrelated main-branch changes were replaced.

Integrated validation passed: 219 focused unit/component tests in 27 files
(including extracted listing-removal cases and parent mobile-position
coverage), typecheck, i18n checks and ratchet, public-doc validator tests and
all 46 published pages, 36 specification-linter tests, full specification
lint, and whitespace checks.

A fresh managed backend/web build produced synthetic-data PR screenshots
at 1280x850 desktop and 360x780 touch-phone sizes. The desktop root menu,
phone root drawer and nested workflow-step drawer were inspected and PNG
compressed. The ignored `.pr-assets/manifest.json` maps all three files;
the disposable capture spec was removed. The manual Tailscale preview
remained stopped throughout.

The first combined build/browser invocation was terminated with SIGTERM
(exit 143) after 19 passing tests, including the cross-device capture; it
reported no assertion failure. Its processes had exited before the final
regression runs reused the same fresh build.

The final desktop run passed all 23 tests with one worker and no retries:

```sh
cd apps/web
pnpm e2e:run --host --no-build --project chromium tests/task/threads-task-actions.spec.ts tests/task/threads-view.spec.ts tests/kanban/cross-workflow-task-move.spec.ts -- --retries=0
```

Both bounded mobile runs also passed, with one worker and no retries:
14 Threads/task-action/swipe tests and 17 shared-sidebar regressions.
Together, final browser verification covers 23 desktop and 31 mobile cases.

```sh
pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-threads-task-actions.spec.ts tests/task/mobile-threads-view.spec.ts tests/task/mobile-threads-swipe.spec.ts -- --retries=0
pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-sidebar-task-actions.spec.ts -- --retries=0
```

## PR feedback correction (2026-09-10)

The Back and Close drawer controls now use the repository-required pointer
cursor. The existing mobile regression reproduced the prior `default` cursor
before the two class changes. A fresh managed build passed all five mobile
task-action cases; the focused drawer/surface/header component checks passed
five tests, with ESLint and typecheck also passing. The 360px nested drawer
was inspected. Task 03 records the exact commands and RED/GREEN evidence.

Reviewer follow-up also reproduced and fixed same-workflow moves being rejected
when that workflow is hidden, loaded active steps being shadowed by a placeholder
snapshot, and archived targets remaining eligible in a stale snapshot. The
GitLab link row again retains its pre-existing 48px phone minimum. Regression
tests failed for each case before the scoped fixes. The focused suite passed
64 tests in ten files; the fresh mobile build passed all five Threads action
tests plus the GitLab linking regression (six tests, one worker, no retries).
The six desktop action tests passed after the behavior-preserving cleanup.

Claude's suggestions removed ineffective props memoization and redundant
workflow filtering, named the empty-board dependency, and clarified the existing
error-toast ownership. Cubic's postfix-important warning was not reproducible:
the generated CSS includes important width/height for `size-11!`, and the phone
regression checks real 44px targets through the 820px coarse-pointer viewport.
CodeRabbit's documentation nit was valid; the how-to now describes the existing
first-new-thread fallback before the genuinely empty state. These corrections
implement the existing requirements and design, without a new contract or copy.

Parent PR #3570 merged while review was in progress, retargeting this PR to
`main`. Integration with `5ffe8818773bd7c7bda548f3086b305b9b618d55` preserves
the landed control-sizing primitives, shared archive/delete switching helper,
and task-detail dialog focus refs alongside Threads' listing-only removal and
custom focus recovery. Two compatibility tests cover default-ref restoration
and the Threads override. The integrated unit/component suite passed 252 tests
in 29 files, and the fresh managed desktop build passed 27 browser tests with
one worker and no retries, including shared task-detail/preview outcomes.
The integrated phone run passed 15 cases with one worker and no retries:
Threads actions, parent view/picker/swipe behavior, and the shared GitLab link
row. It reused that freshly built backend/web pair.
Typecheck, affected-file ESLint, locale validation, public-doc validation,
specification lint and harness checks passed. Earlier stacked-base results
above remain historical.
Remote exact-head CI/review confirmation remains part of the ongoing fixup,
not a completed claim in this local verification record.

### CI archive concurrency correction (2026-09-10)

E2E run `34527249627`, shard job `103043769179`, reproduced a shared-sidebar
regression: after optimistic navigation from A to B, archiving B while A's
request was pending was silently rejected by the shared global single-flight
guard. The trace contained only A's archive request. The unchanged
`archive-task-redirect.spec.ts` reproduced the same last-task URL failure
locally with retries disabled. Its dependent report and final gate failed from
that one unexpected test; no additional failed leaf was reported.

Four held-request unit cases failed before the fix for archive and delete,
covering concurrent A/B requests, same-task duplicate rejection, out-of-order
completion, and A's failure/retry while B remains pending. The shared removal
owner now guards pending task IDs independently and retains the first pending
ID for existing busy consumers. Navigation, confirmation, error feedback and
Threads' listing-only cleanup still belong to their existing owners.

After the fix, the focused unit/component checks passed 141 tests in 17 files,
including those four regressions. Typecheck, zero-warning affected-file ESLint
and locale validation passed. After `pnpm run build:e2e`, these managed browser
runs passed with one worker and no retries: eight desktop cases and nine phone
cases, including all six actions, cancellation, nested containment and swipe
coexistence.

```sh
cd apps/web
pnpm exec vitest run --maxWorkers=2 hooks/use-task-menu-actions.test.ts hooks/use-task-actions.test.ts hooks/use-task-actions-menu-move-targets.test.tsx hooks/use-task-removal.test.ts hooks/use-task-removal-listing.test.ts hooks/use-task-removal-session-loading.test.ts hooks/use-task-workflow-move.test.ts hooks/use-task-management-flow.test.ts components/task/task-management-surface.test.tsx components/task/task-archive-confirmation.test.tsx components/task/task-delete-confirm-dialog.test.tsx components/task/task-session-sidebar-link-actions.test.ts components/task/task-session-sidebar-move.test.ts components/task/task-session-sidebar-selection.test.ts components/threads/thread-task-actions.test.tsx components/threads/threads-board.test.tsx
pnpm exec vitest run --maxWorkers=2 lib/tasks/task-menu-target.test.ts
pnpm e2e:run --host --no-build --project chromium tests/task/archive-task-redirect.spec.ts tests/task/threads-task-actions.spec.ts -- --retries=0
pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-threads-task-actions.spec.ts tests/task/mobile-threads-swipe.spec.ts tests/task/mobile-archive-task-redirect.spec.ts -- --retries=0
```

CodeRabbit's valid follow-up suggestions also corrected the work-order path,
strengthened inline confirmation containment with an awaited scoped query,
and preserved all six link-handler keys with compile-time checking. The
suggested Threads-agent contract is not a repository requirement: the scoped
guide documents component ownership, not a separate agent. The optional
layout-effect optimization is not applied; its effect reads `orderedIds` and
refreshes geometry/focus bookkeeping, and no lint suppression is introduced.
These changes preserve the approved contract and rendered layout. Remote CI
and current-head review are rechecked after pushing, not assumed from local
results.

## Main integration follow-up (2026-09-11)

Resolved conflicts with the landed task-removal navigation coordinator in the
sidebar, action wrappers and removal hook. Both shared menu operations now use
that coordinator. Threads records pending targets without a detail departure,
so remembered global selection cannot trigger destination loading or failure
recovery navigation. Default detail/preview protection, cascade cleanup,
per-task duplicate guards and localized success/error feedback remain shared.

Four added integration cases cover archive/delete pending ownership, successful
cleanup and failed removal with a remembered active session. Three failed on
the initial mechanical merge: archive acquired a detail departure, deletion
did not register a coordinated operation, and failed archive redirected to the
overview. The focused checks passed after the compatibility fixes: 201 tests
across 25 files, including the landed coordinator and all existing menu/Threads
regressions. The new cases can be rerun from `apps/web` with:

```sh
pnpm exec vitest run --maxWorkers=2 hooks/use-task-menu-actions.test.ts hooks/use-task-removal-listing.test.ts hooks/use-task-actions.test.ts hooks/use-task-removal-coordinator.test.ts hooks/use-task-removal.test.ts hooks/use-task-removal-session-loading.test.ts hooks/use-task-management-flow.test.ts components/task/task-session-sidebar-selection.test.ts lib/state/task-removal.test.ts
pnpm exec vitest run --maxWorkers=2 hooks/use-task-workflow-move.test.ts hooks/use-update-task-priority.test.ts hooks/use-task-crud.test.ts hooks/use-sidebar-multi-select.test.ts components/task/task-management-menu.test.tsx components/task/task-management-surface.test.tsx components/task/task-management-drawer.test.tsx components/task/task-switcher-context-menu.test.tsx components/task/task-session-sidebar-link-actions.test.ts components/task/task-session-sidebar-move.test.ts components/task/task-archive-confirmation.test.tsx components/task/task-delete-confirm-dialog.test.tsx components/threads/thread-task-actions.test.tsx components/threads/threads-board.test.tsx lib/tasks/task-menu-target.test.ts lib/threads/thread-selection-fallback.test.ts
```

Typecheck, affected-file ESLint, formatting, locale checks, specification lint,
harness validation and public-doc validation passed. The incoming canvas fixture required only `gofmt`
normalization; its whitespace-insensitive diff against main is empty.
The system design now names the coordinator. Public documentation and the
existing static menu screenshots still describe the same user workflow and
layout. Managed browser verification passed all 10 desktop and seven phone
cases, with one worker and no retries. The first command rebuilt the backend,
web assets and packaged plugin fixture; the second reused that same build.
Fresh 320px menu and 360px confirmation captures were inspected for containment.

```sh
pnpm e2e:run --host --project chromium tests/task/threads-task-actions.spec.ts tests/task/archive-task-redirect.spec.ts tests/task/delete-task-redirect.spec.ts -- --retries=0
pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-threads-task-actions.spec.ts tests/task/mobile-archive-task-redirect.spec.ts tests/task/mobile-delete-task-redirect.spec.ts -- --retries=0
```

Remote CI/review confirmation remains pending until the merged branch is pushed.

## Risks

- Parent files and archive-confirmation work can change before integration;
  re-read those owners before implementation and preserve their final changes.
- `ActiveThread` lacks priority, step identity, executor and repository details;
  a lossy task projection would silently alter eligibility or consent.
- Sidebar removal helpers can navigate to `/t/:id` or Home using remembered
  global selection; the shared listing option must suppress that side effect.
- Menu unmount, nested overlay handoff, late responses, and filter removal can
  invalidate trigger refs or retarget callbacks unless identity and focus have
  explicit lifetimes.
- The below-640px global menu styles do not cover every phone/coarse-pointer
  case and position nested portals independently; scoped drawer navigation and
  rendered containment tests are required.
