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
profile. Creation, editing, and executor policy saves must retain independently
received catalogue choices. The executor system owns this contract because it
owns profiles and their available settings.

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
- **AC-EXECUTORS-PROFILE-EDITOR-001.13:** When an existing built-in profile save succeeds while its owning executor remains available, the accepted profile shall appear in the shared catalogue without undoing any unrelated executor or sibling-profile addition, update, or removal received while the request was pending. Every unrelated current entry and the owning executor's current metadata shall be retained; unrelated entries removed meanwhile shall remain absent. Desktop and phone task creation and subtask choices shall continue to expose the retained eligible profiles with their current names and executor metadata.
- **AC-EXECUTORS-PROFILE-EDITOR-001.14:** A rejected or failed built-in profile save shall leave the current shared catalogue unchanged, retain the unsaved draft and dirty state, and report the existing failure. A successful save shall retain the existing contributor, saved-state, permission, and notification behavior.
- **AC-EXECUTORS-PROFILE-EDITOR-001.15:** When deletion of a built-in profile succeeds, only that profile shall be removed from the current shared catalogue. All unrelated current executors, owning-executor metadata, and sibling profiles shall be retained, and unrelated entries removed during the request shall remain absent. The existing successful delete navigation shall remain available. A failed deletion shall not publish a catalogue change or successful navigation and shall retain the existing failure handling.
- **AC-EXECUTORS-PROFILE-EDITOR-001.16:** When normal Local, Worktree, Docker, or Sprites profile creation succeeds while its owning executor remains available, the accepted new profile shall appear without undoing any unrelated executor or profile addition, update, or removal received during the request. Every unrelated current entry, including sibling profiles and the owning executor's current metadata, shall be retained; unrelated entries removed meanwhile shall remain absent. Desktop and phone task creation and subtask choices shall expose the retained eligible profiles with their current names and executor metadata.
- **AC-EXECUTORS-PROFILE-EDITOR-001.17:** When a live creation notification has already published the accepted new profile before its normal creation response is applied, applying that response shall leave exactly one membership for that profile in its current owning executor and one corresponding eligible task-picker choice. The accepted response shall supply that membership's profile values without altering unrelated entries.
- **AC-EXECUTORS-PROFILE-EDITOR-001.18:** Normal built-in creation shall retain its existing submitted values, validation, permissions, contributor and dirty-state behavior, and successful navigation to the accepted profile's complete editor. A rejected or failed creation shall leave the current catalogue unchanged, retain the unsaved draft and failed contributor state, report the existing failure, and stay on the creation route.
- **AC-EXECUTORS-PROFILE-EDITOR-001.19:** When an executor MCP-policy save succeeds, every unrelated current executor and profile shall retain additions, updates, and removals received while the save was pending. Newly available eligible choices shall remain available in desktop and phone task creation and subtask choices; deleted choices shall remain absent, including when the catalogue contains both retained and removed entries. Applying the policy acknowledgement shall not restore an executor removed during the request.
- **AC-EXECUTORS-PROFILE-EDITOR-001.20:** An executor MCP-policy save shall retain its submitted policy and existing system-executor payload restrictions. Success shall publish the accepted policy and use it as the saved baseline; an unchanged submitted draft matching that baseline shall become clean. A newer draft shall remain intact and dirty when it differs from the accepted baseline. A normalized response shall supply the accepted baseline without replacing the current raw draft. Failure shall preserve the current catalogue, unsaved draft, dirty state, and existing failed-save indication.

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
- Concurrent server arbitration for the edited profile, target deletion versus save ordering, retired-page request ownership, or catalogue-wide revision policies.
- Restoring an executor removed during creation, arbitration against a newer update of the accepted target, repairing previously duplicated catalogue rows, or redesigning live notification writers and separate SSH, Remote Docker, Kubernetes, or plugin creation flows.
- Executor-policy save versus newer same-executor server writes, policy validation redesign, executor deletion and profile-card refresh publication, or request lifetime and navigation redesign.

## Implementation plans

- [Unified profile editor](../../../plans/executor-profile-editor-unification/plan.md)
- [Preserve scripts during partial saves](../../../plans/executor-profile-script-preservation/plan.md)
- [Preserve the current catalogue during profile mutations](../../../plans/executor-profile-catalogue-preservation/plan.md)
- [Preserve choices during built-in profile creation](../../../plans/executor-profile-create-catalogue-preservation/plan.md)
- [Preserve choices during executor policy saves](../../../plans/executor-policy-catalogue-preservation/plan.md)
