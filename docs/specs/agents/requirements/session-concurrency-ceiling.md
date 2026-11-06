---
status: active
system: agents
created: 2026-09-15
updated: 2026-11-02
owners:
  - kandev
---

# Session Concurrency Ceiling Requirements

## Overview

An operator can enable a shared ceiling for automatic starts and new message
turns. The ceiling is disabled by default. Explicit execution controls retain
their override, and admitted work keeps its capacity.
This contract belongs to the agent system because it
controls admission of agent executions. Tasks own their durable deferral data.

This contract extends the delivered launch contract with message-turn admission.
The [message-turn package](../../../plans/idle-session-message-capacity/plan.md)
records the compatibility change from
[issue #4998](https://github.com/kdlbs/kandev/issues/4998). Ordinary message
turns now wait at the enabled ceiling while explicit execution controls keep
their override.

The disabled default, Settings opt-in, live application, and queue explanation
are implemented. Delivery and verification are recorded in the
[opt-in plan](../../../plans/session-ceiling-opt-in/plan.md).

## Terminology

- **Session ceiling:** The maximum number of sessions in `STARTING` or `RUNNING`
  state, including launches admitted in the current process.
- **Deferred launch:** A durable task record that contains the complete launch
  kind and payload for an automatic request that the ceiling refused.
- **Manual override:** Capacity admission requested through an explicit Start,
  Resume, Send Now, or legacy Run next control. Ordinary message submission
  follows requirement 003, including messages from a user.
- **Message turn:** New orchestrated work requested through chat, a message API,
  an agent message, or an automatic prompt queue. Warm runtime readiness does
  not establish capacity admission for that new work.

## Requirements

### REQ-AGENTS-SESSION-CEILING-001: Bound instance agent sessions

**Intent:** Keep automatic agent starts within the instance capacity while
keeping accepted work and manual recovery available.

**User story:** As an operator, I want automatic agent starts to respect a
bounded instance capacity, so that one installation does not overload its host.

#### Acceptance criteria

- **AC-AGENTS-SESSION-CEILING-001.1:** When an automatic launch would exceed the
  configured ceiling, the system shall refuse the launch and persist its kind,
  replay payload, origin, reason, and enqueue time on the task.
- **AC-AGENTS-SESSION-CEILING-001.2:** When a manual launch would exceed the
  ceiling, the system shall admit it and record that it used a manual override.
  Ordinary message submission shall follow requirement 003.
- **AC-AGENTS-SESSION-CEILING-001.3:** The ceiling shall count sessions in
  `STARTING` and `RUNNING` state together with in-flight reservations, and shall
  release a reservation only after its launch reaches a counted state or fails.
- **AC-AGENTS-SESSION-CEILING-001.4:** A deferred launch shall retain its first
  payload when a different launch for the same task arrives, and the second
  caller shall receive an explicit conflict so it retains ownership.
- **AC-AGENTS-SESSION-CEILING-001.5:** A retry sweep shall replay deferred
  launches by their stored kind. It shall preserve a record when capacity is
  still full or replay fails for a non-ceiling reason, except for the
  permanent failures in AC-AGENTS-SESSION-CEILING-001.11 and
  AC-AGENTS-SESSION-CEILING-001.12. The log entry for a preserved non-ceiling
  failure shall name its cause.
- **AC-AGENTS-SESSION-CEILING-001.6:** A callback from an older execution shall
  not release or confirm a reservation held by a successor execution for the
  same session.
- **AC-AGENTS-SESSION-CEILING-001.7:** With no explicit configuration, the ceiling
  shall be disabled on fresh and upgraded installations, regardless of CPU count.
  `KANDEV_MAX_CONCURRENT_SESSIONS` shall retain its explicit non-negative integer
  override, with zero meaning disabled. Unset, blank, or invalid values shall
  fall back to the saved setting, then to disabled.
- **AC-AGENTS-SESSION-CEILING-001.8:** When disabled, the ceiling shall not defer
  automatic launches or produce new manual-override notices, including when the
  session population cannot be read. Other launch eligibility checks still apply.
- **AC-AGENTS-SESSION-CEILING-001.9:** Disabling or increasing the ceiling shall
  trigger retry of eligible ceiling-deferred launches without losing their
  payload, entry ownership, or original queue time. Disabling shall not stop the
  retry mechanism or bypass workflow WIP and task eligibility checks.
- **AC-AGENTS-SESSION-CEILING-001.10:** A newly created automation task shall
  have one initial-start owner before feeder publication. Workflow auto-start
  shall yield until definitive admission or termination releases that exact
  owner. Continuations shall preserve the existing ownership identity. Restart,
  lease expiry, unknown acknowledgement and failed Stop shall not release
  unresolved durable retry ownership or authorize generic replay.
  When the ceiling refuses the start of an
  admitted automation run, the system shall keep the run open, holding its
  concurrency slot, and keep its task and deferred start. The replayed start
  shall bind its session and turn to that run, regardless of workflow-step
  auto-start eligibility. A replay that fails for a non-ceiling reason, or a
  dropped start, shall fail the run; a replay whose session launched but could
  not be bound shall also stop that session. A start whose run is no longer
  open shall be dropped without launching. This includes a record that
  AC-AGENTS-SESSION-CEILING-001.5 preserved after its replay failed the run,
  so the next sweep drops it instead of replaying it. If the task is deleted
  while its start waits, the unbound run shall fail and release its concurrency
  slot.
- **AC-AGENTS-SESSION-CEILING-001.11:** When a replayed launch fails validation
  of its built agent or continue command, the sweep shall drop the record with
  a launch-invalid reason instead of preserving it. A later pass would build the
  same command and leave another failed session. Wrapping the command-validation
  error shall not change this disposition. A transient failure whose message
  only mentions validation shall still be preserved.
- **AC-AGENTS-SESSION-CEILING-001.12:** When a record's workflow entry can never
  become valid, the sweep shall drop it as superseded. This covers a legacy
  `start` with no recipient session, a binding that cannot be derived or is
  incomplete, an unrouted entry whose step now routes sessions, and a task with
  no durable entry identity. A failed step, transition, or task read during that
  check shall preserve the record for a later pass.

### REQ-AGENTS-SESSION-CEILING-002: Configure automatic session capacity

**Intent:** Let administrators enable and adjust the instance ceiling in Settings.

#### Acceptance criteria

- **AC-AGENTS-SESSION-CEILING-002.1:** Settings > Task Behavior shall expose
  "Limit automatic sessions", initially off, and a maximum that is editable when
  enabled. The section shall state that this setting affects all workspaces.
  Enabling shall require saving a positive whole-number maximum.
- **AC-AGENTS-SESSION-CEILING-002.2:** A successful save shall persist the enabled
  state and maximum across restarts and apply to subsequent admissions without
  restart. Disabling shall retain the saved maximum for later use. An unsaved
  edit or Reset action shall not change effective behavior.
- **AC-AGENTS-SESSION-CEILING-002.3:** Enabling or lowering the ceiling shall
  preserve running sessions and already admitted launches. Later automatic
  launches shall wait until capacity permits them. Manual starts retain their
  existing override behavior.
- **AC-AGENTS-SESSION-CEILING-002.4:** An explicit valid environment override
  shall take precedence over the saved setting. Settings shall show the effective
  value and its source and prevent changes while this override applies. Removing
  it and restarting shall restore the saved setting, or disabled when none exists.
- **AC-AGENTS-SESSION-CEILING-002.5:** Authenticated members shall have read-only
  access. Administrators, including the existing single-user identity when
  authentication is disabled, shall be able to save. Invalid values, failed
  loads, and failed saves shall show an error without claiming success or
  changing the effective setting.
- **AC-AGENTS-SESSION-CEILING-002.6:** Desktop and phone users shall be able to
  find the section, enable, edit, save, reset, and disable it. Phone controls shall
  have touch targets of at least 44px and no document horizontal overflow. Labels,
  help, validation, and state messages shall be localized and keyboard accessible.
- **AC-AGENTS-SESSION-CEILING-002.7:** The section shall explain that the ceiling
  limits automatic starts and new message turns across all workspaces.
  It shall name the explicit execution controls that can exceed the limit and
  distinguish workflow WIP. The default maximum offered after enabling shall not activate
  the ceiling before the user saves.

### REQ-AGENTS-SESSION-CEILING-003: Bound message turns

**Intent:** Operators can submit messages without implementing a separate
capacity check or queue.

#### Acceptance criteria

- **AC-AGENTS-SESSION-CEILING-003.1:** With the ceiling enabled, a new message
  turn shall require capacity even when its runtime is already ready.
  A full or unreadable population shall leave eligible input pending without
  provider dispatch. The rule shall apply to UI, message API, agent-message,
  and automatic queue delivery paths.
- **AC-AGENTS-SESSION-CEILING-003.2:** Concurrent message turns and launches
  shall share the population in criterion 001.3. A bounded admission shall
  reserve its slot before work starts. Nested startup, readiness, and stale
  callbacks shall not release the slot of a pending or successor turn.
  One session shall count once across these reservations and counted states.
- **AC-AGENTS-SESSION-CEILING-003.3:** Accepted pending input shall retain its
  recipient, order, content, model, mode, attachments, references, attribution,
  and delivery identity. A capacity refusal shall not duplicate transcript
  messages or processed workflow triggers. A failed admission shall not claim
  success or clear an unsaved draft.
- **AC-AGENTS-SESSION-CEILING-003.4:** Capacity release, an increased limit, or
  a disabled limit shall request retry through the existing capacity worker.
  Eligible pending messages shall progress without another submission.
  Auto-run OFF, clarification, WIP, cancellation, recovery, and ownership
  barriers shall retain their authority. Repeated refusal shall not create an
  immediate retry loop.
- **AC-AGENTS-SESSION-CEILING-003.5:** Two pending conversations of one task
  shall remain independent. Work in one conversation shall not erase or
  redirect input accepted for its sibling. Among eligible message waits,
  higher task priority shall run first, then earlier enqueue time.
- **AC-AGENTS-SESSION-CEILING-003.6:** Backend restart shall preserve accepted
  queue contents without starting an agent solely to deliver those messages.
  Later authorized session activity shall restore eligibility under the
  [resume queue contract](../../tasks/requirements/resume-prompt-queue.md).
- **AC-AGENTS-SESSION-CEILING-003.7:** Existing session eligibility shall remain
  in force. An explicitly supported completed-session follow-up shall wait
  without launching at full capacity. Failed and cancelled targets shall retain
  their existing rejection. A stale recipient or failed ownership read shall
  not authorize a replacement session.
- **AC-AGENTS-SESSION-CEILING-003.8:** Desktop and phone chat shall show accepted
  pending input through the existing prompt queue. The agent-message tool shall
  report a queued outcome when delivery waits. A disabled ceiling shall retain
  current message behavior, including an unreadable population. Explicit
  execution overrides and accepted-turn continuations shall retain their
  existing admission and audit behavior.

### REQ-AGENTS-SESSION-CEILING-004: Hold launches under host memory pressure

**Intent:** When enabled, a new agent process is not started while the host is
short of memory, so running sessions are not pushed into swap, allocation
failures, or a host watchdog restart.

#### Acceptance criteria

- **AC-AGENTS-SESSION-CEILING-004.1:** When `features.sessionMemoryGuard` is on
  and host available physical memory is below the configured floor or system
  commit charge is above the configured ceiling, an automatic launch that would
  add an agent process shall be deferred through the existing ceiling queue
  with the `memory_pressure` reason, whether or not the session-count ceiling
  is enabled. Its card note shall describe the host condition rather than a
  session count.
- **AC-AGENTS-SESSION-CEILING-004.2:** A manual launch (a user-initiated start,
  resume, or prompt, including when the admitted population cannot be read), a
  prompt or resume into an already counted session, a dynamic-route hand-off of
  a counted session, running turns, and Office coordinator conversations shall
  not be held by memory pressure, and a manual launch shall not read the host.
- **AC-AGENTS-SESSION-CEILING-004.3:** A deferred launch shall be replayed by the
  existing retry sweep once the host is no longer under pressure.
- **AC-AGENTS-SESSION-CEILING-004.4:** When host memory cannot be read or the
  platform reports an incomplete reading, launches shall proceed and one warning
  per failure streak shall be logged.
- **AC-AGENTS-SESSION-CEILING-004.5:** The flag shall be off in every shipped
  profile. With it off, the host shall not be read and admission shall be
  unchanged. `KANDEV_SESSION_MEMORY_MIN_AVAILABLE_MB` (default 8192) and
  `KANDEV_SESSION_MEMORY_MAX_COMMIT_PERCENT` (default 75) shall set the
  thresholds; an invalid value shall keep its default.

## Out of scope

- Per-workspace or per-user ceilings.
- Changing workflow WIP limits or imposing a hard session-count limit on
  manual launches. The opt-in host-memory hold of REQ-AGENTS-SESSION-CEILING-004
  is a host resource condition, not a count limit, and applies only to
  automatic launches.
- A release toggle or YAML setting for the session-count ceiling, or automatic
  CPU-based capacity selection.
- Replacing the orchestrator's existing launch seams or task repository.
- A separate message-capacity setting or a new release toggle.
- Direct terminal keystrokes and provider input within an already admitted turn.
- A global priority guarantee across launch, automation, and message queues.

## System design

- [Session concurrency ceiling](../system-design/session-concurrency-ceiling.md)
- [Session memory guard](../system-design/session-memory-guard.md)
