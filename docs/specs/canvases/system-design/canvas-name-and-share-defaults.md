---
id: canvases-name-and-share-defaults-design
title: Canvas naming and quick sharing system design
status: draft
system: canvases
owners:
  - canvases
requirements:
  - REQ-CANVASES-NAME-SHARE-001
  - REQ-CANVASES-NAME-SHARE-002
created: 2026-09-22
last_updated: 2026-09-22
---

# Canvas naming and quick sharing system design

## Requirement mapping

- [Canvas naming and quick sharing requirements](../requirements/canvas-name-and-share-defaults.md):
  `REQ-CANVASES-NAME-SHARE-001` and `REQ-CANVASES-NAME-SHARE-002`.
- Preserve the [agent-authored canvas](agent-authored-web-apps.md) lifecycle
  and [marketplace sharing](marketplace-sharing.md) preparation safeguards.

## Current boundary and observed gap

`canvas_lifecycle_metadata.title` is the instance title. Releases and their
manifest bytes are immutable. The host toolbar in
`apps/web/components/settings/canvas-host-components.tsx` renders the title
but offers no rename action. The browser canvas API has no rename operation.

`CanvasShareDialog` seeds from `canvas.active_release`, but
`canvasReleaseResponse` in `backendapp/canvas_routes.go` drops the package ID,
version, display name, description, and author already available in
`canvas.ReleaseMetadata`. The current example manifest contains those values,
so the blank fields in Share are a projection defect. The release legitimately
lacks a license and minimum Kandev version. `PrepareExport` can fill from the
immutable manifest, but rejects the remaining required gaps only after the
user submits. Client-only fallback would leave the form misleading.

## Host rename contract

Add `PATCH /api/v1/canvases/:canvasID` with `{ "title": "..." }`, returning the
normal canvas metadata response. The route uses the same feature gate and
workspace authorization as other canvas management routes. The canvas service
checks existence, trims and validates with `MaxTitleLength`, and updates only
the lifecycle title and `updated_at`. Use a conditional repository update so
a concurrent removal cannot create a row or report a false success. Keep the
plugin instance and release stores untouched. A rename emits an owner-scoped
`canvas.updated` lifecycle notification after commit; include ID, scope, title,
and timestamp, without manifest or runtime content. Existing canvas list
consumers refresh their projections on this event. A direct read remains the
fallback after a missed event.

Expose the action next to the toolbar title on desktop. It opens a compact
name editor with the current name selected, Save and Cancel, Enter to save,
and Escape to cancel. Disable duplicate submission and keep the draft open on
failure. On phones, place Rename in the host action drawer and use a focused
sheet with 44px controls and safe-area padding. Both presentations use one
host-owned rename state and accessible label. Update the host's canvas state
from the response without rebuilding the runtime iframe; navigation and task
picker projections receive the owner-scoped event. Use localized copy.

## Share defaults and preparation

Add `GET /api/v1/canvases/:canvasID/export-defaults`, authorized through the
same workspace check as export preparation and available only for a valid,
active release. Return `{ expected_release_id, metadata, missing_required }`
without files or secrets. Read the immutable release manifest, not browser
supplied metadata or a mutable task title. Reuse `fillExportMetadata` for
manifest values and source-mode normalization, then supply the first compatible
canvas-distribution version from server-owned version metadata when the
manifest minimum is absent. Never lower a declared minimum. If a valid
server version cannot be resolved, leave the field missing with an explicit
error rather than exporting a placeholder. Do not infer author or license;
description stays empty if the release did not provide one. A canvas title is
only a display-name fallback. Preserve package ID and version from the active
release. Repository URL is optional.

Fix `canvasReleaseResponse` so safe manifest metadata reaches all host views,
but make the export-defaults endpoint the Share dialog's canonical seed and
source of `expected_release_id`. This also avoids a race between a stale
host snapshot and export preparation. Reuse `normalizeExportMetadata` and the
existing validator in `POST /exports`; defaults are convenience only. Keep
existing behavior for direct API callers that omit metadata.

On Share open, load defaults and show release identity plus missing required
fields. Put populated metadata in a collapsed, editable Package details
section. Prepare downloads stays prominent; if a required value is missing,
focus that field and explain it. License remains a visible author choice and
never defaults to MIT or another license. On successful preparation, retain
the existing inventory, size, private-content reminder, and two download
actions. Form edits invalidate and delete the prepared review. On active
release change, discard the prior release's draft, fetch new defaults, and
disable download until prepared again. On close, discard the temporary draft.
On fetch failure, show Retry; do not expose a falsely complete draft.

## Mobile and accessibility

Retain the desktop Dialog and full-height phone Drawer. Each has a fixed
title/action region and one scrollable body. Focus the first missing field or
the Prepare action after defaults load, announce load/failure/review states,
and preserve focus when expanding Package details. The phone view uses
full-width actions, safe-area spacing, no horizontal overflow, and at least
44px touch targets. All host copy is localized in six catalogs; author data
remains untranslated.

## Validation

- Canvas service, repository, HTTP, and event tests: authorized rename,
  whitespace/length rejection, concurrent removal, scope preservation,
  restart persistence, and no release/runtime mutation.
- Distribution tests: old and distribution-profile manifests, missing
  license/author/description, minimum-version fallback and higher minimum,
  static/project mode, stale active release, changed draft, and validation
  error retention.
- Desktop and phone E2E: rename updates toolbar/navigation without iframe
  reload; Share prefills the example release, asks for license, prepares and
  downloads after review, and recovers from stale/failing defaults.
