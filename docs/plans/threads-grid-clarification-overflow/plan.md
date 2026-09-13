---
created: 2026-09-13
status: draft
requirements:
  - REQ-UI-THREADS-DECK-004
  - REQ-UI-THREADS-DECK-005
system_design:
  - ../../specs/ui/system-design/threads-conversation-deck.md
  - ../../specs/ui/system-design/threads-saved-views.md
legacy_specs: []
---

# Implementation Plan: Threads Grid Clarification Overflow

## Overview

Restore wheel and touch access to every required question option and action
inside a bounded Threads footer. One sequential work order covers the narrow
containment correction and its desktop/mobile regression evidence.

This is a conformance repair to the reviewed [Threads layouts package](../threads-layouts/plan.md),
particularly [Task 02](../threads-layouts/task-02-render-grid.md),
[Task 03](../threads-layouts/task-03-composer-disclosure.md), and
[Task 06](../threads-layouts/task-06-presentation-polish.md). Their recorded
results remain historical; this package does not reopen or replace them.

Diagnosis and this package are complete. Production implementation and permanent
tests require a later explicit implementation request under `/fix`. No PR has
been opened, and no merge or queue action belongs to this package.

## Scope

### In scope

- Let vertical scrolling pass from the existing clarification scroll region
  through its clipped wrapper to the existing bounded Threads footer.
- Cover auto-hide on and off, actual option/Next/Submit activation, keyboard
  access, stable sibling tiles, and existing transcript/CI behavior.
- Preserve Columns, normal task chat, clarification resizing/collapse, and the
  phone's one-conversation composition, touch input, and safe areas.

### Out of scope

Grid sizing/fallback redesign, new scroll containers, extra response renderers,
new state or APIs, persisted preferences, arbitrary tile resizing, mobile
composition changes, agent lifecycle changes, parent PR/branch writes, and
additional tasks, sessions, or delegated agents.

## Evidence and requirement conformance

The [diagnostic report](evidence.md) records the original user wording, current
main/merge verification, browser setup, raw geometry, and a browser-only causal
probe. The defect reproduces at 100% and real 90% Chromium browser zoom with
both auto-hide states. The implementation violates active
`AC-UI-THREADS-DECK-005.5` and the short-tile reachability outcome in `005.6`.
The existing design's [Composer geometry](../../specs/ui/system-design/threads-conversation-deck.md#composer-geometry)
already requires bounded scrolling and reachable question actions, including
auto-hide off. No requirement, acceptance ID, design boundary, or ADR needs to
be created or changed.

Preservation criteria are `AC-UI-THREADS-DECK-004.1`, `004.6`, `004.7`, and
`005.1`, `005.3`, `005.6`, `005.7`, `005.8`, `005.9`, `005.11`.
Saved-view presentation semantics remain owned by the existing saved-views
requirement/design pair; no settings writes are part of the correction.

## Technical approach

`TaskChatPanel` places `ClarificationPanelSection` and `ChatFooter` inside
`ComposerFooterAllocation`. The footer is correctly bounded to the tile body
minus an 80px transcript floor. The question's existing inner scroll region
is capped at 50vh, which can exceed a half-height tile's footer.

The intermediate `clarification-overlay-container` has `overflow: hidden` and
`overscroll-behavior: contain`. Once the inner scroller reaches its own bottom,
this clipped wrapper blocks input from reaching the still-scrollable footer.
The footer remains at scrollTop 0, so the final option and Next stay clipped.

In `apps/web/components/task/chat/clarification-panel-section.tsx`, use the
existing optional `useComposerDisclosureContext` to identify a Threads host.
Scope a vertical-only scroll-chaining allowance to that host. Check context
presence, not `disclosure.enabled`: auto-hide-off and touch Threads also have
the bounded footer and reproduced/implicated path. Keep non-Threads behavior
unchanged. Do not change the 50vh cap, drag clamp, minimum height, collapse
state, or question renderer.

`ComposerFooterAllocation` remains the tile's outer vertical containment
boundary (`overflow-y-auto`, `overscroll-contain`, and the 80px reserve). The
existing resizable question retains its local scroll region; its input must
continue into the footer at its boundary. This restores the existing scroll
chain without a new scroll owner, wheel handler, resize observer, or transcript
scroll loop. Leave horizontal containment and deck navigation unchanged.

Required-action reporting already forces the native composer open. The
question is a sibling of `ChatFooter`, outside its disclosure animation. Do
not change pending-action state, disclosure timing, CI placement, or input
mounting to repair a vertical scroll-chain defect.

## Mobile design contract

Entry remains `/threads` through the shared listing navigation. The nearest
shipped exemplar is the current Threads phone composition in
`ThreadColumn`/`ThreadsBoard`, using the same focused-conversation approach as
`MobileColumnTabs` and the existing thread/session picker sheets. Title and
status precede one transcript; required questions and the normal composer
remain inline. Primary outcome is answering the question inside that surface.

Retain this full-height composition because answering is part of the active
conversation. No extra drawer or task navigation is required. The existing
dynamic-height app shell, safe-area handling, visible touch composer, and
single active transcript remain authoritative. The bounded footer owns the
outer allocation/containment; the existing resizable question's local scroll
region must chain into it. Reuse all question state and submission logic.
Phone/coarse-pointer targets retain their current sizing; assert at least
44px for option rows and primary Submit, plus actual hit targets and viewport
containment for every exercised action. Do not enlarge desktop controls.

## ASCII UI preview

UI-01: Short desktop Grid, long pending question, auto-hide on or off.

```text
BEFORE                              AFTER
+-------------------------+         +-------------------------+
| Task A            [Open]|         | Task A            [Open]|
| transcript (80px floor) |         | transcript (80px floor) |
| Question / options     |         | Question / options      |
| inner scroll stops     |         | scroll continues into   |
| [final option clipped] |         | bounded footer          |
| [Next unreachable]     |         | [last option] [Next]    |
+-------------------------+         +-------------------------+
| Task B: stable tile     |         | Task B: stable tile     |
+-------------------------+         +-------------------------+
```

Header/tile geometry is fixed. The footer scrolls vertically within its
allocation; options, Next, and the existing header Submit can each be scrolled
into view. They need not all fit simultaneously. The question header is not
made sticky and no action moves. The drawing's spacing is illustrative; the
scroll path and stable containment are required (`005.5`, `005.6`, `004.1`).

UI-02: Phone with Grid saved; one active conversation remains.

```text
[Threads / View] [1/2] [Menu]
[Task A v]            [Open]
[one transcript            ]
[inline required question  ]
[scroll to option / Submit ]
[normal composer           ]
[safe-area clearance       ]
```

Keep swipe/picker navigation and touch targets. Vertical question input reaches
the same bounded footer; there is no desktop Grid mounted behind this surface
(`004.6`, `005.7`, `005.8`). Tests cover a normal Pixel 5 viewport, short 393x500
viewport, and the existing coarse-pointer tablet path.

## Tests

Existing clarification panel/resize, composer disclosure, and transcript resize
unit tests remain the compatibility checks. The regression is rendered browser
scroll behavior, so its RED evidence must come from actual wheel input and hit
geometry, not a class-name assertion or an auto-scrolling locator click.

## E2E tests

| Flow | Acceptance criteria | File/project |
| --- | --- | --- |
| Short Grid, long multi-question bundle, auto-hide on/off; wheel reaches final option and Next; all answers submitted | `005.5`, `005.6`, `005.9` | `threads-composer-disclosure.spec.ts`, chromium |
| Keyboard navigation/activation; Columns compatibility; current CI, history/bottom/frozen scrolling, sibling geometry and disclosure scenarios retained | `004.1`, `005.1`, `005.3`, `005.6`, `005.9`, `005.11` | Same desktop file |
| Phone long question via touch scroll, final-option/Submit hit containment; short viewport and single detail; touch composer/settings preserved | `004.6`, `005.7`, `005.8`, `005.9` | `mobile-threads-composer-disclosure.spec.ts`, mobile-chrome |
| Normal task question/carousel and 50vh drag/reset behavior | `005.9` | `chat/clarification-resize.spec.ts`, chromium |

Keep the existing scenario coverage and add to it. Reuse `/e2e:clarification-multi`
with a second admitted task: one task deliberately fills the available height
and would miss the half-height Grid regression. The current three-question
fixture already exceeds the affected allocation. Shared setup and geometry/
gesture helpers belong in `threads-presentation-helpers.ts` or a narrowly
scoped sibling helper if its size warrants it. Task 01 assigns the shared
question/gesture/native-zoom helper to `threads-clarification-helpers.ts`.

The work order defines exact commands and the native zoom recheck. A viewport
approximation may supplement permanent automation but must be labelled and
cannot be reported as browser zoom. No deviceScaleFactor or CSS zoom substitute
is accepted as proof of the user's 90% condition.

## Work orders

- [ ] [Task 01: Restore required-question scrolling](task-01-restore-question-scrolling.md)

One sequential work order; no dependencies beyond the already merged parent.
No delegation is authorized.

## Verification results

Planning evidence only: fresh backend/Vite/plugin fixture build passed;
existing selected-session clarification baseline passed 1 test; the temporary
native-zoom diagnostic passed 1 instrumentation scenario containing four
baseline cases and one causal probe. The first diagnostic attempt failed at
browser configuration before application behavior (inherited deviceScaleFactor
with viewport null); the corrected run used native zoom without that emulation.
See [exact commands/results](evidence.md#validation).

Production RED/GREEN, affected desktop/mobile suites, keyboard submission,
full action containment, and regression checks remain pending Task 01.
Document validation passed: the catalog validated 267 decisions and 868
specifications; all specification files passed lint; `git diff --check` passed.
Local requirement/AC and artifact-link validation also passed. The hook receipt
is recorded in the task plan/handoff.

## Risks

- Gating by auto-hide enabled would leave auto-hide-off and phone cases broken.
- Removing containment globally would change other chat hosts. Override only
  vertical chaining within Threads; keep the outer footer containment.
- Locator click/focus/scrollIntoView can scroll ancestors and mask this defect.
  Require actual gesture input and geometry before activation.
- A large wheel delta may overshoot an option after chaining works. Use bounded
  gestures until the entire target is within its visible owner, then click.
- Native zoom has fractional geometry. Use sensible CSS-pixel tolerances and
  record actual zoom/viewport; physical trackpad and mobile keyboard behavior
  still require hardware validation if a platform-specific failure remains.
