---
status: active
system: agents
created: 2026-09-15
owners:
  - kandev
---

# Session Concurrency Ceiling Requirements

## Overview

An operator can opt into a shared ceiling for new automatic starts. The ceiling
is disabled by default and preserves explicit user actions and accepted work.
This contract belongs to the agent system because it
controls admission of agent executions. Tasks own their durable deferral data.

The disabled default, Settings opt-in, live application, and queue explanation
are implemented. Delivery and verification are recorded in the
[opt-in plan](../../../plans/session-ceiling-opt-in/plan.md).

## Terminology

- **Session ceiling:** The maximum number of sessions in `STARTING` or `RUNNING`
  state, including launches admitted in the current process.
- **Deferred launch:** A durable task record that contains the complete launch
  kind and payload for an automatic request that the ceiling refused.
- **Manual origin:** A direct user action. It can use a manual override when the
  ceiling is full.

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
- **AC-AGENTS-SESSION-CEILING-001.3:** The ceiling shall count sessions in
  `STARTING` and `RUNNING` state together with in-flight reservations, and shall
  release a reservation only after its launch reaches a counted state or fails.
- **AC-AGENTS-SESSION-CEILING-001.4:** A deferred launch shall retain its first
  payload when a different launch for the same task arrives, and the second
  caller shall receive an explicit conflict so it retains ownership.
- **AC-AGENTS-SESSION-CEILING-001.5:** A retry sweep shall replay deferred
  launches by their stored kind. It shall preserve a record when capacity is
  still full or replay fails for a non-ceiling reason.
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
  limits automatic starts, manual starts can exceed it, and workflow WIP is a
  separate limit. The default maximum offered after enabling shall not activate
  the ceiling before the user saves.

## Out of scope

- Per-workspace or per-user ceilings.
- Changing workflow WIP limits or imposing a hard limit on manual launches.
- A new release toggle, YAML setting, or automatic CPU-based capacity selection.
- Replacing the orchestrator's existing launch seams or task repository.
