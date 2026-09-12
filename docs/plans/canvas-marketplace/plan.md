---
created: 2026-09-10
status: done
requirements:
  - REQ-CANVASES-MARKETPLACE-001
  - REQ-CANVASES-MARKETPLACE-002
  - REQ-CANVASES-MARKETPLACE-003
  - REQ-CANVASES-MARKETPLACE-004
  - REQ-CANVASES-MARKETPLACE-005
  - REQ-CANVASES-MARKETPLACE-006
  - REQ-PLUGINS-MARKETPLACE-002
system_design:
  - ../../specs/canvases/system-design/marketplace-sharing.md
  - ../../specs/plugins/system-design/marketplace.md
legacy_specs: []
---

# Implementation plan: Canvas marketplace

## Overview

Add canvas distribution to the existing Plugins marketplace. Authors download
an installable bundle and source project, then follow manual repository and
registry instructions. Recipients inspect screenshots and permissions before
installing an independent workspace canvas.

The [requirements](../../specs/canvases/requirements/marketplace-sharing.md) and
[design](../../specs/canvases/system-design/marketplace-sharing.md) are the
authoritative pair. The selected owner is Canvases because it owns reusable
canvas identity, source lineage, workspace instances, and discovery.

Implementation is complete across the seven sequential work orders: portable
package contract, export preparation, reviewed installation, registry
enrichment, marketplace UI, sharing UI, and public instructions. No native
implementation subagents were authorized by this plan. Each work order's
Results section records its implementation and validation evidence.

## Scope

### In scope

- One existing marketplace with a dedicated Canvases tab in Plugins settings.
- `.tar.gz` upload, direct HTTPS bundle link, and catalog installation.
- Required registry cover URL and up to seven additional previews for canvases.
- Optional registry screenshot URLs and the same details gallery for plugins.
- Screenshot-free bundle creation, source downloads, and upload/link installs.
- Package metadata, source mode, license, compatibility, and permission review.
- Download bundle and source ZIP from the same prepared release snapshot.
- Manual repository/release/sharing instructions and PR-based registry listing.
- Independent workspace copies, durable provenance, retry-safe confirmation.
- Static runtime isolation, workspace authorization, quotas, and disabled paths.
- Desktop and phone UI, localization, targeted E2E, author/public documentation.

### Out of scope

- Repository-URL installation, cloning, builds during install, or new hosting.
- Automated code-host publishing, repository creation, or registry PR submission.
- Canvas updates/replacement, state export, credentials, or public live instances.
- Screenshot capture, ratings, payments, telemetry, and a standalone website.
- Runtime toggle promotion, new flags, or native binary canvas execution.

## Baseline and companion plans

The `plugin-backed-canvases` package is marked done. The UX follow-up records
Tasks 01-06 complete and Task 07's external ACP evaluation outstanding.
This plan extends their source-retention and discovery boundaries; it does not
reopen their completed work or change their historical verification results.
Implementation must preserve the one-core-read authoring contract. The external
ACP evaluation is not an additional delivery gate for this feature.

## Technical approach

### Portable package boundary

Extend `internal/plugins/manifest` with optional typed distribution metadata.
Extend `internal/plugins/webapp` with strict distribution validation, checksum
coverage, source-only file retention, and safe
export writers. `Runtime.Serve` denies `distribution/` files. The native
`pkgtar` installer stays unchanged. Add `cmd/canvas-package` for offline
inspection through the production validator; the registry builder uses this
command instead of implementing a second parser.

No-build source remains the current application files. Build-based source is
retained under `distribution/source/` in the same immutable release. Extend
authoring collection and Quick Chat materialization to preserve that subtree.

### Export and installation boundary

Add a small canvas distribution service with expiring, bounded, user-bound
preparations. Export reads the immutable active artifact and derives both
downloads without touching the executor or changing the active release.
Installation stages one inspected package and approves those exact bytes.

Add `InstallPrepared` through the canvas/instance transaction boundary and one
`canvas_install_receipts` table. The receipt preserves provenance and prevents
duplicate confirmation after timeout/restart. Reuse artifact cleanup,
workspace admission, grants, and lifecycle invalidations. Native plugin
registry records and its auto-update poller do not contain canvas instances.

New browser routes live beside `backendapp/canvas_routes.go` in focused files.
Authorize human workspace access at service boundaries; server-side URL fetch
uses bounded, public-destination-only transport. Tests inject a local transport.

### Registry and catalog boundary

Add optional `kind: canvas` to the existing pointer schema. The builder fetches
the exact versioned bundle, inspects it, and requires its archive digest.
Preview URLs come from the registry and pass unchanged into the generated index.
Use the proposed [registry schema](registry-entry.schema.json) for both kinds;
there is no image extraction, mirroring, or Pages media staging. Use additive catalog
fields and kind filtering. Instance annotations are workspace-authorized and
must not enter the shared source cache. Old native entries keep their behavior.

### UI boundary

`PluginsSettings` adds Canvases alongside Installed and Browse. Add domain hooks
and a typed canvas-distribution API client; reuse existing gallery/dialog,
permission summary, upload, and settings primitives where they fit. Details
and review derive from one shared model across viewports. The preview gallery
also serves native plugin listings with images; their direct install path stays
available. Share is host-owned,
available from the task/workspace canvas and workspace rows. All copy uses the
five real locales and generated Traditional Chinese/pseudo catalogs.

## ASCII UI preview

All views below are proposed. Structure, action order, permissions before
confirmation, explicit image controls, and separate phone composition are
requirements. Names, image proportions, and spacing are illustrative.

### UI-01: Canvas catalog

Entry: Settings > Plugins > Canvases, or Browse shared canvases from a workspace.
Maps to AC-CANVASES-MARKETPLACE-004.1, 005.2, 005.4, 006.1, and 006.2.

```text
Desktop
+-------------------------------------------------------------------+
| Plugins                                                           |
| Installed | Browse | CANVASES                                     |
| Workspace [Project A v]  Search [____________] Sort [Recent v]     |
|                                                [Install canvas]   |
| +----------------------+ +----------------------+                 |
| | [cover screenshot]   | | [cover screenshot]   |                 |
| | Sprint board  1.0.0  | | Release viewer       |                 |
| | by example-author   | | by contributor       |                 |
| | Description...      | | Description...       |                 |
| | [View details]      | | 2 copies [Open v]    |                 |
| +----------------------+ +----------------------+                 |
+-------------------------------------------------------------------+

Phone
+------------------------------+
| Plugins                      |
| Installed | Browse | Canvases |
| Workspace [Project A v]      |
| Search [_____________]       |
| Sort [Recent v]              |
| [Install canvas]             |
| +--------------------------+ |
| | [cover screenshot]       | |
| | Sprint board / author    | |
| | Description...           | |
| | [View details]           | |
| +--------------------------+ |
+------------------------------+
```

Cards open details by pointer, keyboard, or tap. One page scroll owns the list.
Empty: explain no matching canvases, retain search and Install canvas. Failed
source: inline source warning plus Retry; healthy results remain visible.
Loading: skeleton covers with a status announcement. Gate-off: no Canvases tab.

Existing Browse plugin rows may optionally expose the shared gallery:

```text
Plugin with previews: [icon] Plugin name [View details] [Install]
Plugin without:       [icon] Plugin name                [Install]
Details:              [Back] [Cover] [Previous] 1/3 [Next]
                      Description / author / version / [Install]
```

This applies on desktop and phone; phone details use UI-02's focused surface.
It covers AC-PLUGINS-MARKETPLACE-002.2-002.3 without changing native installation.

### UI-02: Canvas details and install review

Use "Preview images" as the gallery label for both canvases and plugins.

Entry: catalog card or inspected upload/link. Maps to
AC-CANVASES-MARKETPLACE-002.3, 004.2-004.6, 005.3, and 006.1-006.3.

```text
Desktop details
+-------------------------------------------------------------------+
| [Back to canvases] Sprint board 1.0.0                 [Close]      |
| +--------------------------------+  by example-author             |
| |                                |  Description                   |
| |       [selected image]         |  License: MIT   [Repository]    |
| |                                |  Requires Kandev ...           |
| +--------------------------------+                                |
| [Previous] 1 / 3 [Next]  [thumb 1] [thumb 2] [thumb 3]              |
| Permissions                                                       |
| Read: tasks, workflows     Write: none                             |
| Events: none  State: none  Network: none                           |
| Workspace [Project A v]                     [Review & install]    |
+-------------------------------------------------------------------+

Phone details / final review
+------------------------------+
| [Back] Sprint board          | fixed
| 1.0.0 / by example-author    |
| [selected image]             |
| [Previous] 1 / 3 [Next]      |
| Description, license, repo   | one scrolling body
| Permissions                  |
| Read: tasks, workflows       |
| Write: none                  |
| Events / State / Network     |
| Workspace [Project A v]      |
| Package verified / version   |
|------------------------------|
| [Install in Project A]       | fixed, safe-area footer
+------------------------------+

Upload/direct-link entry (shared order)
+-------------------------------------+
| Install canvas                      |
| Upload bundle | Direct link         |
| [Choose .tar.gz file]               |
| Workspace [Project A v]             |
| [Cancel]            [Review bundle] |
+-------------------------------------+
```

The desktop review uses the same detail surface with authoritative inspected
metadata. Catalog Review & install stages bytes first; the final Install action
explicitly approves the displayed permissions. No runtime iframe appears here.
Phone inputs use a focused full-height form. No two-column detail remains
mounted on phones. Upload/link reviews omit the gallery; registry reviews label
images as listing previews, separate from verified permissions. Image enlargement replaces the gallery region with Back to
details; it does not nest another dialog. One image hides previous/next.

States: inspecting shows progress; failed inspection retains input; expired or
changed review offers Review again; incompatible package disables confirmation
with the required version; failed image shows a placeholder and Retry; success
shows Installed in Project A and Open canvas. Add another copy always starts a
new review; retrying a successful request returns its original copy.

### UI-03: Share canvas

Entry: host Share action or workspace canvas row. Maps to
AC-CANVASES-MARKETPLACE-001.2-001.3, 002.1, 003.1-003.2, 003.4, and 006.1-006.3.
Screenshots are supplied later in the registry entry, never in this form.

```text
Desktop
+-------------------------------------------------------------------+
| Share canvas                                            [Close]   |
| Active release: 1.0.0                                             |
| Package ID [sprint-board]   Version [1.0.0]                        |
| Name [Sprint board]  Author [________]  License [________]        |
| Description [____________________________________________]       |
| Source: [Static application v]                                    |
| [How to share]                           [Prepare downloads]      |
|-------------------------------------------------------------------|
| Ready: review included files [Expand]                             |
| Check exported files for private content.                        |
| Bundle: ... MiB          Source: ... MiB                           |
| [Download bundle]       [Download source]                         |
+-------------------------------------------------------------------+

Phone
+------------------------------+
| [Back] Share canvas          | fixed
| Active release: 1.0.0        |
| Package ID [___________]     |
| Version [____]               |
| Name / author / license      | one scrolling body
| Description [__________]     |
| Source [Static v]            |
| [How to share]               |
| Ready: included files, sizes |
| Check for private content.   |
| [Download bundle]            |
| [Download source]            |
|------------------------------|
| [Prepare downloads]          | fixed until ready
+------------------------------+
```

Download controls appear only for the current prepared snapshot. After ready,
the footer can show Close while both downloads stay reachable in the body;
neither download dismisses the form. Form edits invalidate prepared output.
Missing source shows Edit and republish guidance. New active release requires
preparation again. Closing discards temporary share inputs, not canvas data.

### UI-04: Sharing instructions

Entry: How to share in UI-03. Maps to AC-CANVASES-MARKETPLACE-003.3-003.4 and 006.1.

```text
Desktop information dialog          Phone inset bottom drawer
+--------------------------------+  +------------------------------+
| How to share             [X]  |  | How to share            [X]  |
| 1. Download source + bundle   |  | Download source + bundle     |
| 2. Create a repository        |  | Create/upload repository     |
| 3. Upload source              |  | Publish bundle; copy link    |
| 4. Publish bundle; copy link  |  | Send link or file            |
| 5. Send the link or file      |  | Add to registry (optional)   |
| Optional: list in Kandev      |  | [Copy registry example]      |
| [Copy registry example]      |  | [Done]                       |
|                        [Done]|  +------------------------------+
+--------------------------------+
```

Keep instruction steps in a bounded scroll body; fixed title/dismiss and bottom
safe-area padding. Closing returns focus to How to share and preserves UI-03.
The copy action copies text only. No remote creation action is offered.
Registry instructions include the required canvas screenshot URLs and alt text,
cover ordering, and the same fields for custom marketplace registries. Explain
that plugin listings can also supply optional screenshots.

## Tests

All test files/methods below are proposed unless explicitly named as existing.
Work-order frontmatter lists each full AC identity, including those abbreviated
in this table. Use nearby `@covers` annotations when method names are insufficient.

| AC suffixes under AC-CANVASES-MARKETPLACE | Planned evidence |
| --- | --- |
| 001.1, 001.2, 001.4 | `webapp/distribution_test.go`: `TestDistributionPackageRoundTrip`, `TestDistributionSourceModes`, `TestDistributionRejectsNonCanvas`; `backendapp/canvas_distribution_source_test.go`: `TestCanvasProjectSourceSurvivesEditAndExecutorCleanup` |
| 001.3, 003.1, 003.2, 003.4 | `canvas/distribution_export_test.go`: `TestCanvasExportSnapshot`, `TestCanvasExportExclusions`, `TestCanvasExportStaleRelease`, `TestCanvasExportNoLifecycleMutation` |
| 002.1, 002.2, 002.4 | `plugin-registry/canvas-index.test.mjs`: registry URL/list shape, cover order, optional plugin previews; export/install tests prove screenshots are not required |
| 002.3, 004.2 | `canvas-marketplace-detail.test.tsx`: gallery ordering, one-image mode, broken image, permission groups; `backendapp/canvas_distribution_routes_test.go`: `TestCanvasInstallReviewUsesPackagePermissions` |
| 004.1, 004.3, 004.4 | `canvas/distribution_install_test.go`: `TestCanvasInstallSources`, `TestCanvasInstallDigestReview`, `TestCanvasInstallAtomicFailure`, `TestCanvasInstallRestart` |
| 004.5, 004.6 | `canvas/distribution_install_test.go`: `TestCanvasInstallConcurrentRetry`, `TestCanvasInstallIndependentCopy`; `backendapp/canvas_distribution_routes_test.go`: `TestCanvasDistributionWorkspaceAuthorization` |
| 005.1, 005.3 | `plugin-registry/canvas-index.test.mjs`: exact release asset, actual manifest, required registry URLs, source, ID/version, checksum, image metadata independent of package, invalid PR rejection |
| 005.2, 005.4 | `marketplace/canvas_catalog_test.go`: `TestCanvasCatalogWorkspaceProjection`, `TestCanvasCatalogDegradedSource`, `TestCanvasCatalogLegacyEntries`; existing marketplace client/hook tests |
| 003.3 | `canvas-share-help.test.tsx`: complete manual instructions and exact registry example; public-doc validators |
| 006.1, 006.2, 006.3 | Desktop/phone E2E below; API/hook stale-response tests; i18n checks and pseudo-locale rendered pass |
| 006.4 | `backendapp/canvas_distribution_routes_test.go`: `TestCanvasDistributionFeatureOff`; frontend tab/hook tests; existing native plugin tests; `TestDistributionLegacyLocalPackage` |

Additional package tests cover encoded source-subtree access, archive traversal,
links, expansion bombs, duplicate normalized paths, binary contributions
disguised by category, and current file budgets. Registry tests cover URL schemes,
credentials/fragments, list bounds, alt text, and custom-source equivalence.
Receipt tests cover lost response, restart, concurrent confirm, quota races,
workspace deletion, fresh schema, replay, and gated PostgreSQL behavior.

## E2E tests

Use the existing `e2e/tests/canvas/canvas-fixture.ts` pattern with disposable
workspaces, a known mock-agent canvas, and a locally served catalog/package.
Enable the existing feature explicitly and restore it in cleanup. The new
tests must fail, rather than skip, if their required fixture is unavailable.

| File and project | UI flows and AC mapping |
| --- | --- |
| `tests/canvas/canvas-marketplace.spec.ts`, chromium | UI-01/UI-02: search, three images, permission review, upload and link, registry install, reload/Open, additional copy, expired review, bad digest, source outage; 002.3, 004.1-004.6, 005.2-005.4 |
| `tests/canvas/mobile-canvas-marketplace.spec.ts`, mobile-chrome | UI-01/UI-02 by tap: browse, gallery controls, workspace selection, upload/link/catalog review, install/Open, one-column layout, viewport containment and focus; 006.1-006.2 plus same install/gallery criteria |
| `tests/canvas/canvas-sharing.spec.ts`, chromium | UI-03/UI-04: no screenshot input, metadata, prepare, both downloads, help with registry URLs, import screenshot-free bundle and open resulting canvas; 001.1-001.3, 002.1, 003.1-003.4 |
| `tests/canvas/mobile-canvas-sharing.spec.ts`, mobile-chrome | UI-03/UI-04: screenshot-free preparation, both downloads, help drawer/back, input retention, pseudo-locale, actual touch targets, keyboard/safe-area containment; 006.1-006.3 plus sharing criteria |

Use real backend inspection/confirmation and local fixture HTTP; do not intercept
the install response into a fake success. Preview details must not issue a
runtime URL request. Capture downloads and inspect archive contents. Use causal
HTTP/WS waits, no arbitrary sleeps, and no display server or real provider account.

Native-plugin preview coverage: add cases to existing `tests/plugins/plugins.spec.ts`
and `tests/plugins/mobile-plugin-settings-row.spec.ts` for optional screenshots,
ordered gallery, failed-image fallback, and unchanged native Install behavior.
These cover AC-PLUGINS-MARKETPLACE-002.1 through 002.4. Registry Node tests and
shared-gallery unit tests cover the same contract; no second canvas schema is used.

## Work orders

- [ ] [Task 01: Define portable canvas packages](task-01-portable-packages.md)
- [ ] [Task 02: Prepare canvas exports](task-02-export-preparation.md)
- [ ] [Task 03: Install reviewed canvas bundles](task-03-reviewed-installation.md)
- [ ] [Task 04: Publish canvas catalog entries](task-04-registry-catalog.md)
- [ ] [Task 05: Build canvas marketplace browsing](task-05-marketplace-ui.md)
- [ ] [Task 06: Add canvas sharing controls](task-06-sharing-ui.md)
- [ ] [Task 07: Document canvas distribution](task-07-distribution-docs.md)

Dependencies: 01 -> 02 -> 03 -> 04 -> 05 -> 06 -> 07. This deliberately uses
sequential delivery because schema, shared UI/API models, and fixtures overlap.

## Verification commands

Run each work order's exact block from the repository root. Work orders use
isolated subshells for working directories. Install workspace dependencies once
before the first pnpm command: `(cd apps && rtk pnpm install --frozen-lockfile)`.
No product commands below have been run during this design turn.

The work orders own Go/package tests, registry Node tests, focused Vitest,
typecheck, affected-file lint, i18n, and managed desktop/phone E2E commands.
Managed E2E builds fresh production artifacts. No all-worker overrides or
overlapping full suites are allowed. Persistence work includes SQL guard and
store-conformance checks; PostgreSQL needs `KANDEV_TEST_POSTGRES_DSN`.

Design-package checks:

```bash
rtk proxy python3 scripts/lint-spec-files.test.py
rtk proxy python3 scripts/lint-spec-files.py --all
rtk git diff --check -- docs/specs docs/plans/canvas-marketplace
rtk git status --short -- docs/specs docs/plans/canvas-marketplace
```

## Verification results

Initial design validation on 2026-09-10 (before the registry-image revision):

- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Requirement/work-order/link audit: six requirements, 26 acceptance criteria,
  seven work orders, and ten new documents checked. Every criterion has a work
  order; all relative document links and system-design paths resolve.
- `git diff --check -- docs/specs docs/plans/canvas-marketplace`: passed.
- New-document whitespace audit: passed, including untracked work orders.
- `git status --short -- docs/specs docs/plans/canvas-marketplace`: inspected;
  the package and index/cross-reference edits are present and uncommitted.

Registry-image revision validation on 2026-09-10:

- Specification linter tests: passed, 36 tests; full specification lint passed.
- Traceability/link audit: seven requirements, 30 criteria, seven work orders;
  all criteria assigned and document/design links resolved.
- Proposed JSON Schema parses and contains the preview definition and
  conditional canvas requirement. Full JSON Schema engine validation was not
  run in this environment; implementation owns semantic schema test cases.
- Tracked diff and new-document whitespace checks passed.

The revision adds the Plugins-owned preview requirement and the proposed schema
artifact; it does not change the production registry schema yet.

Implementation checks were not run; all seven work orders remain pending.
Record actual command results here and in each task during delivery.

## Risks

- The current source allowlist excludes TS/TSX and YAML; widening must be
  limited to an inert subtree that runtime URLs cannot serve.
- Registry image URLs can change or fail independently of package releases;
  gallery failure must not block valid installation or imply package corruption.
- Registry metadata currently uses a shallow parser and nullable hash. Canvas
  entries need production inspection without breaking native plugin entries.
- Receipt insertion, grants, count admission, and artifact references must be
  atomic across retries and supported database dialects.
- Host URL fetch must not gain private-network access through DNS or redirects.
- Shared catalog caching must never retain a user's workspace instance annotations.
- Old clients must fail safely on static canvas bundles; set a real minimum
  compatible release before shipping the format.
- Private data embedded in exported files cannot be reliably removed
  automatically. Show the exact inventory and author review reminder.
- Official registry discovery remains GitHub-only. Other hosts can share public
  direct bundle links or files; the helper must explain that distinction.

## Design handoff

The next phase is explicit implementation of these work orders with TDD.
This package does not authorize commits, pushes, registry submissions, or
external publishing. Public docs are updated with implementation, not presented
as shipped behavior during this planning turn.
