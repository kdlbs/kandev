---
status: draft
system: agents
created: 2026-11-17
owners:
  - Kandev
---

# Native Agent Update Checks Requirements

## Overview

Some agent runtimes include an update checker that understands their
installation method and configured release channel better than registry
metadata. Operators need Kandev to report these runtimes as current, outdated,
or unknown from the agent's own check, then use the existing notification and
Settings update action. The agents system owns this contract because it owns
runtime identity, update capability, status, and maintenance jobs.

This document extends
[runtime update notifications](runtime-update-notifications.md) and
[harness-owned runtime updates](runtime-updates.md#req-agents-runtime-updates-003-harness-owned-runtime-updates).
For an agent that declares a native check,
AC-AGENTS-RUNTIME-NATIVE-CHECK-002.4 replaces the stable-reference behavior in
AC-AGENTS-RUNTIME-UPDATES-003.2, 003.6, 003.7, and 003.11.

## Terminology

- **Native update check:** A trusted built-in command that reports update
  availability for the agent's own installation without installing or
  changing it. Like any start of the agent, it can initialize the agent's
  local state.
- **Check reason:** A stable code that explains an `unknown` native result,
  such as a failed check, an externally managed installation, or unrecognized
  output.

## Requirements

### REQ-AGENTS-RUNTIME-NATIVE-CHECK-001: Agent-declared native update checks

**Intent:** Report update status from the checker that applies to the same
installation and channel that the agent's updater modifies.

#### Acceptance criteria

- **AC-AGENTS-RUNTIME-NATIVE-CHECK-001.1:** When a built-in agent declares a
  native update check, Kandev shall determine that agent's update status from
  the check and shall not query npm or GitHub release metadata for it, whether
  or not Kandev can install updates for that agent. Agents without the
  declaration shall keep their existing release lookup and status results.
- **AC-AGENTS-RUNTIME-NATIVE-CHECK-001.2:** The check command shall come only
  from built-in agent metadata. It shall run directly without a shell on the
  Kandev host with the same environment and executable resolution as the
  agent's trusted update command, and Kandev shall not add channel-selection,
  force, or installation arguments. Oh My Pi shall declare `omp update --check`.
- **AC-AGENTS-RUNTIME-NATIVE-CHECK-001.3:** When a check exceeds 30 seconds,
  produces more than 64 KiB of output, cannot start, or is cancelled by
  shutdown, Kandev shall stop the check and every process it started without
  waiting for the remaining time limit. It shall report `unknown` with the
  matching check reason to every request waiting for that check.
- **AC-AGENTS-RUNTIME-NATIVE-CHECK-001.4:** Each check shall produce exactly
  one state: `update_available`, `up_to_date`, or `unknown`. It shall report
  the current version, latest version, and channel when the output reports
  them. A zero exit status without a recognized result shall produce
  `unknown`.
- **AC-AGENTS-RUNTIME-NATIVE-CHECK-001.5:** When a recognized result reports an
  available update, Kandev shall report the latest version as printed,
  including a prerelease channel version such as `18.22.0-canary.3`. It shall
  not discard the version because it is not a stable release.
- **AC-AGENTS-RUNTIME-NATIVE-CHECK-001.6:** When a check fails, reports an
  externally managed installation, prints conflicting results, or prints
  unrecognized or malformed output, Kandev shall report `unknown` with a check
  reason and shall never report `up_to_date`.
- **AC-AGENTS-RUNTIME-NATIVE-CHECK-001.7:** When the check reports a current
  version that differs from the version reported by the agent's runtime,
  Kandev shall report `unknown` with a runtime-mismatch reason.
- **AC-AGENTS-RUNTIME-NATIVE-CHECK-001.8:** Declaring a native update check
  shall not enable automatic updates. It shall not change the agent's update
  command, execution command, install script, session recovery, passthrough
  behavior, or container command, and it shall not imply rollback support.

### REQ-AGENTS-RUNTIME-NATIVE-CHECK-002: Shared status, caching, and delivery

**Intent:** Reuse the existing runtime status, notification, and approval
experience so that native checks add no separate update flow.

**User story:** As an operator on a configured release channel, I want Kandev to
tell me when my agent's channel has a newer version, so that I can approve the
existing update from Settings.

#### Acceptance criteria

- **AC-AGENTS-RUNTIME-NATIVE-CHECK-002.1:** Native results shall use the shared
  runtime status cache. Recognized results, including an externally managed
  installation, shall be reused for six hours, and other `unknown` results for
  fifteen minutes. Concurrent status requests and background passes shall
  share one execution, and a background pass shall not run a cached check
  again.
- **AC-AGENTS-RUNTIME-NATIVE-CHECK-002.2:** When the resolved executable, the
  runtime-reported version, or the agent's declared update configuration
  changes, the next status request shall run a new check. Update configuration
  means the files and environment values that the agent declares as selecting
  its update channel, installation ownership, or release source. A declared
  file counts as changed when it is created or removed, or when its size or
  modification time changes. An undeclared input, and an edit to a declared
  file that keeps both its size and its modification time, shall take effect
  no later than when the cached result expires.
  When an update job for the agent ends, successfully or not, Kandev
  shall discard that agent's cached native result. A check that started before
  that point shall not supply the result for later status requests.
- **AC-AGENTS-RUNTIME-NATIVE-CHECK-002.3:** An `update_available` native result
  for an enabled and available agent shall produce the existing runtime update
  notification for that agent and latest version, subject to existing
  preferences and deduplication. `up_to_date` and `unknown` results shall not
  notify. Disabled or unavailable agents shall not run native checks.
- **AC-AGENTS-RUNTIME-NATIVE-CHECK-002.4:** On desktop and phone, Settings and
  the update dialog shall show the native result instead of a stable-release
  reference. They shall show the latest version when an update is available,
  the channel when reported, an up-to-date state, or a localized explanation of
  the check reason. The update dialog shall continue to run the existing
  user-approved update job and probe.

## Out of scope

- Automatic installation, scheduled updates, rollback, or version selection for
  agents with native checks.
- Native checks inside Docker, Sprite, SSH, or Kubernetes executor
  environments. Status describes the Kandev host installation.
- Changing an agent's configured channel or passing `--canary` or `--stable`.
- Native checks for custom agents or arbitrary user-supplied commands.
