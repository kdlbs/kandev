---
status: active
system: executors
created: 2026-10-12
owners:
  - kandev
---

# Agentctl instance creation deadline and concurrency

## Overview

The backend creates an agent instance through the agentctl control API before
an agent can start. Instance creation does more work than other control calls:
the controller starts the instance's process manager and workspace trackers
before it responds. Under concurrent session starts, that response can take
longer than the deadline used for short control calls.

Two contracts address this. Instance creation has its own bounded client
deadline, while other control calls keep their existing deadline. The
controller also runs concurrent creations side by side instead of queueing each
one behind another creation's startup, without weakening instance ownership,
abandonment cleanup, or shutdown.

Executors owns this contract because it defines how execution environments
reach their controller.

## Terminology

- **Control call:** A backend request to the agentctl control API, such as
  instance creation, instance deletion, health, or diagnostics.
- **Creation deadline:** The longest time the backend waits for a controller
  response to one instance creation request.
- **Startup:** The slow part of creation on the controller: building the
  process manager, preparing comparison targets, and starting workspace
  trackers.
- **Published instance:** An instance that lookup, listing, and stop can find.

## Requirements

### REQ-EXECUTORS-AGENTCTL-CREATE-DEADLINE-001: Bounded instance creation wait

**Intent:** A controller that is slow but still progressing can finish
creating an instance, while the backend never waits for creation without a
bound.

#### Acceptance criteria

- **AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-001.1:** When the backend sends an
  instance creation request, the system shall wait up to 120 seconds for the
  controller response before it fails the request.
- **AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-001.2:** Control calls other than
  instance creation shall keep their existing 30-second deadline.
- **AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-001.3:** When the caller cancels
  instance creation or its own deadline expires first, the system shall stop
  waiting at that point and report the creation failure to the caller.
- **AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-001.4:** Instance creation shall use
  the same authentication credential and runtime binding as other control calls
  from the same client.

### REQ-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002: Concurrent instance creation

**Intent:** One slow creation does not delay other creations or operations on
other instances, and every creation still ends published or fully cleaned up.

#### Acceptance criteria

- **AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002.1:** When several creations are
  in progress on one controller, each creation's startup shall run without
  waiting for another creation's startup to finish.
- **AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002.2:** While a creation owns an
  instance ID, another creation for the same ID shall be rejected without
  waiting for the first creation's startup. Each in-progress creation shall
  hold its own port.
- **AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002.3:** When the caller gives up
  before the instance is published, the controller shall not publish it. It
  shall release the instance's listener, process resources, and port, or retain
  them for a later cleanup retry when cleanup fails.
- **AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002.4:** When controller shutdown
  begins while a creation is in progress, shutdown shall not return until that
  creation has ended. Afterward, the instance shall not be published and its
  port shall not remain reserved.
- **AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002.5:** Lookup, listing, and stop
  of other instances shall not wait for an in-progress creation's startup. A
  published instance shall always be serving its HTTP endpoint.

## Out of scope

- Creation that takes longer than the creation deadline. Such a request still
  fails with a timeout.
- Making a single creation's startup faster.
- Retry, backoff, or failure classification for instance creation.
- Deadlines for agent launch phases outside the control call.
- A user or operator setting for the creation deadline or for creation
  concurrency.

## Related design

- [Agentctl instance creation deadline and concurrency](../system-design/agentctl-instance-create-deadline.md)
- [Agentctl authentication modes](agentctl-authentication-modes.md)
