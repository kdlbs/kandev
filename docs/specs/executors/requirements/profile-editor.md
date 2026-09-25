---
status: draft
system: executors
created: 2026-09-17
owners:
  - kandev
---

# Executor profile editor requirements

## Overview

Users need the same profile controls regardless of how they reach an executor
profile. The executor system owns this contract because it owns profiles and
their available settings.

## Requirements

### REQ-EXECUTORS-PROFILE-EDITOR-001: Consistent profile editing

**Intent:** Every entry point opens the complete editor for the selected profile.

#### Acceptance criteria

- **AC-EXECUTORS-PROFILE-EDITOR-001.1:** The executor hub, settings tree, executor profile list, and task disclosure shall open the same editor for a selected profile.
- **AC-EXECUTORS-PROFILE-EDITOR-001.2:** Every supported executor type shall expose its applicable profile controls, regardless of entry point. Docker profiles shall expose Dockerfile, image tag, and build controls. SSH profiles shall expose readiness and task-directory reclamation controls. Kubernetes and Sprites profiles shall retain their applicable runtime controls. Applicable credential and policy controls shall remain available.
- **AC-EXECUTORS-PROFILE-EDITOR-001.3:** A valid executor-scoped bookmark shall reach the complete editor for that profile. Navigation shall preserve query parameters and section fragments. Browser Back shall not revisit a redirect page.
- **AC-EXECUTORS-PROFILE-EDITOR-001.4:** A bookmark with a missing executor, missing profile, or mismatched ownership shall show an unavailable-profile state. It shall not open another profile or change stored data.
- **AC-EXECUTORS-PROFILE-EDITOR-001.5:** Profile edits shall retain the shared Save changes, discard, navigation guard, and permission behavior. Saved values shall survive reload. Discard shall restore saved values.
- **AC-EXECUTORS-PROFILE-EDITOR-001.6:** On phones, users shall reach the same controls through direct settings navigation and valid bookmarks. They shall save and reload profile edits without horizontal page overflow.
- **AC-EXECUTORS-PROFILE-EDITOR-001.7:** Settings search, profile creation, and task-creation credential links shall open that same profile editor.

## Related requirements

The [card-spacing requirement](../../ui/requirements/executor-settings-card-spacing.md)
owns the existing form rhythm and scroll behavior. This requirement extends
editor reachability without redesigning those controls.

## Out of scope

- New executor capabilities, backend APIs, or data migrations.
- Changes to permission policy or executor connection ownership.
- Redesign of the settings shell or profile creation forms.

## Implementation plans

- [Unified profile editor](../../../plans/executor-profile-editor-unification/plan.md)
