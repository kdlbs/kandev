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

One sequential work order fixes the nested action group's alignment and proves
the mobile workflow move remains usable after wrapping. Implementation was
authorized and completed on 2026-10-09.

## Scope

End justification in the normal chat composer toolbar and targeted browser
coverage. Exclude icon overflow redesign, passthrough changes, new text, workflow
logic, and backend changes. Public docs need no update: this corrects placement
without changing commands, terminology, or user steps.

## Technical approach

Follow `PassthroughStatusRow`'s wrapping action-group pattern in
`ChatStatusBarActions`: end justification with a bounded maximum width. Preserve
DOM order and existing handlers. Extend the workflow move fixture for rendered
coverage, using genuine transcript controls and disposable test data.

## ASCII UI preview

UI-01: Task Chat above composer, idle with the next workflow step available.
Current phone placement is established by the supplied screenshot.

```text
Phone before, crowded:
| [transcript icons...................] |
| [Open PR ->]                         |
| [Composer...........................] |

Phone after, crowded:
| [transcript icons...................] |
|                         [Open PR ->] |
| [Composer...........................] |

Desktop after, sufficient width:
| [status]       [transcript icons] [Open PR ->] |
| [Composer..................................] |
```

Right alignment and control order are required; labels and spacing illustrate
the composition. The existing toolbar/composer stay outside transcript scrolling.
When controls fit, phones retain one line. A hidden action leaves no reserved
second line; movement disables the same action in its current location.
Maps to AC-UI-COMPOSER-ACTION-WRAP-001.1 through .5.

## Tests

Retain `chat-status-bar.test.tsx`'s empty Usage alignment case. Existing
`workflow-move-proceed-button.test.tsx` and `chat-input-area.test.tsx` cover
activation/visibility semantics. Add no test that merely checks the new class.

## E2E tests

Add cases to `e2e/tests/workflow/mobile-workflow-step-move-overrides.spec.ts`
using `seedMoveOverrideFixture`. Restore any changed user settings and use a
disposable workflow. Enable auto-scroll controls and produce enough transcript
content to expose the actual utility controls; assert their presence before
testing wrapping. Use Open pull request for review to force a genuine wrapped
action at phone width without replacing controls with synthetic DOM. Near the
workbench boundary, use Open PR so the label fits the narrower desktop chat pane;
wrapping there is allowed, but right alignment is required in either composition.

For .2/.3, measure line separation, right-edge alignment, full control containment,
44px hit dimensions, and no horizontal overflow at 360px and the configured
phone width. Include a longer fitting label. Check computed end justification at
767/768px. For .4, tap after wrapping and verify the existing move request and
destination. For .1/.3/.5, extend
`workflow-step-move-overrides.spec.ts` with a narrow fine-pointer phone-width
check and wide desktop single-line check. Reuse shared geometry helpers only if
both suites need them.

## Work orders

- [x] [Task 01: Align wrapped workflow action](task-01-align-wrapped-action.md)

## Verification results

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

## Risks

An idle transcript can hide navigation controls and fail to reproduce wrapping.
Tests must prove the wrapped layout before asserting its alignment. A class-only
test would miss this regression. The existing non-wrapping utility cluster may
have separate constraints below 360px; this package does not redesign it.
