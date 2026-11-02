---
created: 2026-10-23
status: implemented
requirements:
  - REQ-UI-MOBILE-TASK-ACTION-BAR-001
system_design:
  - ../../specs/ui/system-design/mobile-task-action-bar.md
legacy_specs: []
---

# Implementation plan: Readable PR notices in Threads

## Overview

Repair terminal PR notice allocation in narrow desktop panes through one
work order. UI owns this reusable presentation contract. Tasks and Integrations
retain archive operations and PR state authority.

## Evidence and root cause

The supplied screenshot shows the merged PR notice for PR #4702. Its message
wraps into one-character lines beside Archive and dismiss. Transcript controls
and Done occupy the remainder of the same row.

Source trace: `ThreadConversation` embeds `TaskChatPanel`, which renders
`ChatStatusBar` and its shared `ArchiveDismissBanner`. The banner uses
`flex-1 min-w-0`, nonshrinking controls, and `overflow-wrap:anywhere` on its
message. Only `max-md:basis-full` protects its width. A narrow tile inside a
desktop viewport therefore permits the message to collapse. The screenshot
and source establish the allocation defect. No live instance was modified.

Smallest faithful reproduction: use a desktop viewport wider than 768px,
open Threads with a 360px to 384px tile, and associate a merged PR. Make the
transcript utility controls and next-step action visible. The task itself need
not be archived. The screenshot establishes merged status, not a task archive
state defect. Archived-task filtering is outside this repair.

Existing acceptance criteria cover phone geometry but omit narrow desktop
containers. AC-UI-MOBILE-TASK-ACTION-BAR-001.9 closes that gap in the existing
requirement. The paired design defines allocation through the shared banner.
No new architecture decision is required.

## Scope

Repair merged and closed notices in narrow desktop containers. Preserve phone
composition, wide desktop density, localization, archive confirmation, and
task-scoped dismissal. Include the shared passthrough consumer in compatibility
checks. Exclude provider polling, task lifecycle, new copy, toolbar redesign,
new dependencies, and changes to Threads membership or saved preferences.

## Technical approach

Update the shared banner allocation in
`apps/web/components/task/chat/pr-archive-banners.tsx`: a 20rem preferred flex
basis, minimum width of `min(100%, 20rem)`, normal growth, and maximum width of
100%. Preserve phone full-width placement and existing button sizing. Inspect
the nested passthrough wrapper in `passthrough-toolbar.tsx`; adjust its bounded
allocation only if the shared notice cannot receive its minimum there.

Do not add a Threads-only banner or viewport listener. Keep localized text,
PR eligibility, `useTaskArchiveConfirm`, confirmation anchoring, persistence
keys, and dismissal callbacks in their existing owners. Maintain the empty
state with no notice allocation. Keep word wrapping as an emergency fallback.

The first implementation step is a failing browser geometry regression against
the current source. A component test without browser layout is insufficient.

## UI preview

UI-01: Threads tile with a terminal PR and an available next workflow step.
This textual preview follows the task's request to use structured previews
instead of ASCII drawings. Labels and spacing are illustrative.

| Surface | First region | Following region |
| --- | --- | --- |
| Current narrow desktop, from screenshot | One-character message column beside Archive and dismiss | Utilities and Done share that row |
| Proposed narrow desktop | Readable wrapped PR notice, Archive, dismiss | Compact utilities followed by right-aligned Done |
| Existing phone composition | Full-width short PR notice, Archive, dismiss | Right-aligned Done above the utility row |
| Wide desktop with enough space | Notice, Archive, dismiss | Utilities and Done can share the same row |
| Notice absent or dismissed | No notice region | Existing controls reclaim the space |

Readable text, contained controls, and width-driven wrapping are required by
AC-UI-MOBILE-TASK-ACTION-BAR-001.9. Phone ordering and touch targets retain
criteria .1 through .3. Archive cancellation preserves the notice under .4.
The composer and existing Threads footer retain scroll ownership. This repair
adds no scroller or animation.

## Tests

Retain `pr-archive-banners.test.tsx`, `chat-status-bar.test.tsx`, and
`passthrough-toolbar.test.tsx` for eligibility, dismissal, confirmation wiring,
and empty state. Add unit assertions only if implementation changes logic.
Do not test class strings as a substitute for geometry.

## E2E tests

Add `apps/web/e2e/tests/task/threads-pr-notice-layout.spec.ts` on `chromium`.
Reuse `threads-presentation-helpers.ts` to start a real mock-agent primary
session, and `mockGitHubAssociateTaskPR` to establish terminal PR state.
Restore changed settings. Scope locators to the selected tile.

Cover Columns and Grid with 360px, 384px, and 480px tiles inside a desktop
viewport. Assert actual tile widths so a phone-only reproduction cannot pass.
Include merged and closed notices with visible utilities and Proceed.
Measure text width against its longest ordinary English word using its computed
font. Require no broken ordinary word, contained message/control bounds, and
nonoverlapping utilities. Also check a wide host, a long translated message,
dismissal, and Archive cancellation. Map these checks to .4, .6, .7, and .9.

Run existing mobile chat and passthrough cases for .1 through .8. Add
`apps/web/e2e/tests/task/mobile-threads-pr-notice-layout.spec.ts` for the phone
Threads host: readable notice, contained 44px targets, dismissal, and no page
horizontal overflow. Reuse the shipped phone composition. Inspect screenshots
of the desktop failure and corrected desktop/phone layouts during implementation.

## Work orders

- [x] [Task 01: Protect shared notice width](task-01-protect-notice-width.md)

Execution is sequential. No delegation is authorized.

## Verification results

Design validation passed on 2026-10-23:

- `python3 scripts/list-docs.py validate`: 384 decisions and 1616 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: prospective production paths covered, zero errors.
- `git diff --check`: passed. Status confirms two modified specifications and two new plan files, all uncommitted.

Implementation was authorized on 2026-10-23. The browser regression reproduced
a 0px message width before the shared banner allocation changed. After the
one-line production correction, 57 component tests and all 21 desktop browser
scenarios passed across the compatibility and final targeted runs. Typecheck,
ESLint, formatting, and the translation ratchet passed. All 10 mobile browser
scenarios passed with the corrected local font configuration. Desktop and phone
screenshots match UI-01. Exact commands, environment setup, and logs are in the
work order. The package is implemented and verified.

## Risks

- Passthrough's nested wrapper can constrain the shared banner independently.
- A geometry test with hidden utilities cannot reproduce the reported crowding.
- Long translations and touch targets need containment checks in the real host.

## Documentation impact

Internal requirements and design change. Public instructions, labels, navigation,
and screenshots need no planned update for this allocation correction. Existing
phone composition remains the exemplar. Prior completed mobile plan results
remain historical evidence. Current repair validation is recorded above.
