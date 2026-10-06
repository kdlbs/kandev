---
status: active
system: agents
created: 2026-09-27
owners:
  - Kandev
---

# OpenCode V2 Adoption Requirements

## Overview

New Kandev installations use OpenCode v2 when Kandev installs its runtime.
Existing v1 users receive an explicit upgrade offer and retain v1 until they accept it.

The agent system owns this contract because it owns runtime selection, installation, and session compatibility.
This capability covers the change between OpenCode distributions, including its Settings action.
Ordinary version updates remain in [runtime updates](runtime-updates.md).

## Terminology

- **Managed runtime:** An OpenCode installation whose package and launch command Kandev selects.
- **Standalone CLI:** An OpenCode executable that the user installs independently of Kandev.
- **Upgrade:** The explicit change from OpenCode v1 to v2 for future Kandev launches.

## Confirmed adoption boundary

The user requested an upgrade offer for existing v1 users and v2 as the default for new Kandev installations.
The upgrade selects a managed v2 runtime for Kandev and preserves the standalone CLI.
The user chooses the upgrade in the existing agent runtime update dialog in Settings.
The choice applies to all OpenCode profiles in that Kandev installation, consistent with the existing runtime update scope.
It is not a separate runtime choice for each profile.

After installation and ACP validation succeed, Kandev stores the selected package and exact version in its database.
Future launches resolve their CLI command from that persisted selection.
An installed standalone v1 executable must not override an accepted managed v2 selection.
The dialog must explain the shared scope before the user starts the upgrade.

## Requirements

### REQ-AGENTS-OPENCODE-V2-001: Default and explicit adoption

**Intent:** New users receive v2 without forcing existing users to change their runtime.

#### Acceptance criteria

- **AC-AGENTS-OPENCODE-V2-001.1:** When Kandev installs OpenCode for a new user without an existing OpenCode runtime, it shall install a reviewed, exact v2 release.
- **AC-AGENTS-OPENCODE-V2-001.2:** When a user already uses v1, a Kandev upgrade shall preserve that runtime choice until the user accepts the v2 upgrade.
- **AC-AGENTS-OPENCODE-V2-001.3:** Settings shall offer existing v1 users an explicit v2 upgrade with the current version, target version, and installation scope.
- **AC-AGENTS-OPENCODE-V2-001.4:** After successful activation, Kandev shall use the v2 package and compatible CLI arguments for installation, updates, probes, and future launches.
- **AC-AGENTS-OPENCODE-V2-001.5:** If candidate installation or ACP validation fails, Kandev shall preserve the previous runtime selection and capability catalogue.
- **AC-AGENTS-OPENCODE-V2-001.6:** An upgrade shall not terminate or replace an active agent process. Settings shall report any condition that prevents activation.
- **AC-AGENTS-OPENCODE-V2-001.7:** Restart and default reconciliation shall preserve the user's decision to remain on v1 until an explicit v2 upgrade.
- **AC-AGENTS-OPENCODE-V2-001.8:** Desktop and phone Settings shall provide the same upgrade, progress, failure, and retry outcomes with accessible controls.
- **AC-AGENTS-OPENCODE-V2-001.9:** The existing agent runtime update dialog shall persist the v2 package and exact version in the database only after installation and ACP validation succeed.
  A restart shall retain this selection, and future Kandev launches shall use it even when standalone v1 remains installed.
- **AC-AGENTS-OPENCODE-V2-001.10:** The dialog shall explain that migration affects future launches for all OpenCode profiles in the installation and leaves the standalone CLI unchanged.
  Dismissal before submission, rejection, or a failed candidate validation shall not record v2 as active.
- **AC-AGENTS-OPENCODE-V2-001.11:** When a user independently replaces a native OpenCode v1 installation with a supported v2 installation, Kandev shall use compatible arguments for the observed version and retain existing session identity.
  An unknown major version shall produce an explicit compatibility error rather than a guessed launch command.

### REQ-AGENTS-OPENCODE-V2-002: Existing conversation continuity

**Intent:** An upgrade preserves the user's conversation and task identity.

#### Acceptance criteria

- **AC-AGENTS-OPENCODE-V2-002.1:** After an upgrade, a compatible saved v1 session shall resume through v2 with its existing native session ID and conversation history.
- **AC-AGENTS-OPENCODE-V2-002.2:** An upgrade shall preserve the Kandev agent, profile, task, and session identities that refer to OpenCode.
- **AC-AGENTS-OPENCODE-V2-002.3:** If v2 cannot resume an existing session, Kandev shall report the failure without silently creating a replacement conversation.
- **AC-AGENTS-OPENCODE-V2-002.4:** Kandev shall distinguish candidate validation failure from failure after v2 accesses existing session data.
  It shall not claim that selecting the old executable reverses an upstream data migration.

## Evidence and limits

Isolated experiments on 2026-09-27 used OpenCode 1.18.5 and 2.0.18:

- The current Kandev argument `--log-level ERROR` failed on v2 before ACP initialization.
- The v2 argument `--log-level error` passed initialization and session creation.
- V2 loaded a v1 session ID from the same isolated home and recalled a codeword from the previous v1 turn.
- A temporary test through Kandev's ACP adapter completed a new v2 prompt with corrected arguments.

These results do not establish concurrent v1/v2 storage safety or downgrade compatibility.

Implementation verification on 2026-09-28 added exact managed v1/v2 ACP runs with an isolated home, workspace, shared OpenCode database, and local provider. The v2 process loaded the v1 native session ID and recovered prior conversation history. A separate v2 test checked model/config resolution and process shutdown. The lifecycle suite verifies that a saved OpenCode session load failure does not create a replacement session, and a SQLite reopen test verifies that the selected family/package/version remains durable.

Deterministic suites cover Kandev MCP transport conversion, model state, permission handling, migration failure boundaries, and runtime selection across command consumers. The real-binary fixture does not independently certify every MCP transport or a user permission round trip. Native command dispatch is covered by version fixtures, but no real native 1.18.5 installation or remote executor was claimed. Concurrent external v1/v2 access and downgrade compatibility remain outside the verified contract.

## Related contracts and delivery

The [system design](../system-design/opencode-v2-adoption.md) defines persistence, startup import, command resolution, and activation.
The [plan package](../../../plans/opencode-v2-adoption/plan.md) defines implementation order and acceptance evidence.
The [adoption decision](../../../decisions/2026-09-27-opencode-runtime-adoption.md) amends default activation for OpenCode only.
Ordinary reviewed updates remain within the adopted family. Use Kandev default does not perform a major migration.
Other agents retain the existing [runtime update contract](runtime-updates.md).

## Out of scope

- Replacing ACP with OpenCode's native HTTP API.
- Automatic v1-to-v2 migration without the user's upgrade action.
- Automatic session replacement or a promise of v2-to-v1 data rollback.
- Changes to other agents' default activation behavior.
