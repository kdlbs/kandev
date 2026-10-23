---
created: 2026-10-22
status: completed
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-002
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
  - ../../specs/ui/system-design/markdown-walkthrough-ranges.md
legacy_specs: []
---

# Implementation Plan: Markdown Walkthrough Ranges

## Overview

Restore the walkthrough section indicator for rendered Markdown files.
One vertical work order connects target identity, geometry, navigation,
desktop, and phone behavior. The existing browser-parity changes remain intact
on `feature/improve-markdown-pre-a36`.

## Evidence and root cause

Read-only source tracing confirms the missing integration:

- `walkthrough-reveal.ts::revealFileAtLine` writes pending navigation and opens
  the target. `useOpenFileAction` preserves an existing Markdown mode and uses
  Preview for new Markdown files.
- Monaco and CodeMirror walkthrough hooks read the active range, draw the
  indicator, and publish a connector anchor.
- `MarkdownEditSurface` supplies no walkthrough target to
  `HybridMarkdownEditor`. Preview also has no walkthrough integration.
- Preview source attributes and the hybrid engine's public geometry API already
  provide the required mappings. No engine patch is required.

Smallest reproduction: open a walkthrough with a multiline `.md` target. Open
that target in Preview, then Edit. Neither presentation draws the usual blue
section indicator. Source contains the existing indicator implementation.
This reproduction is established by code tracing. No new live UI test ran in
the design turn.

## Scope and assumptions

Confirmed: the user wants the usual blue outline around Markdown walkthrough
sections. Existing navigation retains the chosen mode. Extend that outcome to
Preview and Edit on desktop and phones. Reuse the existing Source behavior.
No material user decision remains unresolved.

In scope: same-file steps, inclusive line ranges, repository matching,
scroll/reflow geometry, connector occlusion, hidden retained presentations,
source preservation, and stale pending-navigation cleanup.

Out of scope: generation prompts, stored walkthrough schemas, external PR
walkthrough HTML, semantic step rebasing after arbitrary source edits, engine
upgrades, and changes to the card or sheet navigation design.

## Technical approach

Follow [the range design](../../specs/ui/system-design/markdown-walkthrough-ranges.md).
Use a file-scoped target bridge at `MarkdownFileEditor` and its mobile
composition. Pass the resolved range to Preview and hybrid adapters. Source
continues through the existing provider hooks.

Preview uses source-tagged block bounds. Edit uses public range rectangles.
Both retain a single noninteractive section indicator and publish the existing
connector anchor only while active. Use the existing primary-color styling.
Do not move the editing selection to obtain geometry.

Add conditional acknowledgement of walkthrough-owned pending navigation in
`file-editor-cursor.ts`, with explicit owner and request identity. Preserve
search and LSP requests, repository/session keys, and normal Source consumers.
Resolve mode-restoration ordering so the latest step reveals once and a hidden
editor cannot reclaim the anchor.

## UI preview

**UI-01: Desktop Markdown walkthrough.** Entry: an active walkthrough step
opens a Markdown file. The existing file toolbar remains above the scrollable
document. The referenced section gains the same blue outline and light blue
background as Source. The floating card connects to the section when visible.
Preview/Edit/Source controls retain their current order and selected mode.

**UI-02: Phone Markdown walkthrough.** Entry: the same step opens mobile Files.
The fixed file header retains Back, mode, and Save. The document shows the
outlined section above or behind the existing bottom sheet. The sheet retains
its explanation and fixed Previous/Next footer. When the sheet covers the
target, only the connector disappears. No extra scroll container or control
is added.

Before: Preview and Edit show the document without a walkthrough range.
After: every supported mode identifies the active section. These structural
outcomes are required by `.16` through `.19`. Exact spacing remains owned by
the existing components. The task's rich-output preview shows these two views.

## Tests

Task 01 owns named regressions for `.16` through `.19`: inclusive offset
mapping, nested Preview blocks, scoped active targets, same-start/different-end
steps, hidden editor cleanup, pending navigation ownership, and source/history
preservation. It runs the existing Source walkthrough tests as compatibility
checks. Permanent tests start only in the implementation turn.

## E2E tests

Extend `tests/task/markdown-file-editing.spec.ts` and
`tests/task/mobile-markdown-file-editing.spec.ts` with walkthrough fixtures from
the existing review specs. These files already run in the configured native
browser projects. Assert bounds and computed indicator styling, not only test
element presence. Cover default Preview, retained Edit, same-file Next/Previous,
mode switching, scrolling, repository identity, and closing the walkthrough.

Desktop runs Chromium, Firefox, and WebKit. Phone runs mobile-chrome and
markdown-mobile-webkit. Existing review walkthrough specs remain compatibility
checks for Source, floating-card behavior, and bottom-sheet occlusion.
Physical device acceptance is recorded separately from browser emulation.

## Work orders

- [x] [Task 01: Restore Markdown walkthrough indicators](task-01-rendered-ranges.md).

## Verification results

Implementation and automated verification are complete on the existing branch.
The final focused suite passed 216 tests in 22 files. Typecheck, i18n,
production/unit ESLint, formatting, specification validation, documentation
coverage, and whitespace checks passed. E2E ESLint has two existing
nested-ternary warnings and no errors.

All 58 Markdown browser/project cases pass across desktop Chromium, Firefox,
WebKit, phone Chromium, and phone WebKit. The 13 existing Source walkthrough
cases also pass. Chromium results combine the full runs with focused reruns for
the corrected fixture and final geometry; unchanged cases were not repeated.
The [work order](task-01-rendered-ranges.md#results) records commands, failures,
corrections, and logs.

The isolated evaluation runtime was refreshed with the final build. Its
four-step walkthrough covers early text, a heading/table, below-fold text, and
the secondary repository. Firefox desktop and WebKit phone passed 16 live
mode checks, four navigation checks, and four strict boundary checks. Desktop
screenshots confirm the outline excludes preceding content. The phone sheet
obscures the screenshots, so phone proof uses geometry and occlusion assertions.
All 66 fixture hashes and the database were preserved; main port 9998 retained
its original process. The workspace testing guide remains untracked/unstaged.

Physical-device, IME, and assistive-technology checks remain in the separate
[browser-parity package](../markdown-browser-parity/plan.md). This delivery does
not claim those checks passed. No commit or push was performed.

Design and final checks: catalog validation (369 decisions and 1494 specs),
full specification lint, the linter's 36 tests, local links, and documentation
coverage passed. Public documentation checks passed 63 tests and validated
47 pages. No additional product copy was introduced.

## Risks

- Retained hidden editors can overwrite or clear the active connector.
- Nested Preview blocks and hidden Markdown syntax can inflate section bounds.
- Source-mode restoration can race with a pending walkthrough reveal.
- Async images and diagrams can move an outline after its first measurement.

## Delivery constraints

Keep this package and `TESTING-PR3113.md` uncommitted. Do not create a branch.
Implementation does not authorize commit/push or modifying the main `:9998`
instance. Add manual checks to the existing guide after the targeted tests pass.
