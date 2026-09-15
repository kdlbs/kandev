---
status: pending
created: 2026-09-15
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-001
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-002
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-003
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-004
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback.md
legacy_specs:
  - ../../specs/ui/requirements/browser-inspect-annotations-save.md
---

# Implementation Plan: Interactive Web Preview Feedback

## Overview

Replace Browser-panel-local annotations with a task-owned review workflow that
works in both the Browser panel and native HTML-file preview. Users capture text,
DOM elements, or screenshot regions across navigable pages, add required
comments, and review one durable pending collection. The ordinary composer Send
action delivers the current versioned snapshot to the selected Agent session,
using direct delivery or the existing queue, and consumes it only after durable
acceptance.

Implementation follows Red-Green-Refactor inside every work order. The package
first establishes persistence and synchronization, then atomic chat delivery,
shared text and element capture, screenshot capture and attachment ownership,
and final public guidance plus browser proof.

## Scope

### In scope

- A revisioned task-owned preview-feedback collection on SQLite and PostgreSQL.
- Authorized WebSocket CRUD and full-snapshot synchronization.
- Durable screenshot storage through the existing attachment service.
- Exact-reference direct and queued delivery with server-owned formatting and
  atomic feedback consumption.
- One composer context item shared by every session in the task.
- Text, element, and screenshot-region capture in Browser and native HTML-file
  preview surfaces.
- Cross-page pending review and best-effort marker restoration.
- Desktop Popover/panel controls and a phone Drawer with touch capture.
- Localization in all shipped catalogs, public documentation, and focused unit,
  integration, desktop, mobile, and Docker-backed E2E tests.
- Final repository formatting, typecheck, test, and lint requested for the
  implementation phase.

### Out of scope

- Source-line mapping, full browser developer tools, video, full-page stitched
  screenshots, console or network capture, and arbitrary `file://` pages.
- A separate untrusted-content origin or a change to the trusted HTML-preview
  execution model.
- Per-item Run, multi-session broadcast, automatic sending, or task-wide
  conversion of other comment sources.
- Pixel-identical capture of unreadable cross-origin media.

## Technical approach

### Task collection

Add `task_preview_feedback_collections` and `task_preview_feedback` with
task-cascade ownership, monotonic collection revisions, optimistic row versions,
stable order, kind-specific payload validation, and bounded snapshots. Screenshot
rows reference uniquely claimed PNG attachments; staged upload, task feedback,
message, and queue claims use one attachment lifecycle.

Authorized WebSocket actions return complete replacement snapshots and publish
`task.preview_feedback.changed`. A dedicated frontend slice keeps one snapshot
per task and does not use the session comment store or browser storage.

### Message admission

Extend the plan-comment admission envelope with `preview_feedback_refs`. The
backend resolves exact task-owned versions and creates a visible
`### Web Preview Feedback` prompt block. Screenshot comments include their page
context and PNG attachment. The final direct or queued repository transaction
claims all attachments, persists delivery, conditionally removes referenced
feedback, and increments collection revisions. Rejection and conflicts leave
the pending set and composer untouched.

### Shared preview capture

Refactor the injected inspector into a versioned selection bridge. It reports
bounded text, element, page, and region metadata to the parent window and accepts
marker projections for the current route. `BrowserPanel` uses the existing
localhost proxy rewrite. `HtmlPreviewContent` recognizes its already-proxied URL
as inspector-capable without routing it a second time.

Kandev-owned chrome provides capture modes, localized comment editing, upload
state, pending review, and errors. The parent validates iframe events by source
and schema. Element HTML renders only as escaped code.

### Screenshot regions

Add `html2canvas` to the web application and rasterize the selected rectangle
from the same-origin proxied iframe document. Downscale output above the pixel
bound, encode PNG, and require confirmation through a thumbnail before upload.
Save uploads the image, then atomically creates the feedback claim. Capture or
save failure keeps the local draft and creates no incomplete row.

### Responsive composition

Desktop uses one **Annotate** control in preview chrome, an anchored capture
draft Popover, and a pending panel grouped by page. Phone uses one touch-sized
header action and a bottom Drawer for modes, draft editing, and the list.
Screenshot drag mode temporarily owns iframe gestures; every other state keeps
ordinary page navigation and scrolling.

## ASCII UI preview

The control hierarchy, one task-wide pending count, required comment, ordinary
composer Send, and phone Drawer are requirements. Exact spacing, icons, colors,
and row truncation are illustrative and must use existing Kandev tokens and
localized copy.

### UI-01: Desktop preview annotation

Entry point: Browser panel or rendered HTML-file preview. The toolbar is fixed;
the page owns iframe scrolling. Covers `AC-TASKS-WEB-PREVIEW-FEEDBACK-002.1`
through `.5`, `.7`, and `AC-TASKS-WEB-PREVIEW-FEEDBACK-004.1`.

```text
┌ Browser / index.html ───────────────────────────────────────────────────────┐
│ [ local page or file ]                      [Refresh] [Annotate (3) ▾]      │
├─────────────────────────────────────────────────────────────────────────────┤
│                         interactive page (scrolls)                          │
│                                                                             │
│        ┌──── button#save.primary ──────────┐       ②                      │
│        │             Save                  │                               │
│        └───────────────────────────────────┘                               │
├─────────────────────────────────────────────────────────────────────────────┤
│ Pending feedback (3)                                           [Clear all] │
│ /products  ① Text: “Choose a plan”       “Increase contrast”       [Edit] │
│ /products  ② <button.checkout>           “Align with price”        [Edit] │
│ /account   ③ Screenshot  820 × 360       “Form clips here”         [Edit] │
└─────────────────────────────────────────────────────────────────────────────┘

Annotate menu                         Capture draft Popover
┌─────────────────────┐              ┌────────────────────────────────────┐
│ Select text         │              │ Element: <button.checkout>         │
│ Select element      │              │ Comment *                          │
│ Screenshot region   │              │ [ Align with price...            ] │
│ Review 3 pending    │              │                 [Cancel] [Add]      │
└─────────────────────┘              └────────────────────────────────────┘
```

### UI-02: Phone preview annotation

Entry point: focused Browser or HTML-file preview. The preview is full height;
the Drawer has one internal scroll owner and safe-area clearance. Covers
`AC-TASKS-WEB-PREVIEW-FEEDBACK-004.2` through `.5`.

```text
┌ index.html ──────────────────────────┐
│ [Show code]              [Annotate 3]│  fixed, 44 px actions
├──────────────────────────────────────┤
│                                      │
│       interactive page (scrolls)     │
│                                      │
│  Screenshot region: drag to capture  │  capture mode only
│                              [Cancel]│
└──────────────────────────────────────┘

┌ Annotation Drawer ───────────────────┐
│ ━                                    │
│ Add feedback                         │
│ [Text] [Element] [Screenshot region] │
│                                      │
│ Pending feedback (3)                 │
│ /products · Text             [Edit]  │
│ /products · Element          [Edit]  │
│ /account · Screenshot        [Edit]  │
│                                      │
│             one scroll area          │
└──────────────────────────────────────┘
```

### UI-03: Ordinary composer delivery

Entry point: any task session composer. Covers
`AC-TASKS-WEB-PREVIEW-FEEDBACK-003.1` through `.6`.

```text
┌ Context ────────────────────────────────────────────────────────────────────┐
│ [Preview feedback · 3]  [Plan comments · 1]                               │
├─────────────────────────────────────────────────────────────────────────────┤
│ Add an optional message...                                      [Send]     │
└─────────────────────────────────────────────────────────────────────────────┘
```

### UI-04: Screenshot draft and recoverable failure

Entry point: after selecting a region. Covers
`AC-TASKS-WEB-PREVIEW-FEEDBACK-002.3`, `.4`, and
`AC-TASKS-WEB-PREVIEW-FEEDBACK-004.4`.

```text
┌ Screenshot feedback ───────────────────────────┐
│ ┌────────────────────────────────────────────┐ │
│ │        captured region thumbnail           │ │
│ └────────────────────────────────────────────┘ │
│ 820 × 360 · PNG                                │
│ Comment *                                      │
│ [ Form clips below the submit button...      ] │
│ Could not upload the image. Try again.         │  visible when failed
│                         [Cancel] [Retry / Add]  │
└────────────────────────────────────────────────┘
```

## Acceptance evidence

| Acceptance criteria | Evidence owner |
| --- | --- |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-001.1` through `.5` | Task 01 repository, service, WebSocket, frontend snapshot, reconnect, and conflict tests |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-003.1` through `.6` | Task 02 direct-message, queue, formatting, attachment, replay, and composer tests |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-002.1`, `.2`, `.4` through `.7` | Task 03 bridge, Browser/HTML preview, candidate hint, marker, panel, desktop, and phone component tests |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-002.3` | Task 04 rasterization, preview, upload, PNG validation, and attachment-lifecycle tests |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-004.1` through `.5` | Tasks 03 and 05 accessibility, mobile, desktop, and Docker-backed E2E |
| All criteria | Task 05 public guidance, focused E2E, and requested repository checks |

## Work orders

- [ ] [Task 01: Persist and synchronize preview feedback](task-01-persist-and-sync-feedback.md)
- [ ] [Task 02: Deliver preview feedback through chat](task-02-deliver-feedback-through-chat.md)
- [ ] [Task 03: Capture text and elements across previews](task-03-capture-text-and-elements.md)
- [ ] [Task 04: Capture screenshot regions as attachments](task-04-capture-screenshot-regions.md)
- [ ] [Task 05: Document and prove the complete workflow](task-05-docs-e2e-and-verification.md)

## Dependency order

```text
Task 01 durable collection and synchronization
        |
        v
Task 02 exact direct and queued delivery
        |
        v
Task 03 Browser and HTML text/element capture
        |
        v
Task 04 screenshot capture and attachment ownership
        |
        v
Task 05 public docs, desktop/mobile/container E2E, final checks
```

The collection exists before message admission references it. Message admission
is complete before UI capture can create user-visible pending data. Screenshot
work follows the shared capture workflow and reuses the already-tested delivery
and task-attachment boundaries. Final E2E and documentation describe only the
settled end-to-end behavior.

## Verification strategy

- Every work order starts with focused failing tests, records the expected RED
  result, implements the minimum behavior, reruns GREEN, then refactors with the
  same focused tests green.
- Backend repository and service tests run with `-race` and cover both SQLite
  and the environment-gated PostgreSQL paths already used by adjacent plan
  comment and attachment tests.
- Frontend Vitest covers pure bridge/raster helpers, task state, message
  submission, and responsive components. Typecheck, ESLint, and i18n checks run
  after the affected vertical slice.
- Managed Playwright runs rebuild backend and web artifacts, then cover desktop,
  mobile, and the Docker-backed executor path with one worker per project.
- The final work order runs `make fmt` first, followed by
  `make typecheck test lint`, exactly as requested. A conventional commit is
  created only after all checks pass and work-order results are recorded.

## Risks

- `html2canvas` cannot reproduce every CSS feature or unreadable cross-origin
  resource. Mandatory thumbnail review makes the resulting PNG explicit before
  persistence; failed rendering never produces a partial pending row.
- Preview content runs under the existing trusted same-origin model. Bridge
  source/schema validation and server authorization must prevent that content
  from choosing task or attachment ownership.
- Combining ordinary files and screenshots can cross the existing ten-file or
  100 MiB message limits. Admission must reject atomically and the composer must
  explain which limit was crossed.
- Exact feedback consumption touches both direct and queued repositories.
  Reusing the established plan-comment lease and replay boundary is required to
  avoid a second, weaker delivery implementation.
- Touch region capture competes with page scroll. Only the explicit screenshot
  mode may suppress iframe drag gestures, and cleanup must run on completion,
  cancel, navigation, or unmount.
- Full-snapshot events can grow with HTML excerpts. Shared count and aggregate
  byte limits must be enforced before persistence in every database dialect.

## Completion gate

The implementation is complete when all five work orders are `done`, their
focused checks and the final requested repository checks pass, public docs
describe the shipped workflow and limits, generated dependency licenses are
current, and the verified changes have a Conventional Commit. The user can then
move the task to Test.

## Verification results

Pending implementation.
