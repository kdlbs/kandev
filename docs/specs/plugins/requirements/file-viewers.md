---
status: active
system: plugins
created: 2026-11-12
owners:
  - kandev
---

# File viewer requirements

## Overview

Users open supported files directly in Kandev through installed viewer plugins.
When no viewer exists, users can find compatible plugins by file extension.
The plugin system owns registration, discovery, and plugin lifecycle. Workspace
access remains subject to the host's authorization and filesystem boundaries.

This specification incorporates confirmed user decisions through 2026-11-16: the viewer
contract, Three.js, model units, extension discovery, 64 MiB source limit,
one-million-triangle limit, and the existing trusted-native-plugin model.
External files are supported through a host-owned per-plugin Ask/Allow/Deny
setting. Ask is the default. Implementation was authorized on 2026-11-15. The
[implementation record](../../../reviews/file-viewers-implementation.md) tracks
verification and remaining release gates.

## Terminology

- Viewer: a read-only plugin contribution for one or more file extensions.
- Format tag: a host-generated extension label backed by a format declaration.
- Model unit: a coordinate unit whose physical size is unspecified.
- Preview generation: one viewer instance bound to one selected file revision.

## Requirements

### REQ-PLUGINS-FILE-VIEWERS-001: Viewer selection and lifecycle

- **AC-PLUGINS-FILE-VIEWERS-001.1:** Opening ASCII or binary STL shall select an
  eligible installed STL viewer independently of text encoding.
- **AC-PLUGINS-FILE-VIEWERS-001.2:** Existing built-in previews shall retain their
  default priority. One eligible plugin shall open automatically for a format
  without a built-in preview. Workspace files shall load without a confirmation
  action. Multiple eligible plugins shall require a choice.
- **AC-PLUGINS-FILE-VIEWERS-001.3:** Only enabled, ready, compatible plugins with
  matching declared formats shall be eligible. A removed or failed selection
  shall show a recoverable state without switching to another plugin silently.
- **AC-PLUGINS-FILE-VIEWERS-001.4:** Closing a preview, changing its file identity,
  or disabling its plugin shall cancel its work and prevent stale results from
  changing another preview. Switching viewers shall preserve unsaved text.
- **AC-PLUGINS-FILE-VIEWERS-001.5:** Desktop, mobile, and restored file tabs shall
  apply the same selection rules. Restored choices shall be checked again.

- **AC-PLUGINS-FILE-VIEWERS-001.6:** A registered viewer shall render eligible
  files in rich chat previews with the same selection and external permission
  rules as file panels. Collapse or leaving view shall release preview resources.
  Missing session context shall not read bytes. Plugin disappearance shall not
  switch an active card to the text reader. Built-in preview defaults remain.

### REQ-PLUGINS-FILE-VIEWERS-002: Bounded large-file access

- **AC-PLUGINS-FILE-VIEWERS-002.1:** The preview path shall support files above
  10 MiB within explicit byte and geometry limits. The existing text-editor
  limit shall remain unchanged.
- **AC-PLUGINS-FILE-VIEWERS-002.2:** The initial limits shall be 64 MiB of source
  bytes and 1,000,000 triangles. A separate allocation budget shall reject models
  that cannot be processed within available preview resources.
  Exceeding any limit shall stop the preview with a specific explanation.
- **AC-PLUGINS-FILE-VIEWERS-002.3:** Closing or switching files shall stop loading
  and parsing and release resources. Other navigation shall remain usable.
  The loading and ready states shall not have a manual Cancel control.
- **AC-PLUGINS-FILE-VIEWERS-002.4:** A preview request shall read only the selected
  authorized file in the selected execution environment. External files shall
  require the policy in REQ-PLUGINS-FILE-VIEWERS-006. Cross-user, cross-workspace,
  unauthorized traversal, device, and expired-context requests shall fail without
  file content.
- **AC-PLUGINS-FILE-VIEWERS-002.5:** File changes during loading shall invalidate
  the result. Reload shall create a new preview generation, with no retry loop.

### REQ-PLUGINS-FILE-VIEWERS-003: STL interaction and dimensions

- **AC-PLUGINS-FILE-VIEWERS-003.1:** Valid ASCII and binary STL shall support orbit,
  zoom, pan, and Fit/Reset without changing the source file.
- **AC-PLUGINS-FILE-VIEWERS-003.2:** XYZ extents and labelled grid spacing shall
  use model units. The viewer shall not include a unit selector or unit-assumption
  and nonvisual-equivalence notes.
- **AC-PLUGINS-FILE-VIEWERS-003.3:** Camera rotation shall not change model-space
  dimensions. Invalid, empty, non-finite, or unsupported geometry shall show a
  recoverable error before rendering.
- **AC-PLUGINS-FILE-VIEWERS-003.4:** Phones shall support one-finger orbit, pinch
  zoom, and two-finger pan. Fit and zoom controls shall remain
  accessible with touch targets of at least 44px.
- **AC-PLUGINS-FILE-VIEWERS-003.5:** Controls shall have accessible names, visible
  focus, and keyboard alternatives for orbit, zoom, pan, and reset. Dimensions
  shall be available as text. Reduced-motion settings shall disable inertia and
  animated camera movement. No automatic rotation shall occur.

### REQ-PLUGINS-FILE-VIEWERS-004: Extension discovery and recovery

- **AC-PLUGINS-FILE-VIEWERS-004.1:** An unsupported file shall offer Find viewer
  plugin, opening the marketplace with an exact normalized extension filter.
  Unknown text formats shall remain readable and offer this action from Open with.
- **AC-PLUGINS-FILE-VIEWERS-004.2:** Listings shall expose extension tags derived
  from declared formats. General File viewers and 3D categories shall remain
  available without serving as proof of format support.
- **AC-PLUGINS-FILE-VIEWERS-004.3:** Discovery shall transmit no filename, path,
  content, repository identifier, or file-derived hash to a catalog source.
- **AC-PLUGINS-FILE-VIEWERS-004.4:** Installation shall require the normal explicit
  install action. The user shall be able to return to the original file. A new
  eligible viewer shall become available without a full application reload.
- **AC-PLUGINS-FILE-VIEWERS-004.5:** No match, unavailable catalog, incompatible
  plugin, disabled plugin, parse failure, resource limit, and unavailable GPU
  shall have distinct outcomes. Download and external-open remain available
  where the existing host supports them.

### REQ-PLUGINS-FILE-VIEWERS-005: Trust and data handling

- **AC-PLUGINS-FILE-VIEWERS-005.1:** Opening a file shall never install code,
  execute embedded content, or fetch a resource named by the model.
- **AC-PLUGINS-FILE-VIEWERS-005.2:** The official STL viewer shall process content
  locally without analytics or external model transfers. Its labels and errors
  shall render untrusted strings as text.
- **AC-PLUGINS-FILE-VIEWERS-005.3:** Installation guidance shall state that native
  plugins are trusted code. It shall not describe a file handle, worker, or
  read-only viewer declaration as a sandbox.
- **AC-PLUGINS-FILE-VIEWERS-005.4:** Raw file bytes shall not persist in preview
  preferences, browser storage, logs, or crash reports. Closing a preview shall
  release its references and graphics resources without promising secure erasure.

### REQ-PLUGINS-FILE-VIEWERS-006: External file permission

- **AC-PLUGINS-FILE-VIEWERS-006.1:** Each plugin's host-owned settings shall expose
  External file previews with Ask, Allow, and Deny. Missing settings shall use
  Ask. The setting shall apply to that user's plugin installation in the current
  workspace, across its tasks. A plugin or task agent shall not grant itself access.
- **AC-PLUGINS-FILE-VIEWERS-006.2:** Under Ask, the host shall obtain an explicit
  user approval before sending external file bytes to the plugin, parsing them,
  or rendering them. The prompt shall identify the plugin, resolved file path,
  and execution environment. Dismissal shall deny that attempt without a retry loop.
- **AC-PLUGINS-FILE-VIEWERS-006.3:** Approval shall cover one selected file revision
  and preview generation only. A file replacement, different resolved target,
  execution change, plugin update, or restored tab shall require fresh approval.
  Approval shall not cover a directory or another file.
- **AC-PLUGINS-FILE-VIEWERS-006.4:** Under Allow, authorized external regular files
  shall preview without a prompt. Under Deny, the host shall block preview and
  explain how the user can change the setting. Neither choice shall bypass host
  authorization, OS permissions, resource limits, or protected-file exclusions.
- **AC-PLUGINS-FILE-VIEWERS-006.5:** A repository path resolving outside its root
  shall follow the external policy. Local and remote files shall use the selected
  execution environment; an unavailable remote file shall never fall back to
  a same-named backend-host file.
- **AC-PLUGINS-FILE-VIEWERS-006.6:** Tightening policy shall invalidate pending
  approvals and stop active external previews. Changing plugin installation or
  package digest shall reset external policy to Ask. A policy storage error shall
  fail closed. Policy changes and approvals shall be bound to the acting user.
- **AC-PLUGINS-FILE-VIEWERS-006.7:** Desktop and mobile shall provide equivalent
  approval and settings controls. Authoring documentation shall explain policy,
  cancellation, scope, expiry, and the trusted-plugin limitation.

## Out of scope

Mesh editing, slicing, arbitrary point measurement, compressed model archives,
remote model URLs, Git-revision 3D comparison, and isolation of hostile installed native plugins are excluded.
