---
status: draft
system: office
requirements:
  - REQ-OFFICE-CONFIG-EXPORT-001
---

# Office Configuration Export

## Mapping and boundaries

Office config owns preview/download. AC-OFFICE-CONFIG-EXPORT-001.1 maps to routes
and UI state; .2 to the canonical manifest and ZIP; .3 to request identity; .4 to
phone composition. Import, sync mutation guards and secret exclusions remain
unchanged. Existing full ZIP clients remain compatible.

## Evidence

`src/office-routes.tsx` lacks `/office/workspace/settings/export`, although
`app/office/workspace/settings/export/page.tsx` exists. Both live backend endpoints
returned 200 during investigation and the 34-entry ZIP passed integrity checks.
The existing frontend reconstructs YAML and paths using `toYamlLike`; those paths
even omit the backend ZIP's `.kandev/` prefix. `handleExport` ignores selectedPaths.
Restoring the route alone is insufficient.

## Canonical manifest and download

Refactor `config/service.go:bundleToZip` to consume one canonical server-generated
file manifest. Each entry has exact archive-relative path and UTF-8 YAML/markdown
content, using the existing Go serializer and export exclusions. Reject duplicate,
absolute or parent-traversing paths while building the manifest; never read arbitrary
filesystem paths from a selection. Do not implement a second browser serializer.

Add optional `files` and `revision` to GET `/workspaces/:wsId/config/export` while
retaining `bundle`. Revision is a deterministic digest of sorted paths and exact
contents. Existing GET `/config/export/zip` continues exporting all current files.
Add POST `/config/export/zip` accepting `{paths: string[], revision: string}` and
returning an attachment. Validate nonempty, bounded, unique selection against the
newly generated authorized workspace manifest and require a matching revision.
A stale revision returns 409 and prompts reload; invalid selection returns 400.
Keep POST scoped through the existing Office workspace middleware and include
it in route-completeness tests. Download via the authenticated API client as a
blob, create a temporary object URL, trigger an anchor download, then revoke it.
No new package, export persistence, server-side file cache or secret surface.

## Routes and request identity

Register the existing ExportPage in OFFICE_ROUTES. ExportPreview consumes server
files, resets loading/error/files/selection/preview on workspace change, and drops
late responses by workspace/request identity. No-workspace is an explicit state.
Only enable download when the displayed manifest belongs to the current workspace,
loading is false, selection is nonempty and no download is pending. Retry reloads
the current workspace. Download uses the captured manifest revision/workspace;
a workspace switch cancels or discards that response and cannot download old data.
409 keeps a visible stale-preview message and requires an explicit reload before
new download; other failures retain selection and provide retry.

## Desktop and phone

Desktop preserves the file list plus preview with the download action above.
Phone entry is Preferences > Export through the existing Office navigation. Show
one file list with selection controls and export action; tapping a filename opens
a full-height preview with Back to files. It is primary file content, not a nested
picker drawer. Reuse the direct-navigation pattern from the mobile UI language and
the existing page-shell navigation. Shared manifest/selection state survives back.
Each view has one scroll owner, safe-area padding, contained code scrolling and
44px touch targets; no two narrow side-by-side panes. Empty/load/error states and
all application copy use locale catalogs.

## Validation

Backend tests compare manifest bytes with ZIP entries including YAML punctuation,
multiline values, empty bundles and every entity kind. Test revision conflicts,
unknown/duplicate/unsafe paths, exclusions and cross-workspace authorization.
Frontend tests cover reset, response races, no-workspace, retry, zero selection and
blob cleanup. Desktop/mobile E2E navigate from Preferences, preview a file,
uncheck one entry, download and inspect ZIP contents against displayed selection.
Tests also reload the direct URL and exercise a download failure. Public docs
explain export and distinguish it from config-sync mutation.
