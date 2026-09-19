---
status: active
system: plugins
created: 2026-09-18
owners:
  - kandev
---

# Plugin publisher identity requirements

## Overview

Users need to distinguish a publisher identity from an author's claim before installing executable content.
Plugins owns this contract because it owns marketplace identity and package installation.
The same attribution rules apply to native plugins and marketplace canvases.
Canvases retains ownership of workspace installation, consent, and release selection.

The implementation is recorded in the
[publisher identity plan](../../../plans/plugin-publisher-identity/plan.md).
This contract replaces the marketplace's attribution convention and optional
catalog digest enforcement.

## Terms

- **Declared author:** Credit supplied by the package. It proves no identity.
- **Verified publisher:** A repository owner established by the official registry for a particular release package.
- **Official Kandev:** A verified release from an explicitly approved Kandev-owned repository.
- **Source:** The catalog or direct input from which the installation originates.

## Requirements

### REQ-PLUGINS-PUBLISHER-001: Publisher evidence

**Intent:** A plugin cannot grant itself a verified publisher or official badge.

#### Acceptance criteria

- **AC-PLUGINS-PUBLISHER-001.1:** The official registry shall derive publisher identity from repository ownership and bind that identity to the exact release package.
- **AC-PLUGINS-PUBLISHER-001.2:** Only verified releases from explicitly approved repositories owned by `kdlbs` shall receive the Official Kandev designation.
- **AC-PLUGINS-PUBLISHER-001.3:** On every publication, the official registry shall reject unauthorized Kandev author claims, including claims introduced by later releases.
- **AC-PLUGINS-PUBLISHER-001.4:** A custom catalog, source name, URL override, manifest, or browser request shall not grant verified publisher status.

### REQ-PLUGINS-PUBLISHER-002: Installation and update continuity

**Intent:** Attribution must describe the installed bytes, including after updates and restarts.

#### Acceptance criteria

- **AC-PLUGINS-PUBLISHER-002.1:** A catalog installation shall use the selected source, package identity, version, and available digest. A mismatch shall prevent activation.
- **AC-PLUGINS-PUBLISHER-002.2:** A verified installation shall preserve its publisher evidence across restart, independent of catalog availability.
- **AC-PLUGINS-PUBLISHER-002.3:** Updates shall establish evidence for the replacement package. An automatic update shall not change a verified publisher or reduce verified status.
- **AC-PLUGINS-PUBLISHER-002.4:** Uploads, direct URLs, filesystem sideloads, and legacy installations without evidence shall remain unverified and usable under existing permissions.
- **AC-PLUGINS-PUBLISHER-002.5:** Failed verification shall leave the existing installation, attribution, and preferences unchanged. A stale selection shall require a catalog refresh.
- **AC-PLUGINS-PUBLISHER-002.6:** A locally edited or exported canvas shall not inherit publisher verification from its original imported release.

### REQ-PLUGINS-PUBLISHER-003: Visible attribution

**Intent:** Users can judge origin without mistaking metadata or integrity checks for publisher verification.

#### Acceptance criteria

- **AC-PLUGINS-PUBLISHER-003.1:** Catalog, installed-plugin, manifest-detail, and canvas-review surfaces shall distinguish publisher, source, and declared author.
- **AC-PLUGINS-PUBLISHER-003.2:** Without valid evidence, these surfaces shall show Unverified publisher. The declared author shall never replace the publisher label.
- **AC-PLUGINS-PUBLISHER-003.3:** Desktop and phone users shall see the same attribution and errors without hover. Long identities shall remain accessible without horizontal page scrolling.
- **AC-PLUGINS-PUBLISHER-003.4:** Attribution shall not grant permissions, bypass canvas consent, change plugin signature status, or imply that package code is safe.
- **AC-PLUGINS-PUBLISHER-003.5:** Loading and source failures shall not show a verified badge without evidence or overwrite attribution for an installed release.

### REQ-PLUGINS-PUBLISHER-004: Verification of an existing version

**Intent:** An installed native plugin can gain publisher verification without another release.

#### Acceptance criteria

- **AC-PLUGINS-PUBLISHER-004.1:** An administrator shall be able to request publisher verification for an installed native plugin without updating or reinstalling it. If it is running, the host shall briefly stop and restart it during the file comparison.
- **AC-PLUGINS-PUBLISHER-004.2:** Verification shall require trusted evidence for the exact installed version and matching installed package files. Matching names or versions alone shall not qualify.
- **AC-PLUGINS-PUBLISHER-004.3:** Missing evidence, different files, or concurrent replacement shall leave the plugin usable and unverified, with an explanation and retry action.
- **AC-PLUGINS-PUBLISHER-004.4:** Success shall persist verification across restart while preserving version, runtime status, permissions, preferences, runtime data, and original installation source.
- **AC-PLUGINS-PUBLISHER-004.5:** Desktop and phone plugin details shall expose the action beside unverified attribution and show progress and the result without hover.

## Out of scope

- Mandatory package signatures, enterprise trust roots, malware detection, and runtime sandbox changes.
- Automatic verification of direct uploads or URLs against remote registries.
- Retrospective verification of canvas releases and discovery of historical versions absent from the official catalog.
- General name, icon, or Unicode impersonation detection beyond reserved attribution checks.
- Revocation infrastructure and retrospective auditing of published packages.
- Changes to canvas authoring authority or plugin capability approvals.

## Design

See the [publisher identity design](../system-design/publisher-identity.md).
