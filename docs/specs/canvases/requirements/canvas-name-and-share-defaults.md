---
id: canvases-name-and-share-defaults
title: Canvas naming and quick sharing
status: draft
system: canvases
owners:
  - canvases
created: 2026-09-22
last_updated: 2026-09-22
---

# Canvas naming and quick sharing requirements

## Overview

Canvas owners need to change the name shown by Kandev without editing or
republishing the application. Preparing a private bundle and source download
should start with the selected release's known metadata so authors only fill
genuine gaps. This extends the [canvas lifecycle](agent-authored-web-apps.md)
and [sharing](marketplace-sharing.md) contracts. Canvas identity and export
preparation remain host operations; embedded applications gain no new write
capability.

## Terminology

- **Canvas name:** Mutable instance title shown in Kandev navigation and host
  chrome. It is separate from a release's immutable package display name.
- **Share draft:** Temporary distribution metadata for one active release. It
  does not change the live release or canvas name.

## Requirements

### REQ-CANVASES-NAME-SHARE-001: Rename from host toolbar

**Intent:** An authorized user can correct a canvas name where they view it.

#### Acceptance criteria

- **AC-CANVASES-NAME-SHARE-001.1:** The task and workspace canvas host toolbar
  shall offer an accessible Rename action beside its displayed name. Desktop
  keyboard and phone touch users shall reach the same action without entering
  the canvas iframe or an agent editing session.
- **AC-CANVASES-NAME-SHARE-001.2:** Rename shall accept a nonempty, trimmed
  title within the existing canvas title limit. The user can save or cancel;
  invalid input and authorization or network failure shall keep the current
  name and an editable draft with a clear error.
- **AC-CANVASES-NAME-SHARE-001.3:** A successful rename shall update the
  existing canvas instance and its title in the open host, task canvas picker,
  workspace navigation, and subsequent reads after refresh or restart. It
  shall preserve canvas ID, scope, task binding, active/pending releases,
  package identity, grants, instance state, and runtime URL.
- **AC-CANVASES-NAME-SHARE-001.4:** Only a user authorized to manage the
  canvas's workspace may rename it. A stale or removed canvas shall not be
  recreated. The embedded application shall not receive a rename API.

### REQ-CANVASES-NAME-SHARE-002: Release-based share draft

**Intent:** An author can prepare downloads by reviewing a few accurate
fields instead of transcribing release metadata.

#### Acceptance criteria

- **AC-CANVASES-NAME-SHARE-002.1:** Opening Share for a valid active release
  shall load an authorized draft bound to that release ID. Package ID, version,
  package display name, description, author, source mode, and repository URL
  shall come from that release when declared. An absent repository URL shall
  remain optional; the mutable canvas name may fill only a missing package
  display name.
- **AC-CANVASES-NAME-SHARE-002.2:** Kandev shall supply a valid minimum
  compatibility version from the release when declared, otherwise from its
  server-owned first-compatible distribution version. The UI shall not guess
  this from browser code or expose a placeholder as a real version. A higher
  declared minimum shall be preserved.
- **AC-CANVASES-NAME-SHARE-002.3:** Kandev shall never choose a license or
  invent an author or description. Undeclared required values shall be shown
  as short, visible actions with field-specific validation. Source mode shall
  default to static only if the release has no distribution source mode and
  its retained files support a static export.
- **AC-CANVASES-NAME-SHARE-002.4:** The first share view shall show the
  release being exported, the required gaps, and Prepare downloads. Already
  populated fields shall remain reviewable and editable in a secondary
  package-details section. A complete draft shall allow one-step preparation;
  preparation shall still show inventory, sizes, private-content warning,
  and separate bundle/source downloads before a file is downloaded.
- **AC-CANVASES-NAME-SHARE-002.5:** User edits shall survive validation
  failures while Share stays open. Changing the active release or a prepared
  field shall invalidate the review and downloads; a changed release shall
  refresh defaults without silently keeping another release's package
  identity. Server validation, expected-release checks, and download
  authorization shall remain authoritative. Sharing shall not rename or
  publish the live canvas.
- **AC-CANVASES-NAME-SHARE-002.6:** Desktop and phone users shall complete
  the same share flow with localized labels, keyboard focus, announced
  loading/errors, at least 44 CSS pixel phone controls, safe-area clearance,
  one scrolling content area, and no horizontal overflow.

## Out of scope

- Automatically choosing legal terms, publishing to a repository or registry,
  and sharing a live instance URL.
- Changing the package ID or version of an immutable active release.
- Granting canvas applications direct access to host rename or export APIs.
