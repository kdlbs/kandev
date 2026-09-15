---
status: draft
system: canvases
created: 2026-09-10
owners:
  - canvases
---

# Canvas marketplace and sharing requirements

## Overview

Users distribute a reusable canvas, discover it in Settings > Plugins, and
install a private instance in their workspace. Canvases owns this vertical
contract because it owns canvas identity, source lineage, scope, and discovery.
Plugins supplies the existing catalog, manifest, validation, and isolated runtime.

This is the first distribution version. Sharing uses downloads and instructions;
Kandev does not create repositories or publish external releases for the user.

## Terminology

- **Bundle:** A versioned, installable archive containing a canvas application,
  editable source and distribution metadata. Registry previews are separate.
- **Source download:** A project archive for inspecting, editing, and putting
  the same application in a repository. It includes build inputs when needed.
- **Instance:** One installed canvas with its own workspace, grants, and state.
- **Cover:** The first screenshot in the author's ordered image list.
- **Registry:** The curated repository list in the Kandev repository that feeds
  the existing marketplace.

## Requirements

### REQ-CANVASES-MARKETPLACE-001: Portable canvas distribution

**Intent:** A recipient can run and edit a shared canvas without its creator's
task, executor, or workspace.

#### Acceptance criteria

- **AC-CANVASES-MARKETPLACE-001.1:** A valid bundle shall contain the application,
  required local assets, editable source, identity, version, author, description,
  license information, and declared permissions. Installation shall require no
  build, package manager, agent, or author-side service.
- **AC-CANVASES-MARKETPLACE-001.2:** Downloaded source shall match the selected
  published release. For a build-based application, it shall include build
  inputs, dependency declarations, lockfiles, and documented build instructions.
  For a no-build application, its HTML, CSS, and JavaScript are its source.
- **AC-CANVASES-MARKETPLACE-001.3:** Exports shall exclude Kandev instance state,
  grants, runtime tokens, workspace bindings, task history, dependency caches,
  and repository history. The author shall see the file inventory and a notice
  to inspect exported files for embedded private content.
- **AC-CANVASES-MARKETPLACE-001.4:** Invalid, oversized, incompatible, incomplete,
  or unsafe packages shall be rejected before execution. Category labels shall
  not allow a managed binary or native frontend contribution to run as a canvas.

### REQ-CANVASES-MARKETPLACE-002: Screenshot presentation

**Intent:** Recipients can inspect the canvas interface before installing it.

#### Acceptance criteria

- **AC-CANVASES-MARKETPLACE-002.1:** Canvas listings in the official registry
  or a custom marketplace registry shall require one to eight preview image
  URLs. Bundle creation, source downloads, upload/link installation, private
  canvases, and agent drafts shall not require screenshots.
- **AC-CANVASES-MARKETPLACE-002.2:** Registry maintainers shall be able to supply,
  reorder, remove, and describe preview URLs in the listing. The first image
  shall be its cover. Changes shall not require rebuilding the canvas bundle.
- **AC-CANVASES-MARKETPLACE-002.3:** Canvas registry cards shall show the cover;
  details shall show all listed images in order with descriptions, position,
  and explicit previous/next controls. A single image shall hide unnecessary
  navigation. Failed images shall not hide permissions or block installation.
- **AC-CANVASES-MARKETPLACE-002.4:** Preview URLs shall use absolute HTTPS
  addresses without credentials or fragments, with nonempty alternative text.
  Invalid URL/list metadata shall fail registry validation. Images shall render
  without executing package code. Unlisted bundle reviews shall work without
  any gallery.

### REQ-CANVASES-MARKETPLACE-003: Share downloads and guidance

**Intent:** Authors can share a canvas without connecting a code-host account.

#### Acceptance criteria

- **AC-CANVASES-MARKETPLACE-003.1:** An authorized user shall be able to open
  Share from a task or workspace canvas with an available active release,
  review its metadata and files, and download both its bundle and source.
  Both downloads shall describe the same prepared version.
- **AC-CANVASES-MARKETPLACE-003.2:** Preparation shall report missing source,
  invalid metadata, or unavailable release files with recovery
  guidance. A new active release or edited share input shall require preparation
  again before download. No export shall silently substitute another release.
- **AC-CANVASES-MARKETPLACE-003.3:** Share shall include an information dialog
  explaining how to create a repository, upload the source, publish the bundle,
  share its direct download URL or file, and request registry inclusion through
  a pull request to Kandev. Instructions shall distinguish publishing a package
  from acceptance into the curated marketplace.
- **AC-CANVASES-MARKETPLACE-003.4:** Downloading or reading instructions shall not
  create a repository, push code, publish a release, submit a pull request, or
  share the author's live canvas instance. Closing or retrying shall preserve
  the canvas and its active release.

### REQ-CANVASES-MARKETPLACE-004: Reviewed workspace installation

**Intent:** Users know what they install and which workspace it can access.

#### Acceptance criteria

- **AC-CANVASES-MARKETPLACE-004.1:** Settings > Plugins shall expose a dedicated
  Canvases section with installation by bundle upload and direct bundle link.
  Registry entries shall enter the same installation review from their details.
- **AC-CANVASES-MARKETPLACE-004.2:** Before confirmation, users shall see package
  identity, author, version, description, license, compatibility,
  destination workspace, and all requested read, write, event, state, and exact
  network-origin permissions. Registry installs shall also show listing previews.
  Empty permission groups shall be stated clearly.
- **AC-CANVASES-MARKETPLACE-004.3:** Confirmation shall install the exact reviewed
  bytes and approved declaration in an authorized workspace. A changed package,
  expired review, revoked access, quota failure, or invalid workspace shall
  prevent installation and leave existing canvases and grants unchanged.
- **AC-CANVASES-MARKETPLACE-004.4:** Successful installation shall create a new
  workspace canvas with empty instance state and new grants, show Open canvas,
  and appear in workspace canvas navigation after reload. It shall preserve
  package version and provenance across backend restart.
- **AC-CANVASES-MARKETPLACE-004.5:** Installing the same package intentionally
  again shall create an independent copy. Retrying the same confirmation shall
  not create duplicates. Existing copies shall never be overwritten or updated
  automatically, including after local edits.
- **AC-CANVASES-MARKETPLACE-004.6:** Workspace owners shall be able to install
  static canvases in their own workspaces without native-plugin administrator
  authority. Native plugin installation and registry-source administration shall
  retain their current authorization requirements.

### REQ-CANVASES-MARKETPLACE-005: Curated marketplace discovery

**Intent:** Authors list their repositories through the existing Kandev registry.

#### Acceptance criteria

- **AC-CANVASES-MARKETPLACE-005.1:** An author shall be able to submit a canvas
  repository to the existing registry list in Kandev. Admission shall require
  a valid versioned bundle, matching repository/package identity, required
  registry preview URLs, source, and permission metadata. Listing shall require maintainer
  acceptance and successful catalog publication.
- **AC-CANVASES-MARKETPLACE-005.2:** Users shall find listed canvases through
  search and existing catalog sorting, open details, inspect images and
  permissions, and install without copying a URL. Native plugin entries shall
  retain their current install behavior.
- **AC-CANVASES-MARKETPLACE-005.3:** Package details shall identify the selected
  version; previews shall come from that registry listing and shall not be used
  as permission or integrity evidence. Installation shall verify the catalog's
  expected package digest. A mismatch shall fail before a canvas is created.
- **AC-CANVASES-MARKETPLACE-005.4:** A failed catalog source shall show a degraded
  state without blocking healthy sources or upload/link installation. Installed
  canvas badges and Open actions shall disclose only the selected authorized
  workspace's instances.

### REQ-CANVASES-MARKETPLACE-006: Accessible delivery and compatibility

**Intent:** Desktop and phone users can complete the same sharing and discovery
flows without weakening existing canvas isolation or feature controls.

#### Acceptance criteria

- **AC-CANVASES-MARKETPLACE-006.1:** On desktop and phones, users shall complete
  upload/link installation, registry discovery, permission review, image
  navigation, both downloads, and repository guidance. Required actions shall
  be visible and accessible without hover, drag-and-drop, or swipe alone.
- **AC-CANVASES-MARKETPLACE-006.2:** Phone details and sharing shall use focused
  full-height surfaces with one content scroll owner, safe-area clearance,
  touch targets of at least 44 CSS pixels, and no document horizontal overflow.
  Keyboard users shall have predictable focus, dismissal, and image navigation.
- **AC-CANVASES-MARKETPLACE-006.3:** Host copy and errors shall use the selected
  locale. Author descriptions, image descriptions, and identifiers shall remain
  author data. Loading, preparation, failure, and success shall be announced.
- **AC-CANVASES-MARKETPLACE-006.4:** With canvases disabled, new canvas routes,
  actions, catalog entries, preparation, downloads, and installation shall be
  unavailable. Ordinary plugin behavior shall remain available according to its
  existing gate. Existing private canvas releases shall need no migration or
  screenshot backfill to run.

## Out of scope

- Repository-URL installation, Git cloning, or install-time builds.
- Automated GitHub, GitLab, or Bitbucket repository/release/PR creation.
- New code-host integrations, private-host authentication, or hosted sharing.
- Public live-instance links, state export, credentials, or data migration.
- Automatic screenshots, image editing, ratings, payments, or usage telemetry.
- Automatic canvas updates, replacement installs, or upstream merge tooling.
- A new plugin runtime, native binary canvases, or a standalone marketplace site.
- Changing existing runtime flag identities or shipped defaults.

## Related records

- [System design](../system-design/marketplace-sharing.md)
- [Implementation plan](../../../plans/canvas-marketplace/plan.md)
- [Existing canvas lifecycle](agent-authored-web-apps.md)
