---
status: active
system: agents
created: 2026-09-15
owners:
  - kandev
---

# Session Concurrency Ceiling Requirements

## Overview

One Kandev instance can start more agent sessions than its host can serve. A
shared ceiling must limit new automatic starts while preserving explicit user
actions and accepted work. This contract belongs to the agent system because it
controls admission of agent executions. Tasks own their durable deferral data.

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
- **AC-AGENTS-SESSION-CEILING-001.7:** The startup environment variable
  `KANDEV_MAX_CONCURRENT_SESSIONS` shall accept a non-negative integer, use zero
  for unlimited, and use a derived default with a minimum of two when unset or
  invalid.

## Out of scope

- Per-workspace or per-user ceilings.
- A frontend settings control for this startup-only value.
- Replacing the orchestrator's existing launch seams or task repository.
