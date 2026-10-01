---
created: 2026-09-30
status: implemented
requirements:
  - REQ-UI-BOUNDED-CHANGES-001
system_design:
  - ../../specs/ui/system-design/bounded-changes-rendering.md
legacy_specs: []
---

# Fix Plan: Changes Timeline Measurement Refresh

## Overview

Restore contiguous Changes rows after presentation changes invalidate virtual
measurements. Implement the bounded refresh and regression coverage first,
then let CI run the full test matrix and deliver through PR review and merge.
The primary session executes both work orders sequentially.

## Ownership and settled intent

UI owns this reusable geometry contract; task, Git, and provider data ownership
does not change. The screenshot requests consistent commit spacing, and the
user explicitly requested full E2E and merge delivery while AFK. Delegation
and persistent child tasks are not authorized. No material product question
remains unresolved.

This repair implements existing AC-UI-BOUNDED-CHANGES-001.6, with .4, .7, and
.8 guarding anchor, context, and phone behavior. No new requirement or ADR is
needed. The design package was handed off uncommitted at the repository checkpoint;
the user subsequently explicitly authorized implementation and PR delivery.

## Confirmed cause and reproduction

`ChangesTimelineViewport` delegates mounting measurement to
`measureFileTreeElement`, which uses cached sizes or estimates until a positive
observer entry arrives. Its presentation observer calls `virtualizer.measure()`
on width, font, locale, and pointer changes. TanStack virtual-core 3.17.8 clears
`itemSizeCache` without restarting observation of unchanged connected rows.
Those rows therefore retain estimates until another real dimension change or
remount produces a positive measurement. Newly mounted rows can become correctly
measured beside older estimated rows, matching the screenshot's uneven spacing.

The desktop commit estimate is 34px while a compact commit header renders at
24px. Changing that estimate alone would leave the refresh defect in other
variable-height rows and pointer modes.

A disposable Vite fixture used the real viewport, measurement callback,
virtualizer, and application CSS, with 80 synthetic rows in a 500px-high
scroller. The fixture did not access a running Kandev instance or user data.
Checks used a named headless Chrome session and settled animation frames.

| Synthetic geometry | Original after locale/font/width refresh | Temporary candidate |
| --- | --- | --- |
| 24px rows, 34px estimate, desktop viewport | 10px gaps | 0px adjacent gaps |
| Same compact rows after scrolling | Mixed 0px and 10px gaps | 0px adjacent gaps |
| 52px rows, 34px estimate, 393px viewport | 18px overlap | 0px adjacent gaps |

The refined candidate snapshots only mounted positive sizes, clears and rebuilds
measurements, applies the snapshot, then makes the saved anchor available for
restoration. After a font event at scroll offset 180px, compact row 7 retained
its -12px visible offset; tall row 3 retained its -24px visible offset. Compact
scrollTop changed to 200px because unmounted prefix rows returned to estimates;
the visible key and offset remained stable. Mounted rows stayed between 15 and
32. This is diagnostic evidence, not permanent implementation or full-app E2E.

The simplest deterministic regression measures rows once, triggers a font or
locale invalidation without emitting new row observer entries, and asserts
contiguous transforms and the same visible anchor. Existing tests mainly assert
that `measure()` was called and cannot detect lost measured geometry.

## Scope

### In scope

- Restore mounted geometry after presentation invalidation without relying on
  a subsequent observer event for an unchanged row.
- Preserve positive hidden-row fallback, variable heights, focus, visible
  anchors, stable row identities, and bounded mounting.
- Add desktop and phone commit-spacing regressions and compatibility checks.
- Run the full E2E project matrix, record skips and blockers accurately, and
  deliver screenshots, PR checks, reviewer remediation, and merge.

### Out of scope

- Git history ordering, provenance, detail fetching, backend APIs, or data cleanup.
- Changes to shared Files measurement behavior, global CSS density, overscan,
  dependencies, settings, persistence, or feature flags.
- A new phone composition or product copy.

## Technical approach

Add a focused `refreshChangesTimelineMeasurements` helper in
`apps/web/components/task/changes-timeline-measurement.ts`. Ground its types in
the installed virtualizer and keep its inputs limited to the viewport and
virtualizer needed for the refresh. Read only mounted
`[data-changes-timeline-row]` wrappers, resolve current indices and stable keys,
and collect positive actual heights or their previous positive keyed sizes.
Finish layout reads before clearing or publishing any sizes.

Call `measure()`, rebuild the current measurements with the library API, and
apply the captured sizes through `resizeItem()`. In the presentation-change
callback in `changes-timeline-viewport.tsx`, capture the current anchor before
refresh and publish `pendingPresentationAnchorRef` only after sizes are applied.
Keep the normal mount/scroll callback in `file-tree-measurement.ts` intact.
Validate indices and avoid publishing stale disconnected or replaced wrappers.

Extend the controlled observer component tests to keep row observations silent
after invalidation. Test positive sizes, hidden fallback, mixed heights, context
replacement, and anchor restoration. Permanent browser tests use actual commit
headers and the production bundle, with panel-relative bounding-box assertions.

## ASCII UI preview

UI-01: Changes commit history, after resize, font loading, or scrolling.

```text
Observed desktop          Corrected desktop       Corrected phone Changes
Commits                   Commits                 +------------------------+
o abc1234 first commit    o abc1234 first commit   | Changes                | fixed
o def5678 second commit   o def5678 second commit  | Commits                |
                         o 901abcd third commit   | o abc1234 first   [Open]|
o 901abcd third commit    o 234def0 fourth commit  | o def5678 second  [Open]|
                                                  +------------------------+
                                                  | Chat | Files | Changes | fixed
                                                  +------------------------+
```

The shared structural requirement is contiguous measured row wrappers with one
content scroller; spacing in this sketch is illustrative. Phone entry remains
the Changes bottom-navigation button, the existing `MobileChangesPanel` owns
the surface, and commit actions retain their existing touch targets of at least
44px. No new labels, controls, or navigation are introduced. UI-01 maps to
AC-UI-BOUNDED-CHANGES-001.6 and .8.

## Tests

| Proposed regression and location | Criteria |
| --- | --- |
| `changes-timeline-measurement.test.ts`: refreshes unchanged mounted sizes and retains keyed hidden fallback | .6, .8 |
| `changes-timeline-viewport.test.tsx`: keeps rows contiguous without another row observer entry after font/locale/width invalidation | .6 |
| Same component suite: restores the visible key and offset after measurements are repopulated | .4, .6 |
| Same component suite: replacement context cannot reuse old wrapper measurements or anchors | .7 |

Criterion suffixes refer to AC-UI-BOUNDED-CHANGES-001. Work order 01 owns the
exact test commands and desktop/mobile regression matrix.

## E2E tests

Extend `large-changes-virtualization.spec.ts` and
`mobile-large-changes-virtualization.spec.ts`, using shared helpers in
`changes-commit-spacing-helpers.ts` and the existing large-file helpers. Seed collapsed commit metadata through the existing
environment-aware store bridge. Cover more rows than the viewport, scrolling,
font invalidation without changed dimensions, width changes, hide/reopen, and
actual phone commit control sizing at 393px and 767px. Add 768px desktop
boundary evidence with a fine pointer. Check adjacent wrapper gaps within 1px,
visible anchor key/offset, and no horizontal page overflow.

Retain the existing 50,000-file tests, source-navigation tests, and real-Git
checks. Work order 02 originally planned local full projects. The user subsequently
stopped local testing and requested PR publication with CI-owned tests. The
partial local Chromium run, its two scoped setup repairs, and cancelled
remaining projects are recorded there. Skips are not passes.

## Work orders

- [x] [Task 01: Restore measured timeline geometry](task-01-restore-measured-geometry.md)
- [ ] [Task 02: Run full E2E and deliver the repair](task-02-full-e2e-and-delivery.md)

Task 02 depends on Task 01. No parallel implementation is planned.

## Companion packages and documentation

The [completed bounded rendering package](../bounded-changes-rendering/plan.md)
owns the initial virtualization work. This repair preserves its historical
results and adds new evidence here. The shared Files renderer stays under the
[hidden measurement package](../file-tree-hidden-measurements/plan.md).

Public documentation review covered `docs/public/sessions-and-review.md`,
`README.md`, and `docs/screenshots.md`. Public docs do not need changes because
this restores existing row presentation without new controls, terminology, or
workflow behavior. Internal design and delivery records change.

## Verification results

Diagnostic reproduction and temporary candidate comparison completed on
2026-09-30 as recorded above. The named browser and isolated Vite process tree
were stopped, and the disposable fixture/candidate files were removed.

Design-package validation passed:

- `python3 scripts/list-docs.py validate`: 336 decisions and 1,269 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: both work orders cover the
  planned measurement implementation boundary with no reference errors.
- Relative-link, whitespace, and tracked diff checks passed.

The user explicitly authorized implementation after the design handoff.
Task 01 is done: the controlled-observer regression reproduced three failures,
and the bounded refresh passes 43 focused unit tests, web typecheck, changed-file
ESLint, i18n checks, five desktop E2E tests, and thirteen phone E2E tests.
Synthetic captures show contiguous compact desktop rows and phone touch controls.
Task 02 is in progress for PR/merge delivery with testing delegated to CI at
the user's later instruction. The local full Chromium run was cancelled after
969 passes, two test-setup failures, and fourteen skips. Both observed setup
failures were repaired; their validation is pending in CI. Its result record
keeps the partial run distinct from a complete successful suite.

## Risks

- Publishing the anchor before repopulating sizes restores an estimate-only
  offset and moves the visible entry; ordering is part of the regression.
- Reading geometry on every row mount or scroll would reintroduce forced-layout
  work. Keep the explicit reads bounded to presentation refreshes.
- Hidden wrappers can report zero; save positive keyed fallback before clearing.
- Browser unit tests with fake geometry can pass while rendered spacing fails;
  permanent production-build bounding-box tests remain required.
- Full E2E requires browser binaries and, for container projects, Docker/Kind
  prerequisites. Report unavailable coverage and terminal failures explicitly.
