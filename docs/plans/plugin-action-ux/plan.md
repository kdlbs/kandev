---
created: 2026-09-25
status: complete
requirements:
  - REQ-PLUGINS-ACTION-UX-001
  - REQ-PLUGINS-ACTION-UX-002
  - REQ-PLUGINS-ACTION-UX-003
system_design:
  - ../../specs/plugins/system-design/plugin-action-ux.md
legacy_specs: []
---

# Implementation plan: Plugin action UX

## Overview

Add an optional `host.ui.Action` with styling selected by its host location.
Existing plugins retain their registrations and behavior. Adoption changes the
control shell without transferring plugin state or callbacks to a new registry.

Deliver five sequential work orders. Start with the shared contract and composer
slice, then add topbar/sidebar and status surfaces. Finish mixed-version
compatibility evidence and author documentation. This package does not release
changes to the separate plugin repositories.

The plugin system owns the extension contract. Existing UI contracts own sizing,
status placement, and mobile navigation. The user confirmed additive adoption
and no mandatory plugin changes. The earlier investigation supplies the
[source audit](plugin-adoption.md), not runtime compatibility certification.

## Inputs

- [Requirements](../../specs/plugins/requirements/plugin-action-ux.md)
- [System design](../../specs/plugins/system-design/plugin-action-ux.md)
- [Decision](../../decisions/2026-09-25-additive-plugin-action-chrome.md)
- [Control sizing](../../specs/ui/system-design/control-sizing.md)
- [Status surface](../../specs/ui/system-design/app-status-bar.md)
- [Contribution lifecycle](../../decisions/2026-08-04-plugin-contribution-lifecycle-authority.md)
- [Status ordering](../../decisions/2026-07-21-portable-status-bar-order.md)

## Scope

### In scope

- Typed Action and ActionGroup exports, shared native styles, and explicit slot context.
- Composer, main/task topbar, sidebar workspace, and status presentations.
- Native controls that establish each affected action cluster's shared styling.
- Legacy preservation, owner cleanup, status order, mobile selection, and fallback tests.
- Authoring examples and a repository-specific adoption map.

### Out of scope

- A generalized action registry, new slot names, forced migration, or slot removal.
- Publishing or implementing official plugin updates in their separate repositories.
- Redesigning rich plugin content, status metrics, split buttons, or submit controls.
- New toolbar overflow behavior, navigation destinations, backend APIs, flags, or persistence.
- Changing tablet status routing or its existing 24px compact bar.

## Technical approach

### Public contract and renderer

Add typed `Action` and `ActionGroup` entries to the SDK's `PluginUIShape` and
exports in `host-api.ts`. Add the props to host type aliases. Retain the
runtime-free SDK and test an external consumer through `sdk-contract.test.ts`.

Create internal `components/actions/surface-action.tsx` and
`surface-action-styles.ts`. Use the current Button behavior, shared sizing,
and native surface tokens. Do not alter Button defaults. The public adapter
filters styling props and preserves supported refs, events, and ARIA metadata.

`ActionGroup` owns spacing between multiple standard actions in one contribution.
An ordinary single Action adds no margin. Existing host parents retain spacing
between registrations. There is no new wrapper around legacy components.

### Slot context and vertical integration

Add a DOM-free surface provider through an internal `actionSurface` prop on
`PluginSlot` and `PluginSlotRegistrationView`. Keep existing keys and slotProps.
Mounts supply explicit context. Presentation updates change context, not identity.

Task 01 integrates all native composer surfaces and their adjacent attachment
control. Task 02 integrates both topbars and sidebar workspace actions, including
representative ordinary native controls in each cluster. Task 03 integrates
status item rendering and the native LSP status action.

Native menu and sidebar-footer navigation contributions already use host
renderers. They remain unchanged. The sidebar footer is not the app status bar.
The first supplied image resembles that footer; it does not justify adding a
new arbitrary footer slot.

### Documentation and delivery

Task 05 updates the canonical authoring reference, PLUGIN-API, and web guidance.
Document feature detection and single-path fallback. Record the supporting
release only when a plugin release actually adopts the API.

Run work orders sequentially with TDD. No delegation is authorized. A fresh
worktree requires `(cd apps && pnpm install --frozen-lockfile)` before pnpm checks.
Use the managed E2E runner, which rebuilds the production frontend and backend.
Do not run full suites or overlap E2E runs for this package.

## ASCII UI preview

These sketches specify grouping and location. Glyphs, sample values, and labels
are illustrative. Pixel dimensions come from the design, not ASCII spacing.
All real labels use localization.

### UI-01: Desktop action locations, available state

```text
Main/task topbar: [native tool] [plugin icon] [plugin icon 63%]
Composer:        [attach] [plugin mic] [plugin cost] ... [send]
Sidebar row:    [New task] [Quick terminal] [Quick chat] [plugin]
Status bar:     connected | plugin status 63%              LSP
```

Standard controls within each role share border, padding, height, and focus
style. The host owns inter-control gaps. The status bar stays 24px high.
Sidebar row actions retain their compact 24px role and 14px glyphs.
Legacy controls can coexist with their existing appearance.
Covers AC-PLUGINS-ACTION-UX-002.1 through .4 and .6.

### UI-02: Phone entry points, available and expanded states

```text
App menu                     Active composer
+------------------------+   +------------------------+
| Plugins                |   | Draft text             |
| [plugin] [plugin 63%]   |   | [attach] [mic] [send]  |
| [workspace tool]       |   +------------------------+
+------------------------+

Status entry -> inset drawer
+------------------------+
| Status                 |  fixed header
| [icon] Plugin status   |  44px or taller rows
| [icon] Other status    |  one scrolling body
+------------------------+  safe-area clearance
```

Reuse current menu and drawer routing. Task contributions replace the same
plugin's workspace topbar contribution only when they render. Sidebar actions
remain independent. Actions stay near their composer. No new nested scroller.
Covers AC-PLUGINS-ACTION-UX-002.5 through .7 and 003.3 through .5.

### UI-03: Interaction states, same dimensions

```text
Idle       Pressed      Busy, still stoppable    Disabled
[ mic ]    [ stop ]    [ spinner + stop ]        [ muted ]

Icon + value: [icon 63%]   Long value: [icon long...]
Focus: visible native ring around the same control bounds
```

Busy does not disable automatically. The accessible name stays available when
visible text truncates. Custom glyphs animate inside a fixed icon box.
Covers AC-PLUGINS-ACTION-UX-003.1 through .3 and 002.3/.7.

## Tests

Paths are relative to `apps/web` unless stated otherwise. New test names below
are implementation targets, not claims that tests already exist.

| Criteria | Test file and cases |
| --- | --- |
| 001.1, 001.2, 001.4 | `lib/plugins/sdk-contract.test.ts`: legacy consumer, typed Action consumer, old-host single-path fallback |
| 001.3, 001.5, 003.5 | `components/plugins/plugin-slot.test.tsx`: mixed owners, null contribution, presentation rerender, disable/re-enable |
| 002.1-.3, 003.1-.3 | New `components/plugins/plugin-action.test.tsx`: allowed props, fixed state geometry, forwarded ref/hold events, disabled activation, tooltip composition |
| 002.4, 003.5 | `components/task/chat/chat-input-plugin-actions.test.tsx` and `components/task-create-dialog-selectors.test.tsx`: all four composer contexts, stale capability unchanged |
| 002.4, 003.4 | `components/plugins/mobile-plugin-nav-section.test.tsx`: rendered task replacement and null fallback |
| 002.6, 001.3 | `components/app-status-bar/app-status-bar-plugin-slots.test.tsx` and `app-status-bar-order.test.ts`: presentation and stable ordering identity |
| 001.1-.5 | New `lib/plugins/action-compatibility.test.tsx`: immutable legacy fixtures, mixed API generations, fallback and cleanup |
| 001.2, 003.2 | `apps/packages/plugin-sdk/test/standalone-contract.node.mjs` plus web typecheck: runtime-free SDK and React-compatible event/ref types |

Numeric ranges in this table use the `AC-PLUGINS-ACTION-UX-` prefix.
Browser tests provide geometry evidence; DOM unit tests do not prove CSS sizes.

## E2E tests

New files live in `apps/web/e2e/tests/plugins/` and use the packaged fixture.
Tests select `composer`, `chrome`, or `status` in their titles for focused slices.

| File/project | Flow and criteria |
| --- | --- |
| `plugin-action-ux.spec.ts`, chromium | Compare native/plugin bounds and computed border, radius, padding, gap, and glyph box in each surface; 002.1-.4/.6/.7 |
| Same file, chromium | Light/dark, long values, two actions per contribution, keyboard, disabled/busy/toggle, ref/hold, overlay dismissal and focus return; 003.1-.3 |
| `mobile-plugin-action-ux.spec.ts`, mobile-chrome | Reach topbar actions in Plugins, all four composers, and ordered Status drawer; complete actions by tap; 002.4-.7, 003.1/.3-.5 |
| Same file, mobile-chrome | Phone, 767px, 768px and wide coarse-pointer cases; 44px ordinary hit targets, containment and no document overflow; compact status exception measured separately |
| `plugin-action-compatibility.spec.ts`, chromium | Legacy and adopted controls coexist; raw/custom styles unchanged; disable/re-enable and persisted status order after reload; 001.1/.3/.5 |
| Existing composer and mobile topbar/status specs | Existing workflows remain usable after shared primitive integration |

Use finite-animation waits, scoped locators, and real hit testing. Compare
same-role controls within a 1px geometry tolerance. Inspect screenshots from
these focused runs against UI-01 through UI-03. Restore changed user settings.

## Work orders

| Order | Work order | Dependency | Status |
| --- | --- | --- | --- |
| 1 | [Shared action API and composers](task-01-action-api-composers.md) | None | done |
| 2 | [Topbar and sidebar actions](task-02-topbar-sidebar.md) | 01 | done |
| 3 | [Status action presentations](task-03-status-actions.md) | 02 | done |
| 4 | [Legacy compatibility evidence](task-04-compatibility.md) | 03 | done |
| 5 | [Authoring and adoption guidance](task-05-authoring.md) | 04 | done |

## Verification results

Implementation: all five work orders complete.
Design validation on 2026-09-25:

- `python3 scripts/list-docs.py validate`: passed, 306 decisions and 1153 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/decisions docs/plans/plugin-action-ux`: passed.
- Local link, dependency, criterion, and whitespace audit: passed for all 10 new documents.
- All 17 acceptance criteria are referenced by work orders.
- `git status --short`: only this untracked design package; no staged or production changes.

Task 01 checks passed: plugin SDK tests/typecheck, 75 focused web tests, web
typecheck, desktop/phone composer E2E, and focused screenshot inspection. Task 02
checks passed: 119 focused web tests, web typecheck, desktop topbar/sidebar chrome
E2E, phone/task/workspace action E2E, existing mobile topbar/sidebar E2E, and
desktop/phone chrome screenshot inspection. Task 03 passed five focused Vitest
files (24 tests), web typecheck, desktop status E2E, phone Status Action E2E, and
the full mobile Status drawer E2E. Browser geometry confirmed the 24px status
bar at desktop and coarse-pointer tablet widths and 44px phone drawer actions;
desktop and phone status screenshots were inspected.

Task 04 checks passed: attributed legacy-shape fixtures, SDK/old-host fallback
tests, mixed-generation lifecycle and status-order coverage, and the managed
Chromium compatibility plus desktop composer suite (7/7). The phone composer
suite passed 3/3, fixture Go tests passed, and the desktop compatibility
rendering was inspected. The existing legacy host `icon-sm` button measures
24px; the raw metric remains 28px. Fixture hashes, base revision, and
release-certification limits are in [the adoption map](plugin-adoption.md).

Task 05 checks passed: plugin SDK tests/typecheck, focused action/compatibility
tests (14), web typecheck, i18n checks, focused ESLint, public documentation
tests (62) and validation (47 pages), document catalog validation (306
decisions/1153 specifications), spec linter tests (36) and full lint, and
`git diff --check`. The managed E2E run also rebuilt the backend, Vite assets,
and packaged fixture.

## Review remediation results (2026-09-25)

Fixed the four static-review findings. Topbar `ActionGroup` now caps its width
and wraps on phones and coarse-pointer layouts while retaining fine-pointer
desktop spacing and the Status drawer's column layout. The phone E2E fixture
uses four Actions, including two long values; browser checks confirm every
44px control remains within the actual Plugins section and can be tapped. A
scoped important SVG rule makes actual glyph dimensions match each surface
without changing `Button`: 16px composer/topbar/drawer, 14px sidebar, and 12px
inline status. The checks cover both raw SVGs and a component-rendered glyph.

Removed the absolute `/work/...` screenshot output. The compatibility and mobile
Status drawer specs now capture, restore, and verify the previous complete
`system_metrics_display` value in cleanup.

Review verification passed:

- Managed `mobile-chrome` action and Status drawer E2E: 5/5 tests.
- Managed `chromium` action UX and compatibility E2E: 4/4 tests.
- Web typecheck and focused renderer/compatibility Vitest: 3 files, 14 tests.
- Focused ESLint passed. Prettier formatting was applied to the changed TS/TSX
  files. Managed E2E runs rebuilt the Vite app, backend, and fixture package.
- The new phone assertions reproduced the old defects before the style fix:
  an Action extended to x=536 while its Plugins section ended at x=370, and
  nominally 16px SVGs measured 14px.

## PR review remediation results (2026-09-25)

Addressed all seven current-head review threads. Inline status groups are capped
at 18rem, remain on one compact row, and let actions shrink so long values do
not expand over neighboring status items. Disabled action tooltips use a
focusable host wrapper; label-only actions render their label visibly;
ActionGroup reads its context before its empty-child return; and the main
topbar memoizes both its action-surface value and PluginSlot subtree. Removed
the redundant status-bar media-query height class.

Verification passed: focused Action, ActionGroup, status-style, and main-topbar
Vitest (3 files, 19 tests):
`(cd apps/web && pnpm exec vitest run components/plugins/plugin-action.test.tsx components/kanban/main-top-bar-plugin-actions.test.tsx components/actions/surface-action-styles.test.ts)`;
web typecheck `(cd apps/web && pnpm run typecheck)`; focused ESLint and Prettier
on the changed web files; managed Chromium E2E
`(cd apps/web && pnpm e2e:run --project chromium --workers=1 --retries=0 e2e/tests/plugins/plugin-action-ux.spec.ts)` (3/3); `python3 scripts/list-docs.py validate`;
`python3 scripts/lint-spec-files.py --all`; and `git diff --check`.

## Original PR-head CI triage (2026-09-25)

The original PR-head checks reached terminal with 50 passed and 12 failed,
including aggregate checks. Reproduced and corrected the PR-related test
assertion failures; the unrelated timeout and infrastructure failures are
triaged separately below:

- The composer toolbar test expected old `h-7`/`min-h-11` classes on every
  action. Updated it to accept `size-7`/`size-11` square controls or existing
  minimum-size buttons; its focused Vitest suite passed 27/27.
- The sidebar Quick Chat action now renders a visible focus ring. Its older
  E2E test required an outline only; the test now accepts a changed focus ring
  shadow while preserving its silent-focus checks. Managed Chromium passed 1/1.
- Two mobile navigation tests used a substring match for `Files`, which also
  matched the composer action named `Attach files`. Both use exact accessible
  names now. The mobile terminal and HTML preview checks each passed 1/1 in the
  managed runner.
- The unchanged file-tree download test timed out while waiting for a seeded
  worktree file in CI. Its targeted managed Chromium rerun passed 1/1. The test
  has no diff from the PR base.

The backend SQLite retry test failed once in CI, with no backend diff in this
PR. It passed 20 consecutive race-enabled exact-test runs and the full package
race suite passed three times. The documentation coverage job's rerun passed
after GitHub code search returned HTTP 429 on the original attempt. The
backend job rerun did not execute tests because the Go proxy failed to fetch
`k8s.io/kube-openapi` with an HTTP/2 stream error. The latest original-head
snapshot after reruns was 52 passed, 9 failed, and 0 pending; remaining failures
are the original-head E2E/frontend assertions plus the backend download error
and aggregate. The updated tests and checks will run on the next PR head.

## PR follow-up CI remediation (2026-09-25)

The first post-fixup head (`b0b8657ad4fb9f60843546cf757bdd1cd71e0376`) reached
terminal with 55 checks passed and 5 failed. The failures were two backend
gates, two E2E gates, and one E2E shard:

- The E2E shard exposed a real compact-bar sizing defect. A Status-bar Action
  inherited the default Button's coarse-pointer 44px height despite the
  surface's 24px contract. `SurfaceAction` now selects the compact Button size
  for `status-bar`; the managed mobile Status drawer spec passed 2/2.
- The backend shard failed during temporary-directory cleanup in
  `TestManager_SetWorkspacePollMode_PropagatesToPerRepoTrackers`. This test
  starts an asynchronous status refresh and had not stopped its trackers before
  `t.TempDir` removed the repositories. It now registers tracker cleanup before
  teardown. The exact test passed 50 race-enabled repetitions.
- The same E2E shard recorded retry-only failures in file-tree context loading
  and mobile merge-queue recovery. Both focused tests passed locally once with
  retries disabled (1/1 each); the next-head CI run will confirm their status.

The backend and E2E aggregate failures were downstream of the named leaf
failures. No backend product behavior or public authoring contract changed.

## Final PR-head verification (2026-09-25)

The final fixup head `c9280cf237c3d38aa67c25b4af5fbf49eaf5e625` reached a
terminal PR check snapshot with 60 passed, 0 failed, and 0 pending. The
documentation coverage workflow initially hit GitHub Code Search HTTP 429;
rerunning its failed job succeeded. The backend, frontend, E2E, and aggregate
checks all passed on this exact head. All seven review threads are resolved,
the branch is clean, and GitHub reports the PR mergeable with a clean state.

## Risks

- Legacy plugins retain visual differences until their authors adopt Action.
- Pointer/ref adapters must preserve Voice hold behavior and Radix composition.
- Custom glyphs must fit the standard box; existing large artwork is not auto-migrated.
- Existing mobile CSS selectors must not override new Action geometry.
- Tablet status controls retain the existing compact-bar exception, not full touch sizing.
- Latest repository source is not the same as an installed release artifact.
- Automatic live-state migration from an old plugin bundle is outside this host change.
