---
status: active
system: agents
created: 2026-07-26
updated: 2026-09-07
owners:
  - Kandev
---
# Managed Agent Runtime Versions and Updates Requirements

## Overview

Operators need newly released agent models without waiting for a Kandev release. They also need a UI recovery path when the newest npm release is partly published, incompatible with ACP, or otherwise cannot start. Rebuilding an npm cache is not sufficient when an unversioned command selects the same broken release again.

## Requirements

### REQ-AGENTS-RUNTIME-UPDATES-001: Managed Agent Runtime Versions and Updates

**Intent:** Operators need newly released agent models without waiting for a Kandev release. They also need a UI recovery path when the newest npm release is partly published, incompatible with ACP, or otherwise cannot start. Rebuilding an npm cache is not sufficient when an unversioned command selects the same broken release again.

#### Acceptance criteria

- **AC-AGENTS-RUNTIME-UPDATES-001.1:** Settings exposes version management for the built-in managed npm runtimes used by Claude, Codex, OpenCode, Copilot, and Gemini.
- **AC-AGENTS-RUNTIME-UPDATES-001.2:** The update dialog lists stable versions published for the trusted package. The list contains the newest 50 stable versions plus the active and last observed versions when either falls outside that window. The upstream `latest` stable version is selected initially.
- **AC-AGENTS-RUNTIME-UPDATES-001.3:** The backend classifies the selected action as `update`, `rollback`, `repair`, or `up_to_date`. The UI uses this structural state for copy and approval; it never compares translated labels or version strings itself.
- **AC-AGENTS-RUNTIME-UPDATES-001.4:** Kandev stages the exact trusted `package@version`, ACP-probes that candidate, and activates it only after a successful probe. Candidate failure preserves the prior active version and capability catalogue.
- **AC-AGENTS-RUNTIME-UPDATES-001.5:** Every managed npm runtime has an exact Kandev default version. A successful activation persists an exact operator selection for the current default generation. The effective version is that selection when present and the Kandev default otherwise.
- **AC-AGENTS-RUNTIME-UPDATES-001.6:** Kandev does not persist the default as an operator selection. A change to the shipped package or default starts a new default generation.
- **AC-AGENTS-RUNTIME-UPDATES-001.7:** Every Kandev-built ACP command for the managed package uses the effective exact version, including probes, utility calls, standalone sessions, containers, and SSH executors. Active sessions continue unchanged.
- **AC-AGENTS-RUNTIME-UPDATES-001.8:** Settings lets the operator clear the selected version and return to the Kandev default after that default passes the normal candidate validation.
- **AC-AGENTS-RUNTIME-UPDATES-001.9:** When the weekly or manually started pin-maintenance run finds a changed stable default, it validates the catalogue and opens or refreshes one grouped review pull request without activating a runtime or merging the pull request. When no default changes, it creates no branch or pull request.
- **AC-AGENTS-RUNTIME-UPDATES-001.10:** The pin-maintenance run operates with the repository's built-in Actions authorization and does not require a separately provisioned GitHub App or personal access token. Because built-in-token branch and pull-request events do not recursively start validation workflows, the run explicitly dispatches the six required validation workflows against the exact updater branch commit after creating or refreshing the pull request. The repository or organization setting **Allow GitHub Actions to create and approve pull requests** must be enabled, and verification checks that setting.

### REQ-AGENTS-RUNTIME-UPDATES-002: Activate Reviewed Defaults After an Upgrade

**Intent:** A Kandev release must activate its reviewed managed-agent versions. An older operator selection must not hide a new shipped default.

**User story:** As an operator, I want a Kandev upgrade to activate its reviewed agent versions, so that the release works with its tested runtimes.

#### Acceptance criteria

- **AC-AGENTS-RUNTIME-UPDATES-002.1:** When startup detects a changed managed package or Kandev default, Kandev shall remove the prior selection for that agent before it becomes ready.
- **AC-AGENTS-RUNTIME-UPDATES-002.2:** When the shipped package and default remain unchanged, Kandev shall preserve a current-generation operator selection across restarts and unrelated Kandev upgrades. An unmarked legacy selection is reset during the first reconciliation.
- **AC-AGENTS-RUNTIME-UPDATES-002.3:** After Kandev activates a new default, Settings shall let the operator select any validated stable version, including an older version.
- **AC-AGENTS-RUNTIME-UPDATES-002.4:** A selection made after default activation shall remain effective until the operator changes it or a later shipped default changes.
- **AC-AGENTS-RUNTIME-UPDATES-002.5:** Default activation shall affect future probes and launches only. Kandev shall not replace an agent process that remains active during backend recovery.
- **AC-AGENTS-RUNTIME-UPDATES-002.6:** If Kandev cannot complete default activation, startup shall stop before readiness and retry the activation during the next start.
- **AC-AGENTS-RUNTIME-UPDATES-002.7:** On the first release with this behavior, Kandev shall treat an unmarked legacy selection as part of an earlier default generation.

## System design

The migrated technical source is split into [part 1](../system-design/runtime-updates-01.md), [part 2](../system-design/runtime-updates-02.md).
Upgrade-time default activation is defined in [runtime default activation](../system-design/runtime-default-activation.md).
