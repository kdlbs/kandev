---
created: 2026-09-05
status: completed
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
legacy_specs: []
---
# Implementation Plan: Script-Capable Native HTML File Preview

## Overview

Rework the HTML preview prototype around a capability-free script runtime
before treating the feature as ready. Inline JavaScript will run in a dedicated
Web Worker ECMAScript VM with a virtual DOM and bounded host API. A scriptless
controlled renderer will display data-only snapshots and enforce the same
resource and navigation policy for static and dynamically-created content.

The existing preview-state migration, UI placement, localization, and static
navigation normalizer are reusable work. The current static-only iframe
implementation and its inert-script tests are not final evidence and remain
non-ready until the new execution boundary is implemented and reviewed.

## Scope

### In scope

- `.html` and `.htm` preview eligibility for editable text files.
- Format-neutral rendered-preview state with legacy Markdown restoration.
- A capability-free worker runtime for inline ECMAScript and virtual DOM
  mutation.
- A scriptless safe renderer for runtime snapshots and user-event bridging.
- Deny-by-default static and dynamic resource and navigation policies.
- Preview and source toggles in Monaco, CodeMirror, center-panel file tabs,
  Dockview file editors, and the focused mobile file viewer.
- Complete localized runtime failure and source-recovery copy.
- Public guidance for inline scripts, embedded resources, and blocked
  workspace or remote resources.
- Focused unit, component, desktop E2E, mobile E2E, and desktop WebView smoke
  evidence for script execution and isolation.

### Out of scope

- Workspace-relative or remote assets.
- Full browser API compatibility, browser storage, workers, service workers,
  WebGL, credentialed integrations, or multi-file site hosting.
- Backend workspace-preview routes or a Kandev control-plane API for scripts.
- HTML preview reconstructed from Review diffs.
- Browser-panel inspector annotations, source-console forwarding, or rendered
  element source-line comments.

## Technical approach

### Capability-free execution boundary

Implement `PreviewRuntimeWorker` with a pinned ECMAScript engine that accepts
no host imports. Expose only virtual DOM nodes, bounded timers, supported
events, console diagnostics, and runtime-owned blob tokens. Do not use native
`eval`, `Function`, script elements, inline event-handler attributes, or an
iframe with `allow-scripts`.

The runtime message contract carries only the file buffer, sanitized events,
data-only snapshots, and diagnostics. It carries no task, session, repository,
boot, cookie, credential, or Kandev store data. Enforce instruction, wall-clock,
heap, timer, event-queue, and snapshot limits and terminate the worker on
violation.

### Snapshot renderer and policy

Render snapshots through a scriptless controlled surface. Create native nodes
from an allowlisted projection rather than inserting source HTML into the
Kandev DOM. Filter static and dynamic resource URLs to `data:` and
preview-runtime-owned `blob:` values. Make links, forms, meta refresh, window
opening, downloads, and all navigation APIs no-ops.

Retain the existing document-builder link and meta-refresh normalization as
defense in depth. The virtual DOM mutation policy is authoritative after
scripts run.

### File surfaces and compatibility

Reuse the existing generic preview state and legacy `markdownPreview` read
bridge. Replace the current static iframe contract in `HtmlPreviewContent` with
the runtime lifecycle and snapshot renderer. Keep Markdown rendering,
comments, editor selection, source restoration, mobile identity reset, and
other file actions unchanged.

### Public guidance

Update `docs/public/developer-tools.md` only when the runtime-backed behavior
ships. Explain that inline scripts run in a restricted preview runtime, that
embedded data and preview-owned blob resources are supported, and that
workspace-relative or remote resources and full browser APIs remain outside the
contract.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-UI-NATIVE-HTML-PREVIEW-001.1`, `.2`, `.6`, `.7`, `.8` | Existing preview-kind, toolbar, state-restoration, mobile, and Markdown-preservation unit/component coverage, updated for the runtime-backed renderer |
| `AC-UI-NATIVE-HTML-PREVIEW-001.3`, `.4`, `.10` | Runtime VM, virtual DOM, message-contract, budget, failure, and snapshot-renderer tests prove script capability without native browser execution |
| `AC-UI-NATIVE-HTML-PREVIEW-001.5`, `.9` | Static and dynamic resource/navigation policy tests cover links, SVG `xlink`, forms, meta refresh, location, history, fetch, XHR, WebSocket, and window APIs |

## E2E tests

- `apps/web/e2e/tests/chat/html-preview.spec.ts` runs under Chromium. It
  renders an unsaved HTML buffer, proves an inline script changes visible
  output, dispatches a user event through the runtime, and attempts dynamic
  network and navigation actions. It proves the preview remains visible and
  no preview-originated request occurs.
- `apps/web/e2e/tests/task/mobile-html-preview.spec.ts` runs under
  `mobile-chrome`. It proves the runtime-backed preview entry point, script
  output, source recovery, 44-pixel touch target, and document containment.
- Desktop WebView smoke coverage repeats the no-request and no-navigation
  assertions before release because WebView policy behavior differs from
  Chromium.

## Work orders

### Script-capable delivery

- [completed] [Task 05: Build the capability-free preview runtime](task-05-script-capable-preview-runtime.md)
- [completed] [Task 06: Integrate the runtime with the preview state and renderer](task-06-preview-state-and-renderer.md)
- [completed] [Task 07: Wire responsive preview surfaces](task-07-responsive-preview-surfaces.md)
- [completed] [Task 08: Publish script-capable preview guidance](task-08-script-capable-preview-guidance.md)
- [completed] [Task 09: Prove script execution and isolation in browsers](task-09-script-capable-preview-e2e.md)

### Superseded static prototype work

These work orders describe the non-final static-only contract that is present
in the current PR. They are retained for traceability, but their results do
not satisfy this plan:

- [cancelled] [Task 01: Establish preview state and sandbox contract](task-01-preview-state-and-sandbox.md)
- [cancelled] [Task 02: Add responsive HTML preview surfaces](task-02-responsive-html-preview.md)
- [cancelled] [Task 03: Publish HTML preview guidance](task-03-public-html-preview-guidance.md)
- [cancelled] [Task 04: Prove responsive HTML preview flows](task-04-responsive-html-preview-e2e.md)

## Verification results

The runtime, renderer, component, localization, public-doc, Chromium, mobile,
and desktop WebView checks pass. The license catalog identifies `parse5@7.3.0`
and `quickjs-emscripten@0.32.0` as MIT-licensed. The public-doc validator
reports 43 published pages, and the specification linter passes all files.

## Risks

- The chosen ECMAScript engine and virtual-DOM compatibility surface may add
  bundle weight or fail on one of the supported desktop WebViews.
- The renderer must preserve useful HTML and CSS behavior without an unsafe HTML
  sink or an accidental native event handler.
- Runtime-owned blob URLs need explicit byte ownership and cleanup.
- Execution budgets must stop denial-of-service scripts without making normal
  inline interactions unusable.
- Dynamic CSS and DOM mutations may reintroduce URLs or navigation paths that
  static source normalization cannot see.
- Browser and WebView evidence must cover both request suppression and frame
  persistence. A CSP-only or parent-load-only assertion is insufficient.

## Resolved design choices

- QuickJS via `quickjs-emscripten@0.32.0` supplies the pinned MIT-licensed
  ECMAScript VM. `parse5@7.3.0` supplies the pinned MIT-licensed parser.
- The first capability set is the virtual document, inline styles and scripts,
  bounded timers and microtasks, five user-event types, and owned blob values.
- The initial limits are 250,000 instructions, 250 milliseconds, 8 MiB VM
  memory, 512 KiB stack, 16 timers, 32 queued events, and a 512 KiB snapshot.
- The visible renderer uses an open Shadow DOM surface and never executes
  source-controlled markup or scripts in the browser.
