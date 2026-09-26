---
status: draft
system: agents
created: 2026-09-22
owners:
  - kandev
---
# Agent Permission Control Integrity Requirements

## Overview

An agent profile exposes three permission controls: a permission `mode`, an
`auto_approve` toggle, and a list of `cli_flags`. Each control currently reports
success after Kandev has written it somewhere, not after it has reached the
component that enforces it. A user therefore cannot tell a control that took
effect from one that was accepted and discarded.

Three observed consequences:

- An enabled `cli_flag` is appended to the ACP bridge process. The bridge does
  not forward unrecognized arguments to the agent CLI it wraps, so the flag
  never reaches the process whose behavior the user intended to change.
- A profile `mode` is never supplied when the agent session is created. The
  agent process starts in its own default mode and Kandev switches afterwards
  with `session/set_mode`. A mid-session switch is an instruction-level change
  for some providers; the enforcement the process started with does not
  necessarily follow it.
- That switch is then written and logged as applied. Kandev neither reads the
  mode the agent reports back nor records which of the profile mode, a persisted
  session mode, and a workflow-step mode won, so a clamped, overridden, or
  ineffective mode is indistinguishable from an applied one.
- With `auto_approve` enabled, Kandev selects an offered option by position when
  no option declares an allow kind, and returns a cancellation when the provider
  offers no option at all. Both outcomes reach the agent as a denial, produce no
  permission request, and are reported to the user as auto-approval.

The user-visible effect is that an agent session cannot be configured to run
state-changing Git commands unattended, which blocks multi-repository work where
each repository gets its own task and its own pull request.

## Requirements

### REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-001: CLI flags reach a declared destination

**Intent:** A profile CLI flag must reach the process the user configured it
for, or be refused with an actionable message. It must never be delivered to a
different process while the UI reports it as applied.

#### Acceptance criteria

- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-001.1:** Each agent declares, per launch mode, whether its enabled profile CLI flags are delivered to the launched process argv or to the wrapped agent CLI.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-001.2:** For an ACP launch of an agent that declares no forwarding channel, Kandev continues to append the flags to the launched bridge argv and states that destination in the profile editor's command preview.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-001.3:** A flag that the active agent declares as available only in CLI passthrough mode is rejected when saved on a profile that launches over ACP. The rejection names the equivalent ACP control.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-001.4:** The profile editor's command preview shows the exact argv for the launched process, and names the destination process for the flag section.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-001.5:** **GIVEN** a Claude ACP profile, **WHEN** a user enables the `--dangerously-skip-permissions` CLI flag and saves, **THEN** the save is rejected with a message directing the user to the permission mode control, and no launch is changed.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-001.6:** **GIVEN** a Claude CLI-passthrough profile, **WHEN** the same flag is enabled, **THEN** the save succeeds and the launched agent CLI argv contains the flag.

### REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002: Session mode is delivered at start, confirmed, and attributable

**Intent:** Kandev must configure the agent process with the intended permission
mode before its first turn, must record a mode as applied only after the agent
confirms it, and must make the winning source of the effective mode visible.

#### Acceptance criteria

- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.1:** A mode change is recorded as applied only when the agent's reported current mode equals the requested mode after the call returns.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.2:** A mode that the agent clamps to a different value, refuses, or does not offer produces a session-visible warning naming the requested mode and the effective mode. It is not logged as a successful application.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.3:** The session's displayed mode equals the agent's reported current mode, never the requested mode alone.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.4:** The effective session mode records which source won: the agent profile, a persisted session runtime override, or a workflow-step mode action. That attribution is available on the session and in structured logs.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.5:** **GIVEN** an agent that does not offer `bypassPermissions`, **WHEN** a profile requests it, **THEN** the session reports the effective mode with a warning and does not log the requested mode as applied.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.6:** **GIVEN** a session whose persisted runtime mode differs from its profile mode, **WHEN** the session launches, **THEN** the effective mode names the persisted override as the winning source.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.7:** The effective session mode is delivered to the agent through the agent's declared initial-mode channel before the session's first turn, not only by a mode switch issued after the session exists.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.8:** Initial-mode delivery writes only into per-session agent state that Kandev owns. It never modifies the user's shared agent configuration on the host.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.9:** When the runtime refuses a mode for the launched process identity rather than for the session, the session reports the mode as unavailable with the reason. It does not run in a different mode silently.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.10:** **GIVEN** a profile requesting an unattended permission mode, **WHEN** a session starts, **THEN** the launched agent process is configured with that mode from its first turn.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.11:** **GIVEN** an executor whose process identity disables the requested mode in the agent runtime, **WHEN** a session starts, **THEN** Kandev either makes the mode available for that executor or reports it as unavailable with the reason.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.12:** A start mode does not copy host agent settings or credentials into an isolated executor unless the executor profile selected the applicable configuration or authentication bundle. An unselected host permission rule cannot change the launched agent.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.13:** For every executor on which Kandev reports a start mode as delivered, the launched agent can read the generated settings file at its configured path before its first turn. If transfer or preparation fails, Kandev reports the mode as undelivered.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.14:** When an opted-in portable settings bundle and a start mode target the same file, the agent reads the selected bundle's other settings and the requested start mode. A later transfer cannot erase the mode.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.15:** A valid JSON `null` or another non-object settings root cannot crash session preparation. Kandev either creates a valid session-owned object with the requested mode or reports a preparation error.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.16:** A mode report received during `session/set_mode`, including before the call returns, determines the confirmed effective mode for that request. A report from before that request cannot confirm it. Concurrent requests cannot attribute one report to both requests.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.17:** A start-mode path must not silently invalidate the agent's configured authentication or a user-selected agent configuration directory. If Kandev cannot preserve those inputs for an executor and authentication method, it reports start-mode delivery as unavailable with the reason.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.18:** On a phone or coarse pointer, the mode mismatch warning and both mode names are available through a visible touch control. The same mode choices remain available as on desktop.

### REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003: Auto-approve approves or prompts, and never denies

**Intent:** `auto_approve` must only ever select an option the provider marked
as an allow. It must never turn into a denial or a cancellation without the user
seeing a prompt, and it must never remove the interactive path that exists when
the control is disabled.

Measured today: enabling the control produces no prompt and an immediate
refusal, while the same workspace with the control disabled holds the call
pending until a person answers it. A control named for automation currently
removes the only path that works.

#### Acceptance criteria

- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.1:** With `auto_approve` enabled, Kandev selects only an option whose kind is `allow_once` or `allow_always`.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.2:** When no offered option declares an allow kind, Kandev falls back to the interactive permission prompt rather than selecting an option by position.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.3:** When the provider offers no option at all, Kandev records the request and surfaces it to the user rather than answering with a cancellation.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.4:** Every auto-approval records a permission transcript entry carrying the selected option ID, option kind, and the auto-approval source, so an auto-approved call and a human-approved call are both auditable.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.5:** A permission answered by the delivery-timeout safety valve is surfaced to the user as a timed-out request, distinguishable from a user denial.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.6:** **GIVEN** a provider request offering only reject options, **WHEN** `auto_approve` is enabled, **THEN** the user receives a permission prompt and the agent receives no answer until the user responds.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.7:** **GIVEN** a provider request offering an allow option listed after a reject option, **WHEN** `auto_approve` is enabled, **THEN** Kandev selects the allow option.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.8:** Enabling `auto_approve` never leaves a session less able to proceed than leaving it disabled. Any request the control does not approve remains answerable by a person.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.9:** Whether a permission request reached Kandev, which option Kandev selected, and whether Kandev answered with a cancellation are each determinable from recorded evidence, without reproducing the session.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.10:** **GIVEN** `auto_approve` is enabled and a request Kandev cannot approve, **WHEN** the session runs unattended, **THEN** the request is visible as pending rather than resolved, and answering it lets the call proceed.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.11:** After reload or session replay, each automatic approval still identifies its selected option ID, option kind, and automatic source in durable permission history; a dropped notification cannot silently erase this evidence.

### REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-004: The permission contract carries no unread field

**Intent:** Kandev must not transmit a permission setting that no component
reads, because it makes the wire contract misleading during diagnosis.

#### Acceptance criteria

- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-004.1:** The agent-configure request carries no permission field that no code path consumes.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-004.2:** An agentctl build that still receives the removed field ignores it without failing the configure call.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-004.3:** `auto_approve` reaches agentctl through exactly one documented carrier.

### REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-005: Unattended state-changing Git commands are proven in both directions

**Intent:** The fix is only complete when the intended capability and the
intended restriction are both demonstrated, because a mechanism verified only in
its success case says nothing about the case it exists for.

#### Acceptance criteria

- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-005.1:** An end-to-end check proves that a session started with an unattended-permission profile runs a state-changing Git command with no human response and no pending permission request.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-005.2:** The same end-to-end check proves that a session started with the default profile raises a pending permission request for the same command and does not run it until answered.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-005.3:** Both directions run against the same repository, executor, and workspace mode, so the profile is the only difference.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-005.4:** Acceptance is measured against the observed result of a state-changing command. A displayed session mode, a structured log recording a mode as applied, and an agent-authored statement that a mode is active are not acceptance evidence for any criterion in this document.

### REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-006: Workspace-seeded agent configuration reaches the agent

**Intent:** Configuration Kandev seeds into a workspace must land in the
directory the agent actually reads. A seed that lands one level away is worse
than no seed, because the resulting failure looks like a missing permission
rather than a misplaced file.

#### Acceptance criteria

- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-006.1:** Repository-scoped file seeding states which directory it writes into and whether that directory is the agent's working directory for the workspace layout in use.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-006.2:** When a workspace holds more than one repository, a seed configured to reach the agent is placed where the agent reads it, or the configuration surface reports that it cannot reach the agent in that layout.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-006.3:** The mismatch is detectable without running an agent, so it does not present as a permission failure.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-006.4:** **GIVEN** a workspace with two repositories and a repository-scoped seed intended for the agent, **WHEN** a session starts, **THEN** either the agent reads the seeded file or Kandev reports that the seed does not reach the agent in this layout.
- **AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-006.5:** **GIVEN** a workspace with one repository, **WHEN** a session starts, **THEN** the existing seeding behavior is unchanged.

## Out of scope

- Changing which commands the installed agent CLI classifies as requiring
  permission. That classification belongs to the agent, not to Kandev.
- Adding a Kandev-owned command allowlist or denylist.
- Changing the Changes-panel Git implementation or the permission boundary it
  documents in
  [`git-operations-permission-boundary.md`](git-operations-permission-boundary.md).
- Changing external permission resolution, covered by
  [`external-permission-resolution.md`](external-permission-resolution.md).
- Changing `create_task_kandev` profile resolution, covered by
  [`../../tasks/requirements/mcp-create-task-agent-profile-validation.md`](../../tasks/requirements/mcp-create-task-agent-profile-validation.md).
