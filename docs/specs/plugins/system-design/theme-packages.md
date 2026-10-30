---
status: current
system: plugins
requirements:
  - REQ-PLUGINS-THEMES-001
  - REQ-PLUGINS-THEMES-002
  - REQ-PLUGINS-THEMES-003
---

# Data-only Theme Packages and Imports

## Owner and architecture

Plugins owns the versioned artifact, install inventory and marketplace lifecycle.
UI consumes normalized theme data through its [appearance design](../../ui/system-design/application-themes.md).
There is no imperative registerTheme API, runtime process or new marketplace.
See [the data-only theme decision](../../../decisions/2026-10-22-data-only-theme-packages.md).

The existing `manifest.Manifest`, `pkgtar.Verify`/`Install`, plugin filesystem
`store.Record`, `Service.Install` and lifecycle locks are the package authority.
Today `validatePackageManifest` requires managed binaries and
`validateInstallManifest` resolves a host executable. Extend these with an
explicit theme-only branch; never weaken managed-plugin checks unconditionally.
Canvas `distribution.kind: canvas` remains separate and unchanged.

## Package profile and schema

Extend Manifest with `themes: []ThemeContribution` and distribution kind `theme`.
The profile has `distribution.schema_version: 1`, optional local-install license,
and no canvas `source_mode`. Marketplace releases require explicit license.
Each contribution has `key`, `label`, `base: light|dark`, and `path` pointing to a
canonical JSON definition. Identity uses the existing manifest ID/version grammar;
keys are lowercase alphanumeric/hyphen, max 64 characters, unique within a pack.
Native and imported theme packs cannot combine any runtime, endpoints, UI,
capability, action, webhook, auth, config, executor, repository or agent-tool
contribution. Validate absence explicitly, including future contribution fields.
Clone must deep-copy theme declarations and distribution metadata. A profile
allowlist/completeness test must fail when a new manifest contribution is added
without an explicit theme-profile decision; unknown theme-profile fields are errors.

Example native author input (names are illustrative):

```yaml
id: example-themes
api_version: 2
version: 1.0.0
display_name: Example Themes
author: Example Author
description: Two portable palettes
min_kandev_version: <first-supporting-stable-version>
categories: [theme]
distribution:
  schema_version: 1
  kind: theme
  license: MIT
themes:
  - key: charcoal
    label: Charcoal
    base: dark
    path: themes/charcoal.json
  - key: paper
    label: Paper
    base: light
    path: themes/paper.json
```

The supporting stable version is assigned from the real release at implementation,
not guessed here. Native archives contain manifest.yaml, declared JSON files,
optional plain-text README/LICENSE/NOTICE and checksums.txt (plus the existing
optional signature). Reject every undeclared executable, CSS, HTML, JS, image,
font, nested archive, link, device, absolute/traversal/duplicate path or entry.
Allow at most 16 variants, 128 regular files, 8 MiB compressed and expanded,
256 KiB per definition, and 64 KiB manifest/text files. Check actual streamed
sizes before allocating/extracting; shared archive primitives accept explicit
profile limits rather than mutating global package limits. Reuse checksum
verification and atomic version-directory publication.

Add `internal/plugins/themes/schema/v1.json` and typed validation in
`internal/plugins/themes/`. The canonical strict JSON definition has:

- `schema_version: 1`; base comes from the manifest contribution and may not
  disagree with an optional definition base assertion.
- `ui`: optional Kandev semantic role overrides, fixed keys matching the active
  SPA color token inventory (background/foreground, card/popover, primary,
  secondary, muted, accent, border/input/ring, sidebar, scrollbars, chart 1-5,
  success/warning/info/destructive/running/pending/remote/archived and their
  existing paired foreground roles). No font/radius/size/layout properties.
- `editor`: optional fixed-key colors for background/foreground, cursor,
  active/inactive selection, line highlight, gutter/line numbers, widgets,
  diagnostics, diff added/removed line/text overlays. These are semantic keys,
  not arbitrary CSS or Monaco property names.
- `syntax`: optional portable role styles for comment, keyword, string, number,
  boolean, type, function, variable, parameter, property, constant, operator,
  punctuation, tag and attribute. Style has optional foreground/background hex
  and font-style flags from italic/bold/underline/strikethrough.
- `terminal`: optional background/foreground/cursor/cursor-accent/selection colors
  and 16 named ANSI colors for theme-following terminals only. Omitted ANSI
  entries retain the existing palette for the declared base.
- `textmate`: optional sanitized tokenColors data (scope string or string array,
  settings limited to foreground/background/fontStyle), max 1024 rules, 4096
  scope strings total, each <=256 characters. No grammar or executable content.
- `semantic`: optional standard token types/modifier selectors with the same
  bounded style shape. Preserve supported rules and report renderer limitations.

Color literals accept #RGB/#RGBA/#RRGGBB/#RRGGBBAA only and normalize to hex;
opaque roles such as main backgrounds require alpha 1. Font styles are parsed
as an enum set, not CSS. Reject unknown native roles, invalid colors, duplicate
object keys, excessive nesting (>16), and dangerous object keys at any depth.
Return field paths and stable error codes. Optional source attribution is inert
text only. Normalize to immutable data plus content SHA-256, never CSS source.
Publish the schema from a read-only host endpoint and the author guide; frontend
runtime validation and Go validation share conformance fixtures. A cross-language
contract test ensures schema/role lists and accepted fixtures cannot drift.

## VS Code conversion and compatibility

Accept a standalone UTF-8 JSON/JSONC document, <=256 KiB. Use a maintained parser
such as `github.com/tailscale/hujson` for comments/trailing commas rather than
regular-expression stripping. Reject duplicate keys after standardization.
Accept supported `name`, `type: light|dark`, `colors`, `tokenColors` array,
`semanticTokenColors` and `semanticHighlighting`. If type is missing, review
requires explicit light/dark input. Reject unsupported high-contrast types with
a clear compatibility error in v1 rather than silently claiming full support.
Ignore inert `$schema` as metadata without fetching it. Reject `include`, file
paths for tokenColors, remote/relative inheritance and executable VSIX input.

Map supported colors deterministically, for example:

| VS Code field | Canonical targets |
| --- | --- |
| editor.background / foreground | editor background/foreground and app background/foreground unless native overrides exist |
| sideBar.* and panel.background | corresponding sidebar and card/panel surfaces |
| input.*, button.*, dropdown.*, focusBorder | compatible input/primary/popover/ring pairs |
| editor.selectionBackground, editorCursor.foreground, editorLineNumber.* | corresponding editor roles |
| diffEditor.inserted*/removed* | diff line/text roles; retain structural change indicators |
| terminal.background/foreground and ANSI colors | theme-following terminal roles; omitted ANSI colors keep existing palettes |

Keep the exact recognized-field map versioned with converter tests. Unknown VS
Code color IDs are reported and ignored, while malformed values for supported
fields fail. Preserve sanitized TextMate rules for Shiki/Pierre. Derive portable
roles using a documented fixed scope-alias map and rule order: later matching
rules win; simple standard scope/prefix forms are mapped, complex selectors stay
in textmate and are reported as approximate for Monaco/CodeMirror. Semantic
rules map supported standard token names; unsupported modifiers/language selectors
are reported. Do not promise exact VS Code rendering across different grammars.
Native authors can set portable roles explicitly for consistent engine coverage.

Precedence is built-in base -> supported VS Code conversion -> explicit native
ui/editor/syntax/semantic roles. Merge fresh from base on every resolution, never
from the previously active theme. The normalized descriptor includes mapped,
approximated, ignored and fallback field counts plus detailed bounded findings.
Contrast findings warn; author colors are not rewritten or rejected solely for
contrast. Invalid CSS/data is rejected, not a warning.

Raw imports become generated theme-only archives and use the same install code.
Derive a local ID `imported-<source-sha256-prefix>`, version 1.0.0, one stable key,
with administrator-reviewable display label/base and optional declared attribution.
Unknown author/license remains explicitly unknown and cannot qualify for official
registry publication. Hash identity covers the exact source and selected base;
identical repeated imports return the existing equivalent record, never overwrite
a different owner/source. Same labels do not collide. Native pack updates use
reviewed package identity/version rather than display names.

## Preparation, install, and read APIs

Register static /themes routes before existing /:id routes under /api/plugins.
Use existing authentication and instance-admin management semantics:

- `POST /themes/prepare`: admin-only multipart native archive or raw file, or
  JSON URL plus expected package id/version/digest for a marketplace install.
  Detect a native package from validated manifest/profile, not file extension.
  Return a review ID, immutable normalized descriptors, report, metadata, and
  current installed predecessor identity/version. No install or selection write.
- `POST /themes/commit`: admin-only review ID; recheck principal, expiry,
  feature admission and predecessor generation, then install exact prepared
  bytes. Changed/stale inputs return 409/410 and require preparation again.
- `GET /themes`: authenticated eligible inventory, monotonic process revision
  plus boot identity, package/variant IDs, base, version, content digest and
  normalized read URL. Disabled/invalid packs are not selectable. Management
  lists still show them with status through existing plugin records.
- `GET /themes/:packageId/:key?digest=...`: authenticated canonical data for an
  active installed pack only; resolve current record and exact digest or 409/404.
- `GET /themes/schema/v1`: read-only schema (no installed/private data).

Prepared artifacts live in bounded host-owned temporary storage (10-minute TTL,
8 outstanding reviews per admin, 64 MiB global; cleanup on expiry/restart).
Reviews bind authenticated subject, package digest, normalized conversion, user
metadata/base, and observed predecessor. Commit is idempotent: retain the
successful result through TTL and return it on retry; serialize duplicate commits.
Authorization is rechecked each time. No source references or capabilities are
inferred from metadata. URL download follows existing explicit admin plugin
install HTTP(S), timeout and redirect policy, with tighter theme byte caps and
redirect URL validation; no theme JSON field triggers any request. Registry
installs require the selected artifact checksum, ID and version to match. Do not
turn a failed download into an existing-package update.

## Lifecycle and inventory projection

Theme Install creates the existing filesystem record but activates by publishing
validated definitions, without runtime.Start, health probes, credentials,
capability approval, route/bundle loading or workspace attachment. Apply the
same explicit no-runtime branch in Enable, Disable, Uninstall, startup/bootScan,
Sync, update/autoupdate and cleanup paths. Generic install routes must detect a
theme profile and use the same strict validation; they cannot route it to runtime.
The new review surface owns human preview, but CLI/admin direct installation
still gets full validation and cannot bypass digest/source checks supplied to it.

Use current lifecycle locks and atomic record/version publication. Validate every
variant and build a complete immutable candidate snapshot before changing the
current record or inventory. If validation/storage/publication fails, retain the
previous usable record and inventory; remove only the uncommitted candidate.
Disable/uninstall revokes eligibility, and upgrade removing a key makes that
variant unavailable. Restart reconstructs snapshots from validated stored files,
never trusting catalog metadata or browser caches as installation authority.

Publish `plugin.themes_changed` only after successful lifecycle publication,
carrying boot identity and inventory revision, not arbitrary theme data. Wire it
through the event bus and WebSocket gateway; clients refetch on event, reconnect,
focus and acknowledged mutations. Older in-flight responses cannot replace
newer revision/digest snapshots. Refresh failure is not an empty inventory.
Use hook/API/store boundaries; no fetch from presentation components. Built-ins
work when Plugins is unavailable/disabled; existing plugin admission gates must
cover every new management/read route and startup projection. No new rollout flag
or public host theme enum is introduced.

## Marketplace and author tooling

Extend the existing registry `kind` with `theme`, category `theme`, marketplace
Query kind, IndexEntry metadata and CatalogResult themes projection. Reuse source
ordering, identity dedupe, search/sort, errors, installed state, preview metadata
and expected artifact digest. No new registry server, account or payment service.
Old hosts already skip unknown kinds with warnings; preserve schema version 1
and existing plugin/canvas entries so additive theme listings do not break them.
A theme listing whose archive declares runtime contributions must fail validation.

Extend `cmd/plugin-pack` and `cmd/plugin-package-verify` to verify the strict theme
profile without requiring a platform binary. Registry build-index/check-releases
invoke the same verifier, compare expected kind/identity/version, require a license
and min-host version, and validate attribution and optional listing previews.
Separate private import eligibility from redistributable release metadata.
`plugin-registry/plugins.yaml` remains the curated repository list. Contributors
create their own source repository/release and submit a normal registry PR;
Kandev only provides instructions and downloadable schema/sample, not publication.

The fixture is test support: two variants with non-brand example colors, plus a
Monokai-style imported JSON fixture if licensing permits. It proves the host is
generic. Do not add production marketplace entries or create external repositories.

## UI and mobile contract

Settings > Plugins gains a Themes destination using existing marketplace list/
detail components; Appearance links to it. All users can browse/select installed
themes; import/install/manage controls require existing instance-admin authority.
Use explicit empty/error/incompatible/disabled states and a read-only explanation
for non-admins. Install is a named immediate command; applying a theme remains
an Appearance draft requiring Save.

Nearest shipped exemplars are `canvas-marketplace.tsx` (focused list-to-detail),
its dedicated review flow, and `plugins-settings.tsx` permission handling. Desktop
uses a bounded review dialog with a scrollable report and fixed Cancel/Install.
Phone uses a full-height review surface because metadata/report/samples are dense,
with a fixed header/back, one internal scroll body and safe-area-clear actions.
List-to-detail remains one focal view; do not stack a compressed marketplace and
detail side by side. Use existing responsive primitives and localized text, 28px
fine-pointer desktop controls, >=44px phone/coarse-pointer targets, keyboard focus
and return, viewport containment, and zero document horizontal overflow.

## Requirement mapping and verification

| Requirement | Sections |
| --- | --- |
| REQ-PLUGINS-THEMES-001 | Package profile and schema; Marketplace and author tooling |
| REQ-PLUGINS-THEMES-002 | VS Code conversion; Preparation, install, and read APIs |
| REQ-PLUGINS-THEMES-003 | Lifecycle; Marketplace; UI and mobile contract |

Tests cover valid/unsafe/mixed packages, limits before extraction, checksum/kind
mismatch, duplicate keys, conversion maps/reports, missing-base review, admin gates,
review exactness/idempotence, retained old releases on failure, every no-runtime
lifecycle path, restart, stale response fencing and marketplace old-host behavior.
Desktop/phone E2E use disposable pack and catalog fixtures, install two variants,
apply one, update/remove/disable it, and recover without losing edits.

## Primary references

- [VS Code theme format](https://code.visualstudio.com/api/extension-guides/color-theme)
  and [UI color IDs](https://code.visualstudio.com/api/references/theme-color).
- [Shiki custom themes](https://shiki.style/guide/load-theme).
- [HuJSON parser](https://github.com/tailscale/hujson).
- [Current package profile](../requirements/plugins.md),
  [marketplace design](marketplace.md), and
  [canvas distribution boundary](../../canvases/system-design/marketplace-sharing.md).

## Implementation plan

- [Extensible theme support](../../../plans/application-themes/plan.md)
