---
created: 2026-10-09
status: implemented
requirements:
  - REQ-UI-COMPOSER-ACTION-WRAP-001
system_design:
  - ../../specs/ui/system-design/composer-action-wrapping.md
legacy_specs: []
---

# Implementation Plan: Mobile Composer Action Wrapping

## Overview

Task 01 completed the earlier right-alignment correction. The 2026-10-14 user
correction places the phone workflow action above the floating transcript icons.
One follow-up work order implements this revised composition and replaces the
published PR evidence. Implementation was authorized on 2026-10-14. The revised
layout is verified against UI-02; local results are recorded in Task 02.

## Scope

Change normal chat composer presentation and its targeted rendered regression
coverage. Preserve desktop inline order, status/notice placement, shared workflow
handlers, utility order, and disclosures. Exclude passthrough changes, new copy,
workflow/backend logic, and icon overflow redesign. Update the existing
phone-controls guidance in
`docs/public/mobile-remote-access.md` to describe the new row order. Existing
actions, terminology, and user steps remain the same.

## Evidence and root cause

The saved touch-phone PR capture shows utility icons above the next-step button.
`ChatStatusBarActions` renders utilities first and the action last; phone
`display: contents` wrappers preserve that sequence in the outer flex flow.
An auto margin corrects horizontal alignment but cannot create the requested
preceding action row. Reproduction: seed the existing workflow fixture with
transcript history, reveal navigation controls, and view it at 360px or 393px.
No new production reproduction is needed to establish the current row order.

## Technical approach

Update `ChatStatusBarActions` and `ChatStatusBarRightControls` in
`apps/web/components/task/chat/chat-status-bar.tsx`. Use the canonical
`useResponsiveBreakpoint` phone condition. Keep one keyed instance of each
child under the same parent so responsive ordering preserves state. The phone
action group spans the tray width and stacks the right-aligned workflow button
before a full-width wrapping utility row. Keep `TranscriptNavGroup`'s phone
subgroup flattened. Utilities remain complete targets; the new row must not
restore a non-wrapping cluster. Wider viewports retain bounded end justification
and utilities-before-workflow DOM order.

No action placeholder remains when gates hide it. Moving disables the action
in its established upper row. Keep 44px phone/coarse targets and 24px compact
fine-pointer desktop height. Shared state, move transport, and disclosures stay
in their existing components. Transcript scrolling and composer safe-area
clearance remain authoritative; the rows occupy natural height outside the
transcript scroller. See the design's mobile contract for the surface rationale.

## ASCII UI preview

UI-02: Task Chat composer tray, idle with an eligible next workflow step.
The before view is established by the published phone capture. UI-01 in Task 01
records the previous completed alignment revision.

```text
Phone before (current implementation):
| [transcript utility icons...........] |
|                         [Open PR ->] |
| [Composer...........................] |

Phone after (below 768px):
|                         [Open PR ->] |
| [transcript utility icons...........] |
| [Composer...........................] |

Phone moving:
|                [Open PR -> disabled] |
| [transcript utility icons...........] |
| [Composer...........................] |

Phone action hidden:
| [transcript utility icons...........] |
| [Composer...........................] |

Desktop after (sufficient width):
| [status]       [transcript icons] [Open PR ->] |
| [Composer..................................] |
```

The phone action is always above every utility, including with sparse controls
and a short label. Order, separate phone rows, right alignment, no hidden-action
placeholder, and existing notice/status precedence are structural requirements.
Spacing, icons, and labels are illustrative. Long fitting labels can wrap inside
the 44px-minimum button. Utility rows wrap as complete controls. All rows remain
outside transcript scrolling, above existing composer/bottom navigation.
Maps to AC-UI-COMPOSER-ACTION-WRAP-001.1 through .5.

## Tests

Retain the three existing composer component suites. Add meaningful coverage
only for responsive child identity/order and hidden/moving/empty states affected
by the revision. Do not replace rendered verification with class-string tests.

## E2E tests

Revise `composer-action-layout-helpers.ts` so its phone oracle asserts button
bottom <= minimum utility top, and retains right-edge, containment, reachability,
target-size, and overflow checks. The present oracle asserts the opposite line
order and must change before production code for the TDD red gate.

Update the existing workflow regression cases in
`mobile-workflow-step-move-overrides.spec.ts` and
`workflow-step-move-overrides.spec.ts`. Cover 360px, 393px, and 767px with touch
and fine pointers; verify both a longer fitting label with dense controls and a
short label with sparse controls. At 768px allow the narrower desktop chat pane
to wrap while retaining trailing action order; at 1440px prove the existing
single-line order and 24px height. Resize across the boundary, assert one action,
and check keyboard traversal follows visual order. Tap/click the phone action
and assert the existing move request and destination.

Retain `mobile-chat-status-bar.spec.ts` coverage for terminal notices, 320px,
long labels/translations, and coarse tablets. Restore changed user settings and
use disposable fixtures. Run browser projects sequentially. Capture fresh
phone-touch, phone-mouse, and desktop screenshots, inspect against UI-02, and
update PR #4372 through the already authorized delivery workflow after checks.

## Work orders

- [x] [Task 01: Align wrapped workflow action](task-01-align-wrapped-action.md)
- [x] [Task 02: Place workflow action above utilities](task-02-place-action-above-utilities.md)

Task 02 depended on completed Task 01 and is now done. No delegation was used.
Its exact commands and risks are in the work order. Earlier checks below
validate the previous composition only.

## Placement revision verification results

Implementation checks passed on 2026-10-14: 61 component tests, 6 desktop browser
tests, and 8 mobile browser tests, including dense/sparse phone layouts,
keyboard order, disabled/hidden states, and the landed notice/tablet scenarios.
The revised rendered oracle failed before production changed (660px button
bottom versus 566px utility top). Fresh phone-touch, phone-mouse, and desktop
captures match UI-02; each focused capture regression passed again after clearing
transient hover/focus chrome. ESLint, formatting, catalog/spec lint, prospective
complete-PR documentation coverage, and diff checks passed.

Requirement and design lifecycle is active/current. Three compressed screenshots were
published as [native GitHub attachments](https://github.com/kdlbs/kandev/pull/4372#issuecomment-6164987618);
exact-head CI/review confirmation is pending for the push. Full commands and evidence are in
[Task 02](task-02-place-action-above-utilities.md#results).

## Previous implementation verification results

Placement revision package checks passed on 2026-10-14:

- `python3 scripts/list-docs.py validate`: 381 decisions and 1596 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: all files passed.
- `git diff --check`: passed.
- Source, existing test names, and the saved touch-phone capture confirm the
  current utility-before-action order and the targeted reproduction.

At that design handoff Task 02 was pending. Those package checks were not
rendered evidence for UI-02; the completed implementation checks are above.

Design package checks passed on 2026-10-09:

- `python3 scripts/list-docs.py validate`: 368 decisions and 1495 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all files passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`, using these four artifacts
  and the proposed production path: covered, zero errors.
- `git diff --check -- docs/specs docs/plans/mobile-composer-action-wrapping`: passed;
  status inspection confirms the full package is untracked and uncommitted.

Implementation checks passed on 2026-10-09: 60 component tests, 4 mobile browser
tests, 6 desktop browser tests, ESLint, formatting, specification validation,
documentation coverage, and diff checks. The browser regression failed before
the fix with a 138.17px right-edge gap. Rendered phone geometry and its screenshot
match UI-01. Full command results are in the [work order](task-01-align-wrapped-action.md#results).

## Previous implementation risks

An idle transcript can hide navigation controls and fail to reproduce wrapping.
Tests must prove the wrapped layout before asserting its alignment. A class-only
test would miss this regression. The existing non-wrapping utility cluster may
have separate constraints below 360px; this package does not redesign it.


## Current-main reconciliation

On 2026-10-14, preserved the newer phone action-bar composition while retaining
this plan's right alignment and 44px target contract. The browser helper now
measures actual utility-control bounds. Both failed CI cases are remediated:
the fallback-summary test selects its seeded profile owner, and the shared
session menu starts inline rename after menu focus restoration. Component
validation passed 105 tests; browser validation passed 9 desktop and 8 mobile
tests with retries disabled. Detailed red/green evidence and commands are in
the work order. PR #4372 at `5ea252608` subsequently passed 54 checks with 11 skipped, no pending
checks or unresolved threads, and refreshed screenshots. Those screenshots and
checks describe the previous phone order, not the revised placement.
