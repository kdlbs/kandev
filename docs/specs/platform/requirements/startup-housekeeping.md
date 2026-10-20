---
status: active
system: platform
created: 2026-10-19
owners:
  - kandev
---

# Startup housekeeping requirements

## Overview

Kandev must finish required installation-wide housekeeping before it admits
normal activity, explain that work on the startup screen, and avoid repeating
completed historical work. Two previously working tasks must not wait behind
unrelated historical maintenance after the application appears ready.

Platform owns the readiness boundary and shared startup snapshot. This extends
[startup lifecycle](startup-lifecycle.md) and uses the measurements, localization,
and transport rules in [startup progress visibility](startup-progress-visibility.md).
Executor ownership and conversation identity retain their existing owners.

## Terminology

- **Housekeeping:** Required local persisted-state preparation for normal
  operation. It excludes launching an agent and waiting for a remote machine.
- **Restoration:** Reattaching a surviving agent or loading the same saved
  conversation after its previous process is confirmed stopped.
- **Activity producer:** A scheduler, watcher, plugin, or worker that can launch
  an agent or submit work. Constructing a passive dependency is not activation.

## Requirements

### REQ-PLATFORM-STARTUP-HOUSEKEEPING-001: Preparation before activity

**Intent:** Make readiness mean that required global housekeeping has finished.

#### Acceptance criteria

- **AC-PLATFORM-STARTUP-HOUSEKEEPING-001.1:** During required housekeeping, Kandev
  shall keep its existing liveness and startup-status surfaces available while
  ordinary application requests remain unavailable.
- **AC-PLATFORM-STARTUP-HOUSEKEEPING-001.2:** Housekeeping shall finish before
  activity producers activate or session restoration begins. A producer shall
  not begin an immediate first pass during dependency construction.
- **AC-PLATFORM-STARTUP-HOUSEKEEPING-001.3:** Failed required housekeeping shall
  prevent normal activation and readiness, retain its diagnostic context, and
  release resources through the existing startup failure path. Cancellation
  shall prevent admission of subsequent work.
- **AC-PLATFORM-STARTUP-HOUSEKEEPING-001.4:** Remote availability and per-session
  native-conversation restoration shall not be prerequisites for global
  housekeeping completion or application readiness. Bounded local survivor
  ownership discovery may precede readiness to avoid classifying newly launched
  instances as startup orphans. Unresolved remote ownership shall keep that session
  pending without authorizing a replacement or declaring the prior agent dead.
- **AC-PLATFORM-STARTUP-HOUSEKEEPING-001.5:** Startup shall preserve the existing
  backup-before-mutation rule, event-listener-before-attachment rule, complete
  MCP wiring before agent activation, and final required-store health checks.

### REQ-PLATFORM-STARTUP-HOUSEKEEPING-002: Work proportional to outstanding state

**Intent:** A normal restart should not reprocess all historical conversations.

#### Acceptance criteria

- **AC-PLATFORM-STARTUP-HOUSEKEEPING-002.1:** Reopening an already upgraded store
  shall preserve prompt ordinals and sequence counters without aggregating the
  full historical message corpus again. Historical repair shall operate on
  outstanding rows and sessions missing required counters.
- **AC-PLATFORM-STARTUP-HOUSEKEEPING-002.2:** Fresh initialization, legacy upgrade,
  interrupted upgrade, and replay shall produce the same supported prompt
  ordering on SQLite and PostgreSQL. Existing counters shall never decrease,
  including after message deletion.
- **AC-PLATFORM-STARTUP-HOUSEKEEPING-002.3:** Startup runtime housekeeping shall
  distinguish retained conversation handles from runtime records that require
  investigation. A stable stopped record shall not incur an agent or remote
  liveness probe merely because it retains a resume token.
- **AC-PLATFORM-STARTUP-HOUSEKEEPING-002.4:** Filtering shall preserve resumable
  identities and unresolved cleanup ownership. Terminal session state alone
  shall not exclude a record that may still own a process.

### REQ-PLATFORM-STARTUP-HOUSEKEEPING-003: Visible work and timings

**Intent:** Explain the entire blocking startup interval with named work and
measured durations in the existing startup surfaces.

#### Acceptance criteria

- **AC-PLATFORM-STARTUP-HOUSEKEEPING-003.1:** Each required housekeeping operation
  shall appear as a named startup step while it executes, with elapsed time.
  The startup screen shall retain the durations of completed steps in execution
  order for the current startup.
- **AC-PLATFORM-STARTUP-HOUSEKEEPING-003.2:** Timings shall represent actual
  executions, distinguish repeated activations of the same step, and stop
  increasing on completion. The total startup duration shall freeze at readiness.
  No total or percentage shall be invented for unmeasurable work.
- **AC-PLATFORM-STARTUP-HOUSEKEEPING-003.3:** A browser arriving during startup and
  an already open application detecting a restart shall display the same step
  history and current step. Loss of contact shall mark retained status as last
  known; a new startup identity shall replace the prior history.
- **AC-PLATFORM-STARTUP-HOUSEKEEPING-003.4:** Phone users shall see the same labels,
  durations, and failures in a contained single-column surface with one scroll
  owner, safe-area clearance, keyboard access, and no horizontal document overflow.
  New copy shall be localized in every supported backend and web locale.
- **AC-PLATFORM-STARTUP-HOUSEKEEPING-003.5:** Public startup status shall contain
  only bounded protocol identifiers and numerical measurements. Diagnostic logs
  shall identify the responsible step and duration without exposing SQL, paths,
  credentials, or task identities through the unauthenticated status payload.

### REQ-PLATFORM-STARTUP-HOUSEKEEPING-004: Independent promptless restoration

**Intent:** Bring the viewed conversation back without advancing its work or
waiting for an unrelated conversation to restore.

#### Acceptance criteria

- **AC-PLATFORM-STARTUP-HOUSEKEEPING-004.1:** Restoration caused by startup, opening,
  focus, or reconnection shall submit no instruction, create no user turn,
  replay no tool action, and trigger no queued-instruction dispatch.
- **AC-PLATFORM-STARTUP-HOUSEKEEPING-004.2:** Restoration shall preserve the same
  Kandev session and supported native conversation identity. The previous
  instruction may remain uncertain; restoration shall not require proof of its
  completion or manufacture a completed outcome.
- **AC-PLATFORM-STARTUP-HOUSEKEEPING-004.3:** Two independently eligible sessions
  shall be able to restore concurrently within a bounded process-owned limit.
  A pending focused session shall be selected ahead of pending background
  candidates; a focus signal shall join an existing attempt for the same session.
- **AC-PLATFORM-STARTUP-HOUSEKEEPING-004.4:** Focus shall preserve authorization,
  capacity, archive/Stop, executor ownership, and conversation eligibility
  checks. Failed or unsupported restoration shall retain the existing explicit
  recovery outcome without replacing the conversation automatically.

## Out of scope

- Changing agent survival defaults, backup retention, schema-version policy,
  provider support, or ordinary user-invoked queue and scheduler semantics.
- A migration framework, progress database, telemetry service, or new settings.
- Making all remote recovery complete before the application opens.
- Redesigning the native desktop shell's separately tracked startup window.

## System design

See [startup housekeeping](../system-design/startup-housekeeping.md).
