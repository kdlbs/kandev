---
status: active
system: platform
created: 2026-10-20
owners:
  - kandev
---

# Agentctl Client Lease

## Overview

Each agent execution pins its agentctl client with a read lease for a bounded
operation. A terminal stop takes the write side of the same lease while it
closes and detaches the client. A recovered prompt stream that restores a
failed execution can race that stop. Reading the agentctl URL during
persistence must never wait on the lease, or one session's stop wedges the
execution store for every session.

## Requirements

### REQ-PLATFORM-AGENTCTL-CLIENT-LEASE-001: Lease reads never stall the execution store

**Intent:** A stop and a recovery of the same execution never stop the
backend from launching or streaming other sessions.

**User story:** As a user running several sessions, I want one session's failed
turn and queued follow-up to settle without freezing my other sessions.

#### Acceptance criteria

- **AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.1:** Reading an execution's agentctl
  URL shall not wait on the agentctl client lease, including when the caller
  already holds a read lease and a terminal stop is waiting for the write side.
- **AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.2:** Restoring a recovered failed
  execution shall release its client lease after the agent status read and
  before it marks the execution ready and persists it.
- **AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.3:** The URL read shall return the
  current client's URL and an empty value only when no client is set. While a
  client replacement or detach is in progress, it shall return the URL that
  writer published before taking the client source write side, which can be the
  previous client or the client the replacement is publishing. A concurrent
  persistence write shall therefore never blank a live endpoint.

- **AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.4:** Replaying a recovered durable
  delivery stream shall hold the client lease only while it fetches each page,
  and shall release it before any lifecycle callback, projection or persistence
  runs for that page. A replay whose client was replaced or detached between
  pages shall fail instead of reading from another connection.
- **AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.5:** A prompt cancel shall hold the
  client lease only for the cancel request. Its escalation shall hold the lease
  only for the durable-submission settle request, and shall not hold it while
  it waits for the prompt to finish or while it marks the execution ready and
  persists it.

## Compatibility and exclusions

Stop, client replacement, detach, and every other lease user are unchanged.

## Design and delivery

- [System design](../system-design/agentctl-client-lease.md)
- [Delivery package](../../../plans/agentctl-lease-deadlock/plan.md)
