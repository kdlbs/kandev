---
status: active
system: agents
created: 2026-07-26
updated: 2026-09-04
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
- **AC-AGENTS-RUNTIME-UPDATES-001.5:** Every managed npm runtime has an exact Kandev default version. A successful activation persists an operator-selected exact version for this Kandev install. The effective version is the selected version when present and the Kandev default otherwise.
- **AC-AGENTS-RUNTIME-UPDATES-001.6:** Kandev does not persist the default as a selection. An installation without an operator selection follows default-pin changes delivered by later Kandev releases.
- **AC-AGENTS-RUNTIME-UPDATES-001.7:** Every Kandev-built ACP command for the managed package uses the effective exact version, including probes, utility calls, standalone sessions, containers, and SSH executors. Active sessions continue unchanged.
- **AC-AGENTS-RUNTIME-UPDATES-001.8:** Settings lets the operator clear the selected version and return to the Kandev default after that default passes the normal candidate validation.
- **AC-AGENTS-RUNTIME-UPDATES-001.9:** When the weekly or manually started pin-maintenance run finds a changed stable default, it validates the catalogue and opens or refreshes one grouped review pull request without activating a runtime or merging the pull request. When no default changes, it creates no branch or pull request.
- **AC-AGENTS-RUNTIME-UPDATES-001.10:** The pin-maintenance run operates with the repository's built-in Actions authorization and does not require a separately provisioned GitHub App or personal access token. Because built-in-token branch and pull-request events do not recursively start validation workflows, the run explicitly dispatches the six required validation workflows against the exact updater branch commit after creating or refreshing the pull request. The repository or organization setting **Allow GitHub Actions to create and approve pull requests** must be enabled, and verification checks that setting.

## System design

The migrated technical source is split into [part 1](../system-design/runtime-updates-01.md), [part 2](../system-design/runtime-updates-02.md).
