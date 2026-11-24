---
status: current
system: plugins
requirements:
  - REQ-PLUGINS-FILE-VIEWERS-001
  - REQ-PLUGINS-FILE-VIEWERS-002
  - REQ-PLUGINS-FILE-VIEWERS-003
  - REQ-PLUGINS-FILE-VIEWERS-004
  - REQ-PLUGINS-FILE-VIEWERS-005
  - REQ-PLUGINS-FILE-VIEWERS-006
---

# File viewer system design

## Status and ownership

This design defines the implemented extension contract across host, SDK,
marketplace, and the separate STL plugin repository. Implementation was authorized
on 2026-11-15. See the [implementation record](../../../reviews/file-viewers-implementation.md)
for checks and release gates.

The requirements are in [file-viewers.md](../requirements/file-viewers.md).
The [security review](../../../reviews/file-viewers-security.md) records threats,
controls, verification obligations, and unresolved risks. This design preserves
rationale directly. On 2026-11-15 the user accepted source/triangle limits and
native plugin trust, and required external-file support with Ask as the default.
The external policy below supersedes the earlier repository-only proposal.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-PLUGINS-FILE-VIEWERS-001 | Registration and selection; Lifecycle |
| REQ-PLUGINS-FILE-VIEWERS-002 | Byte transport; Resource admission |
| REQ-PLUGINS-FILE-VIEWERS-003 | STL plugin; Desktop and mobile |
| REQ-PLUGINS-FILE-VIEWERS-004 | Marketplace |
| REQ-PLUGINS-FILE-VIEWERS-005 | Security; Lifecycle; Observability |
| REQ-PLUGINS-FILE-VIEWERS-006 | External file policy; Desktop and mobile |

## Existing boundaries

- `apps/packages/plugin-sdk/src/index.ts` owns `PluginRegistry` and `PluginHostApi`.
  `apps/web/lib/plugins/registry.ts` owns registrations and cleanup.
- `FileEditorPanel`, `FileTabContent`, and `MobileFileViewerPanel` use fixed file
  categories. `getFilePreviewKind` supports Markdown and HTML.
- `requestFileContent` uses the workspace WebSocket path. Agentctl's
  `WorkspaceTracker.GetFileContent` returns UTF-8 or base64 with a 10 MiB cap.
  Its external absolute-path fallback is intentional existing behavior.
- `apps/backend/internal/plugins/manifest` validates package metadata.
  `plugin-registry/build-index.mjs` publishes catalog metadata.
  `marketplace.ApplyQuery` and `MarketplaceBrowser` filter catalog entries.

## Registration and selection

Propose manifest `ui.file_viewers[]` entries with stable local `id`, display
name, and `extensions`. Each extension is lowercase ASCII, without a leading
period: `[a-z0-9][a-z0-9_-]{0,31}`. Allow at most 32 extensions per viewer and
16 viewers per plugin. Reject duplicates, wildcards, paths, and malformed values.
Use the last filename suffix in version one. Files without a suffix have no
exact-format query. MIME guesses are not selection or authorization evidence.

Use `registry.registerFileViewer({ id, Component })`. Runtime registration
must match an installed manifest entry. The host derives formats from the
validated manifest, not mutable runtime claims. Components receive display
metadata, read-only bytes through `readBytes(signal)`, and a generation identity.
The API accepts no caller-selected file path or remote URL.

The shared host resolver checks current plugin generation, readiness, declared
formats, and host compatibility. Existing built-in previews keep priority.
For formats with no built-in preview, select one eligible provider automatically.
Start admission when the selected preview becomes visible, including restored
active tabs. Internal files load automatically. External files follow the current
Ask/Allow/Deny policy before reading. Hidden tabs do not start admission.
Multiple providers require an explicit Open with choice, sorted by display name
and stable ID. No plugin-defined priority or wildcard takeover is permitted.
Ordinary source text remains editable. ASCII STL can default to the matching
viewer while Open as text remains available within the existing editor limit.

Store an explicit provider choice only with the existing per-user file-tab
layout, not as a global extension default. Revalidate on restoration. Camera state remains in memory for the open preview. A missing saved provider
produces a recovery action; it does not transfer bytes to another provider.
Do not discard a dirty editor buffer when changing presentation.

## Byte transport

Add a host-owned, authenticated preview-read service for selected files.
Use a POST admission request with session, repository, file selector, viewer ID,
in the body. These are untrusted selectors. Agentctl captures the file revision
during admission and binds it to the one-use selection. Resolve
user, accessible task, workspace, repository root, and live execution on the
server. Confirm plugin readiness and installed format metadata there.

Return `application/octet-stream` bytes over a bounded HTTP response through
the backend and authenticated agentctl transport. Agentctl supplies a success
trailer only after the final file revision check. The browser response appends
one zero byte only after that check succeeds. The host browser reader requires
the exact admitted byte count plus this marker, then removes the marker. This
framing avoids relying on HTTP trailers that browser Fetch does not expose. No base64 or full model bytes
in WebSocket state or Zustand. The host browser wrapper exposes an ArrayBuffer
to the component. Use `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`,
and attachment disposition. Apply normal authentication, Origin/CSRF protections,
and deny cross-origin access. Do not put paths or bearer capabilities in URLs.
No arbitrary URL proxy, redirects, archive expansion, ranges, or resumable reads
in the first version.

Resolve the file in the selected execution environment. A repository-relative
selector uses a rooted open. An explicit absolute selector or a repository symlink
resolving outside that root enters the external-policy path below. Reject implicit
relative traversal, special files, and protected-file exclusions. Do not invoke
the existing permissive fallback without checking policy. Resolve a home selector
only against the selected execution user's home, never the backend service home.

Agentctl must classify containment and bind a canonical target to an opened file
identity without reading content. Use OS-supported race-resistant opens. Keep
approved reads on that exact regular-file handle, or reopen with verified identity
and containment. Never validate one pathname and read a newly resolved target.
Revalidate after the user answers. A changed target requires a fresh prompt.
Close metadata handles on denial, expiry, or cancellation; cap pending handles.
Enforce `limit + 1` while reading, regardless of stat. Verify identity, size, and
modification state before and after reading and reject observed mutation. This
detects ordinary edits, not a hostile writer who can restore metadata. The parser
must safely handle every received byte sequence.

Propagate cancellation from browser to backend to agentctl. Each preview owns
its request. No shared request deduplication in the first version. On execution
replacement, permission loss, session change, or plugin generation change,
revoke admission and abort active transfers. Never retry against a different
repository, execution, or user automatically. Disconnects terminate resources.
For multiple backend replicas, admission must occur at the shared executor
boundary or use shared accounting; process-local counters alone are insufficient.

## External file policy

The setting is host-owned, outside plugin configuration and plugin-writable
storage. Key it by authenticated user, workspace, and opaque installation ID,
with an installation revision digest and monotonic policy revision. The digest
is SHA-256 of the validated manifest and installation timestamp. It is not a
package integrity hash; package verification remains the installer's responsibility. Default missing state to Ask; storage
errors deny admission. Allow is general across that user's tasks in that workspace,
not across other users or workspaces. Workspace access restrictions always win.
A reinstall or changed package digest resets policy to Ask. Retain disabled-plugin
policy only for the same installation/digest; disable always revokes live handles.

The host plugin settings page offers Ask (default), Allow, and Deny. Only the
interactive authenticated user can change their policy. Reuse host permission and
CSRF enforcement; do not expose mutations through plugin Host APIs or agent tools.
Use compare-and-swap revisions and bounded audit events for setting changes.
An approval dialog must never silently change the persistent setting.

Admission returns either an internal-file read, an external read allowed by current
policy, a denial, or `approval_required` with an opaque pending-request ID. Under
Ask, show a host dialog with plugin identity, full resolved path, file size, task,
and execution label. Show the original path too when a symlink changes its target.
Use Preview once and Cancel, with a separate link to the setting. Do not deliver
bytes or mount the plugin component before permission. Only a visible selected
preview can prompt, including after restoration. Dismissal releases its selection
and shows Retry preview without automatically reopening the prompt. Loading and
ready states have no Cancel action. Closing, switching, hiding, or invalidating
the preview still aborts its work and releases resources. Errors require explicit
retry, with no automatic retry loop.

Bind pending approvals to user, workspace, task/session, repository context,
execution epoch, installation/digest, policy revision, canonical target, file
identity/revision, and preview generation. Expire after five minutes. Allow at
most one visible prompt per browser and four pending requests per user at the
server; excess attempts return busy rather than queue. The approval response
consumes the pending request exactly once and admits only the bound read. Duplicate
responses cannot start another transfer. A reconnect must create a fresh request.

Resolve and authorize metadata before prompting, but read no file body. Expired
or changed requests fail closed. Denial or dismissal suppresses further prompts
for that generation until the user explicitly retries. Closing the file, logout,
permission loss, execution replacement, plugin lifecycle changes, or a policy
revision change invalidates approvals and aborts affected work. Tightening policy
also unmounts ready external previews and releases their data. Already copied bytes
cannot be recalled from a malicious trusted plugin.

Native plugins can bypass this facade using ambient authority. Therefore this
setting governs host-mediated preview behavior and informed user consent, not
containment of hostile installed code. This limitation must appear beside the
setting and in author documentation. No approval grants OS permissions or access
to another execution environment.

## Resource admission

Accepted product ceilings: 64 MiB source and 1,000,000 triangles. Engineering
budgets: 256 MiB accounted allocations, 30 seconds transfer deadline, and 10
seconds parser deadline. These are not performance claims. Validate representative
devices and report any inability to meet the accepted scope before release. Do not raise the existing text-editor cap.

Reserve a worst-case buffer budget before reading. Allow one active transfer
per authenticated user and four per backend/executor admission domain. Return
a bounded busy response instead of accumulating an unbounded queue. Browser
admission allows one parsing job and a total 256 MiB accounted preview budget
across all visible viewers in one browser page. Unmount or suspend inactive viewers and release their
geometry before admitting another model if the budget cannot fit both.

Count raw buffers, decoded text, temporary parser arrays, output geometry, and
estimated GPU copies. A byte ceiling alone is insufficient. Stock ASCII parser
allocations must be measured and bounded before use; a fixed-size source can
produce excessive intermediate allocations. If this cannot be proven, use a
bounded incremental ASCII parser that produces Three.js geometry. Do not treat
a worker timeout as a memory bound. The browser and GPU driver can allocate
unaccounted overhead, so device failure remains a residual risk.

## STL plugin

Keep production source in a dedicated plugin repository. Use the current native
UI plugin packaging contract, including its managed executable requirement.
Use host React and local Three.js assets, with no CDN or network model loading.
The executable needs no file-access or secret capabilities for rendering.

Parse off the UI thread in a dedicated worker. Transfer buffers rather than
copying them. Preflight binary length and triangle-count arithmetic before
allocation. A binary header can begin with `solid`; that prefix is not sufficient
to classify ASCII. Validate the binary layout or fully parse ASCII grammar.
Reject truncated records, excessive facets, non-finite coordinates/normals,
empty geometry, and non-finite bounds or derived dimensions. Bound ASCII token
length and facet count before retaining tokens. Validate worker result lengths
again before creating GPU buffers. Treat STL names and headers as inert data.

Use STLLoader when its validated allocation path fits the budget. Use
OrbitControls for camera motion and a bounded pass over original coordinates
for model-space extents.
Fit the camera to geometry without changing dimension semantics. Display a grid
with labelled spacing. Dimensions and grid spacing use model units. Do not
include a unit selector or unit-assumption and nonvisual-equivalence notes.

## Lifecycle

State sequence: selecting, admitting, awaiting approval (external Ask only),
loading, parsing, ready, failed, disposed.
All asynchronous callbacks carry the complete generation identity. Ignore stale
callbacks. Do not attempt parsing after cancellation. An error boundary protects
React rendering, but cannot recover a blocked UI thread or GPU process failure.

On close, file switch, disable, reload, or uninstall: abort transfer, terminate
worker, detach listeners, disconnect resize observers, stop animation frames,
dispose geometry/material/renderer, revoke object URLs, and release reservations.
Updates create a fresh generation. Already delivered bytes cannot be revoked
from malicious native code. No content survives in layout or plugin storage.

## Marketplace

Extend the manifest, registry build schema, index DTO, query API, and UI with
structured viewer extensions. Display format badges such as `.stl` from these
fields. General categories remain free-form marketplace curation labels. Keep
manifest category `tools`; do not add every extension to its closed enum.

The Browse route accepts a normalized `extension` query parameter and displays
a removable exact-match chip. Filter locally in Kandev's catalog service after
normal catalog fetch, so registry sources never receive a user's file query.
Missing format metadata means unknown support, not a match. Search description
text is never proof. Do not fetch packages merely to inspect their formats.

Registry builds derive formats from the released manifest. Install still checks
the actual manifest, digest, compatibility, and normal permission flow. Listing
metadata is discovery evidence only. A mismatch cannot create a runtime provider.
Unsupported previews show Find viewer plugin and existing download/external-open.
FileViewerOptions shares the editor behavior across file tabs, panels, and mobile.
A preview kind of `none` means the file lacks a Markdown or HTML preview. The
text editor can still display it. Keep discovery out of the editor toolbar.
Render no toolbar container when there are no actions. Keep installed viewer
selection available and preserve editor state when switching. For unknown text,
place discovery inside the explicit Open with flow. Return state
retains only host-local file identity. Cancel/back restores the file. After an
explicit install, offer Open with the ready provider and recheck eligibility.
No auto-install, auto-enable, or automatic replacement after failure.

## Desktop and mobile

Reuse desktop file panels and the full-height `MobileFileViewerPanel` entry path.
The model is the primary surface. Put dimensions in a readable text strip and
Fit/zoom controls near the phone's safe-area edge.
The surface owns gestures only inside the model viewport; it must not
trap Back, browser zoom outside the viewport, or keyboard focus.
External approval uses a desktop dialog and the existing mobile bottom-drawer
pattern. Full paths wrap in a scrollable body, with fixed reachable Preview once
and Cancel actions. Return focus to the file after dismissal. Settings keep all
three policy values visible through a labelled selector on both viewports.
Use native host controls, 44px touch targets, visible focus, localized copy in
all required locales, and no automatic rotation or decorative motion. Add
keyboard camera controls, textual instructions, and readable dimensions.

## Security and observability

Native plugins remain privileged same-origin code and may have a backend process.
The file API limits the supported interface, not all actions of installed code.
Do not advertise isolation. An untrusted-plugin product requires a separate
sandbox architecture. Workers isolate execution scheduling, not network access.
See the security review for the threat matrix and release-blocking evidence.

Record bounded outcome codes for load, parser, limit, cancellation, and GPU
failures. Metrics use no file, repository, user, or plugin IDs as labels. Avoid
raw paths, model headers, bytes, tokens, and unsanitized parser exceptions in
logs or crash reports. Diagnostic IDs can correlate host events without content.

## Compatibility and delivery dependencies

Ship host/SDK registration and bounded transport first, marketplace propagation
second, then the standalone STL plugin. Update the public authoring matrix,
manifest documentation, PLUGIN-API.md, exported SDK types, packaging validation,
and fixture in the same host change. An older host must reject the plugin through
its minimum-version contract. Determine the actual floor from the SDK release
record during release, not an invented version in this draft.

Implementation gates: prove race-resistant file identity on supported executor
OSes, measure ASCII parser allocations, and verify streaming plus cancellation
through local, Docker, SSH, and Kubernetes transport. Also prove that no external
bytes cross into a viewer before the applicable policy or approval admits them.

## Authoring documentation to publish with implementation

Add a File viewer permissions section to `docs/public/plugins-authoring.md` when
the host API ships. Do not publish speculative signatures as available APIs.
The section must communicate this contract:

> Kandev owns file access and the external-file prompt. Use the file handle
> supplied to the viewer. Do not read a selected path through a separate webhook,
> Host request, or backend process to bypass a denied or pending preview.
> External file previews defaults to Ask in the plugin's host settings. Allow
> permits authorized external files for the current user and workspace. Deny
> blocks them. Preview once applies only to one file revision and preview.
> Handle cancellation and permission expiry without retrying automatically.
> Paths refer to the selected local or remote execution environment.
> This policy governs the supported preview flow. Native plugins remain trusted
> code; the setting is not an OS or browser sandbox.

Update the manifest reference and authoring recipe with the registration contract,
byte/triangle limits, approval states, and cleanup example in the same host change.
Link operator settings documentation to this source. Add translated approval and
settings copy through normal host localization; plugins do not own these strings.

## Implementation references

- Manifest and boot contract: `apps/backend/internal/plugins/manifest/file_viewers.go`,
  `apps/backend/internal/webapp/payload.go`, and `apps/packages/plugin-sdk/src/index.ts`.
- Selection and lifecycle: `apps/web/lib/plugins/file-viewer-selection.ts`,
  `apps/web/components/task/plugin-file-preview.tsx`, and
  `apps/web/hooks/use-plugin-file-preview.ts`.
- Host admission and policy: `apps/backend/internal/plugins/file_preview_handlers.go`,
  `file_preview_grants.go`, and `file_preview_policy.go` in the same directory.
- Executor binding: `apps/backend/internal/backendapp/plugin_file_preview.go` and
  `apps/backend/internal/agent/runtime/agentctl/client_file_preview.go`.
- File identity and bounded reads: `apps/backend/internal/agentctl/server/process/file_preview.go`,
  `file_preview_pool.go`, and `manager_file_preview.go` in the same directory.
- Format discovery: `plugin-registry/build-index.mjs` and
  `apps/backend/internal/plugins/marketplace/catalog.go`.

The host reads one admitted file, plus a completion marker, before mounting the
plugin component. The STL worker transfers that buffer and returns bounded vertex
arrays. The parser preflights binary STL before STLLoader and uses a two-pass
bounded tokenizer for ASCII. A conservative calculation reserves source bytes,
five geometry arrays, and 16 MiB for framebuffer attachments. The renderer uses at
most two million pixels without multisampling. Runtime and driver overhead remain
outside this accounting.

Policy changes cancel backend selections immediately. A browser event cancels
previews in the page that changes policy. Other pages check policy every two
seconds; failed checks also cancel. Workspace permission is checked by that route;
session permission and execution identity are checked during admission and transfer.
Data already delivered to trusted code cannot be recalled. A metadata-preserving
hostile filesystem writer is outside revision detection; parsing remains bounded.

## Rich-output file cards

`FilePreview` in `file-preview-block.tsx` uses `shouldOpenFilePreview` and the
reactive plugin registry, then mounts the shared `PluginFilePreview` inside a
bounded responsive container. Session, repository, and path come from the rich
output host context and file reference. Plugin routes never invoke the legacy
text reader automatically, including after unload. Card identity already includes
session, repository, and path. Collapse unmounts the shared preview; its existing
visibility observer releases offscreen selections. One browser-wide preview
reservation remains shared with file panels. The agent catalog does not change.
