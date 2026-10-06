---
status: active
system: agents
created: 2026-07-26
updated: 2026-10-05
owners:
  - Kandev
---
# Managed Agent Runtime Versions and Updates Requirements

## Overview

Operators need newly released agent models without waiting for a Kandev release. They also need a UI recovery path when the newest npm release is partly published, incompatible with ACP, or otherwise cannot start. Rebuilding an npm cache is not sufficient when an unversioned command selects the same broken release again.

This document owns the runtime-update contract for built-in agents in two
flavors. A **pinned runtime** is staged and activated by Kandev from exact
published versions. A **harness-owned updater** is a built-in agent whose own
CLI installs updates and cannot install an arbitrary version.

## Terminology

- **Pinned runtime:** A built-in agent whose update contract resolves, stages, probes, and activates one exact published version.
- **Harness-owned updater:** A built-in agent whose own CLI command installs the release it selects, detecting how the agent is installed. Kandev runs that command and re-probes the agent; it does not stage or select versions.

## Requirements

### REQ-AGENTS-RUNTIME-UPDATES-001: Managed Agent Runtime Versions and Updates

**Intent:** Operators need newly released agent models without waiting for a Kandev release. They also need a UI recovery path when the newest npm release is partly published, incompatible with ACP, or otherwise cannot start. Rebuilding an npm cache is not sufficient when an unversioned command selects the same broken release again.

#### Acceptance criteria

- **AC-AGENTS-RUNTIME-UPDATES-001.1:** Settings exposes version management for the built-in managed npm runtimes used by Claude, Codex, OpenCode, Copilot, and Gemini when Kandev owns their runtime. External native installations use the ownership and guidance boundary in [runtime update notifications](runtime-update-notifications.md).
- **AC-AGENTS-RUNTIME-UPDATES-001.2:** The update dialog lists stable versions published for the trusted package. The list contains the newest 50 stable versions plus the active and last observed versions when either falls outside that window. The upstream `latest` stable version is selected initially.
- **AC-AGENTS-RUNTIME-UPDATES-001.3:** The backend classifies the selected action as `update`, `rollback`, `repair`, or `up_to_date`. The UI uses this structural state for copy and approval; it never compares translated labels or version strings itself.
- **AC-AGENTS-RUNTIME-UPDATES-001.4:** Kandev stages the exact trusted `package@version`, ACP-probes that candidate, and activates it only after a successful probe. Candidate failure preserves the prior active version and capability catalogue.
- **AC-AGENTS-RUNTIME-UPDATES-001.5:** Every managed npm runtime has an exact Kandev default version. A successful activation persists an exact operator selection for the current default generation. The effective version is that selection when present and the Kandev default otherwise.
- **AC-AGENTS-RUNTIME-UPDATES-001.6:** Kandev does not persist the default as an operator selection. A change to the shipped package or default starts a new default generation. OpenCode adoption follows the family boundary in [OpenCode v2 adoption](opencode-v2-adoption.md); a default change cannot migrate an existing v1 user.
- **AC-AGENTS-RUNTIME-UPDATES-001.7:** Every Kandev-built ACP command for the managed package uses the effective exact version, including probes, utility calls, standalone sessions, containers, and SSH executors. Active sessions continue unchanged.
- **AC-AGENTS-RUNTIME-UPDATES-001.8:** Settings lets the operator clear the selected version and return to the Kandev default after that default passes the normal candidate validation.
- **AC-AGENTS-RUNTIME-UPDATES-001.9:** When the weekly or manually started pin-maintenance run finds a changed stable default, it validates the catalogue and opens or refreshes one grouped review pull request without activating a runtime or merging the pull request. When no default changes, it creates no branch or pull request.
- **AC-AGENTS-RUNTIME-UPDATES-001.10:** The pin-maintenance run operates with the repository's built-in Actions authorization and does not require a separately provisioned GitHub App or personal access token. Because built-in-token branch and pull-request events do not recursively start validation workflows, the run explicitly dispatches the six required validation workflows against the exact updater branch commit after creating or refreshing the pull request. The repository or organization setting **Allow GitHub Actions to create and approve pull requests** must be enabled, and verification checks that setting.

### REQ-AGENTS-RUNTIME-UPDATES-002: Activate Reviewed Defaults After an Upgrade

**Intent:** A Kandev release must activate its reviewed managed-agent versions. An older operator selection must not hide a new shipped default.

**User story:** As an operator, I want a Kandev upgrade to activate its reviewed agent versions, so that the release works with its tested runtimes.

#### Acceptance criteria

- **AC-AGENTS-RUNTIME-UPDATES-002.1:** When startup detects a changed managed package or Kandev default, Kandev shall remove the prior selection for that agent before it becomes ready. For OpenCode, this reset applies only within the adopted family and shall not change its runtime source or perform a v1-to-v2 migration.
- **AC-AGENTS-RUNTIME-UPDATES-002.2:** When the shipped package and default remain unchanged, Kandev shall preserve a current-generation operator selection across restarts and unrelated Kandev upgrades. An unmarked legacy selection is reset during the first reconciliation, except for OpenCode legacy import defined in [OpenCode v2 adoption](opencode-v2-adoption.md).
- **AC-AGENTS-RUNTIME-UPDATES-002.3:** After Kandev activates a new default, Settings shall let the operator select any validated stable version, including an older version.
- **AC-AGENTS-RUNTIME-UPDATES-002.4:** A selection made after default activation shall remain effective until the operator changes it or a later shipped default changes.
- **AC-AGENTS-RUNTIME-UPDATES-002.5:** Default activation shall affect future probes and launches only. Kandev shall not replace an agent process that remains active during backend recovery.
- **AC-AGENTS-RUNTIME-UPDATES-002.6:** If Kandev cannot complete default activation, startup shall stop before readiness and retry the activation during the next start.
- **AC-AGENTS-RUNTIME-UPDATES-002.7:** On the first release with this behavior, Kandev shall treat an unmarked legacy selection as part of an earlier default generation. The later OpenCode adoption migration imports its legacy choice before reconciliation instead of discarding it.

### REQ-AGENTS-RUNTIME-UPDATES-003: Harness-Owned Runtime Updates

**Intent:** Some built-in harnesses ship their own package-manager-independent updater. The operator needs the same Settings update surface for those harnesses without Kandev assuming an installer, staging a candidate, or pinning a version the harness cannot install.

#### Acceptance criteria

- **AC-AGENTS-RUNTIME-UPDATES-003.1:** When a built-in agent declares a trusted self-update command, Settings shall expose the update control, the update-state indicator, and the maintenance job that a pinned runtime exposes.
- **AC-AGENTS-RUNTIME-UPDATES-003.2:** When the operator opens the update dialog for such an agent, the dialog shall show the running version and upstream stable latest as a reference, clearly distinguish that reference from the version the configured updater may install, and shall not offer version selection, a target-version input, or rollback.
- **AC-AGENTS-RUNTIME-UPDATES-003.3:** When approval creates an update job, Kandev shall run the agent's trusted update command, stream its output into the job, and then probe the runtime with the agent's ACP command. If the command exits successfully but the probe reports the same version as before the update, Kandev shall fail the job and retain the command output.
- **AC-AGENTS-RUNTIME-UPDATES-003.4:** When the post-update probe succeeds and reports a changed version, Kandev shall publish the refreshed capability catalogue and record the probe-reported version as the current version. Kandev shall not persist a version selection for that agent.
- **AC-AGENTS-RUNTIME-UPDATES-003.5:** When the post-update probe fails or reports no version change, Kandev shall fail the job, keep the previous capability catalogue, and not report a changed version as active.
- **AC-AGENTS-RUNTIME-UPDATES-003.6:** When the current ACP-reported version is equal to or newer than the upstream stable reference, Settings shall keep the update action available because the configured channel may have a newer release. The backend shall run the trusted updater and let it decide whether an update is available.
- **AC-AGENTS-RUNTIME-UPDATES-003.7:** When Kandev cannot resolve the upstream stable latest version for such an agent, it shall report the status as unknown, show an unknown stable reference in preview, keep the update action usable, and not change another agent's status entry.
- **AC-AGENTS-RUNTIME-UPDATES-003.8:** When the agent's update command fails, Kandev shall fail the job with the command's own output and shall not substitute a package-manager command or modify the installation further.
- **AC-AGENTS-RUNTIME-UPDATES-003.9:** The update command, the package used for version metadata, and the probe command shall come only from built-in agent metadata; request input shall not supply a command, a package, or a registry location.
- **AC-AGENTS-RUNTIME-UPDATES-003.10:** Declaring a self-update command shall not change an agent's execution command, install script, session recovery, passthrough behavior, or container command construction.
- **AC-AGENTS-RUNTIME-UPDATES-003.11:** When resolving stable package metadata for a harness-owned updater, Settings shall query the trusted HTTPS registry directly without requiring the `npm` executable. If resolution fails, status is unknown and preview shall still allow the trusted updater to run with an unknown stable reference.
- **AC-AGENTS-RUNTIME-UPDATES-003.12:** When the operator approves a harness-owned update, Settings shall submit an approval request without a target version or `use_default`.
- **AC-AGENTS-RUNTIME-UPDATES-003.13:** When the backend receives an empty JSON approval body, it shall accept it only for an agent with a built-in self-update capability; for that mode, a non-empty `target_version` or `use_default: true` shall be rejected.
- **AC-AGENTS-RUNTIME-UPDATES-003.14:** When Settings runs a self-update, Kandev shall invoke the trusted command without selecting a version or channel; the harness's existing configured channel determines the installed version, which may differ from the stable latest reference and shall be recorded from the post-update ACP probe.
- **AC-AGENTS-RUNTIME-UPDATES-003.15:** When an update approval returns a terminal `up_to_date` result with an empty `job_id`, Settings shall display that result without registering or polling a maintenance job and shall refresh runtime-update status for each such response.
- **AC-AGENTS-RUNTIME-UPDATES-003.16:** A failed status refresh shall preserve the last successful status, and an older response shall not replace a newer status result.
- **AC-AGENTS-RUNTIME-UPDATES-003.17:** When an update job reaches a terminal state, Settings shall refresh the runtime status so the displayed version reflects the completed operation. Concurrent refresh requests shall not leave the displayed status stale.

### REQ-AGENTS-RUNTIME-UPDATES-004: Runtime evidence and model discovery refresh

**Intent:** Runtime evidence stays tied to the model catalog, and open profiles refresh after runtime activation.
Runtime information and update controls belong in Settings > Agents > Agent runtime updates.
Profile model settings remain focused on model selection and discovery.

#### Acceptance criteria

- **AC-AGENTS-RUNTIME-UPDATES-004.1:** Discovery responses shall distinguish the observed bridge version from the effective managed package version. Missing observations shall remain unknown.
- **AC-AGENTS-RUNTIME-UPDATES-004.2:** Discovery responses shall identify the underlying provider runtime as bundled, external, or unknown. Verified bundled dependency and external executable versions shall remain distinct from bridge versions and separately installed login or passthrough CLIs.
- **AC-AGENTS-RUNTIME-UPDATES-004.3:** Runtime observations shall describe the same host launch context as the displayed model list. Draft edits, refresh, navigation, and runtime activation shall not mix observations from different contexts.
- **AC-AGENTS-RUNTIME-UPDATES-004.4:** Runtime information, release status, and update actions shall remain in the existing Agents runtime settings. Profile model settings shall not show a runtime details panel or runtime recovery actions. A bridge update shall not claim to update an external provider executable.
- **AC-AGENTS-RUNTIME-UPDATES-004.5:** After a successful managed activation, an open affected profile shall refresh discovery with its current complete draft. Failed updates shall preserve the prior model list and runtime observation. Neither outcome shall change saved or draft model selections.
- **AC-AGENTS-RUNTIME-UPDATES-004.6:** Discovery success shall mean that the provider returned a catalog. It shall not imply catalog completeness or account access to a missing model. Failed release checks shall remain unknown.
- **AC-AGENTS-RUNTIME-UPDATES-004.7:** Desktop and phone profile pages shall preserve model selection, discovery status, and refresh controls without runtime details. Runtime management shall retain its existing localized desktop and phone controls.
- **AC-AGENTS-RUNTIME-UPDATES-004.8:** Runtime inspection shall be bounded and read-only. It shall not install packages, activate versions, change profiles, expose raw launch settings, or alter running sessions.

#### Exclusions

Automatic updates, model availability inference, bundled dependency replacement, and remote executor inspection are outside this extension.
Existing automatic update policies retain their separate contract.

## Out of scope

- Selecting an arbitrary published version, rolling back to an older version, or recovering a partly published release for a harness that only installs the release its own updater selects.
- Staging a candidate into a Kandev-managed directory for such a harness. The harness's own installer is the integrity boundary.
- Taking ownership of installations that a harness updater declines to manage, such as an installation owned by a system package manager.
- Container and remote-executor package caches for a harness the Settings action does not prepare.

## Implementation plans

- [Model discovery runtime visibility](../../../plans/model-discovery-runtime-visibility/plan.md)

## System design

The migrated technical source is split into [part 1](../system-design/runtime-updates-01.md), [part 2](../system-design/runtime-updates-02.md).
Upgrade-time default activation is defined in [runtime default activation](../system-design/runtime-default-activation.md).
Harness-owned updaters are defined in [harness self-update](../system-design/harness-self-update.md).
Profile discovery observations and refresh are defined in [runtime model discovery](../system-design/runtime-model-discovery.md).
