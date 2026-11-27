---
status: draft
system: canvases
created: 2026-09-25
owners:
  - kandev
---

# Canvas Default Availability Requirements

## Overview

Canvases own agent-authored task applications and their promotion to workspace
applications. This requirement graduates the existing canvas capability from an
installation-wide release toggle. The [canvas contract](agent-authored-web-apps.md)
and the Plugins-owned runtime and permission contract remain authoritative.

## Requirements

### REQ-CANVASES-DEFAULT-AVAILABILITY-001: Canvas availability without a release toggle

**Intent:** A user can use canvases on a normal installation without enabling an
experimental runtime flag, while existing permission and runtime boundaries remain
in force.

#### Acceptance criteria

- **AC-CANVASES-DEFAULT-AVAILABILITY-001.1:** In the first default-on stable
  release, task and workspace canvas entry points shall be available in the
  production, development, and E2E profiles. An installation override or
  explicit environment value may still disable the feature during that release.
- **AC-CANVASES-DEFAULT-AVAILABILITY-001.2:** A later stable release shall remove
  the live `features.canvases` toggle. Canvas availability shall no longer depend
  on a profile default, an installation override, or
  `KANDEV_FEATURES_CANVASES`, including a stored or explicit `false` value.
- **AC-CANVASES-DEFAULT-AVAILABILITY-001.3:** In the retirement release, canvas
  creation, publication, viewing, promotion, and management shall remain subject
  to the existing user, task, workspace, grant, package-validation, and isolated
  runtime rules. Making canvases available shall not create or publish a canvas,
  approve a new grant, or execute a canvas package without the existing action.
- **AC-CANVASES-DEFAULT-AVAILABILITY-001.4:** Desktop and phone users shall find
  the existing canvas destinations and complete the existing primary canvas
  workflow when authorized. Loading, empty, permission-denied, and runtime-failed
  outcomes shall remain reachable and understandable at both sizes.
- **AC-CANVASES-DEFAULT-AVAILABILITY-001.5:** The retirement release shall omit
  the canvas key from the Feature Toggles page and `/api/v1/features`. The old
  key and environment variable shall never control another capability.

## Out of scope

- Changing canvas package trust, sandbox policy, grants, or data scope.
- Automatically creating canvases for existing tasks or workspaces.
- Requiring marketplace sharing before the core canvas workflow is available.
