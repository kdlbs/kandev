---
status: active
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
- **AC-EXECUTORS-PROFILE-EDITOR-001.8:** An ordinary partial save of a built-in executor profile shall preserve each omitted prepare or cleanup script, including a script saved successfully after the partial save began. Name-only saves and saves of the other script shall not restore an older omitted script.
- **AC-EXECUTORS-PROFILE-EDITOR-001.9:** A supplied prepare or cleanup script shall replace that script, including an empty string that clears it. When ordinary saves explicitly supply the same script, the last committed save shall determine its value. Supplying both scripts shall remain an intentional replacement of both.
- **AC-EXECUTORS-PROFILE-EDITOR-001.10:** A successful ordinary partial save's response and profile-update notification shall carry the prepare script, cleanup script, and update timestamp committed by that save. A later save shall not be substituted into that acknowledgement. Subsequent provisioning shall use the stored scripts according to each runtime's existing script behavior.
- **AC-EXECUTORS-PROFILE-EDITOR-001.11:** A rejected or failed ordinary partial save shall not emit a successful profile-update notification. Validation and authorization rejection, cancellation before commit, a missing profile, and a rolled-back storage failure shall leave stored scripts unchanged.
- **AC-EXECUTORS-PROFILE-EDITOR-001.12:** Script omission preservation shall retain existing explicit version-guarded save behavior, plugin profile restrictions, permission checks, runtime configuration validation, environment-variable handling, and the meaning of other profile fields. Full profile replacement shall retain its existing script replacement behavior.

## Related requirements

The [card-spacing requirement](../../ui/requirements/executor-settings-card-spacing.md)
owns the existing form rhythm and scroll behavior. This requirement extends
editor reachability without redesigning those controls.

## Out of scope

- New executor capabilities, backend APIs, or data migrations.
- Changes to permission policy or executor connection ownership.
- Redesign of the settings shell or profile creation forms.
- Concurrent omission preservation for other profile fields or stale editor drafts that explicitly submit both scripts.
- Changes to script execution timing, running resources, credentials, or runtime cleanup policy.

## Implementation plans

- [Unified profile editor](../../../plans/executor-profile-editor-unification/plan.md)
- [Preserve scripts during partial saves](../../../plans/executor-profile-script-preservation/plan.md)
