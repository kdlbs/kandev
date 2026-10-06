---
status: active
system: agents
created: 2026-10-04
updated: 2026-10-04
owners:
  - Kandev
---

# Agent runtime update summary requirements

## Overview

Startup runtime checks can announce several available updates together. One delayed summary lets users start work without a stack of agent notices.
Agents owns this contract because runtime identities, discovery, and update outcomes determine summary membership. Platform supplies preference-aware notification delivery.

These requirements extend [runtime awareness](runtime-update-notifications.md), specifically AC-AGENTS-RUNTIME-NOTIFY-001.3, with grouped availability delivery.
The existing runtime identity and version information remains available in Settings.

## Terminology

- **Availability notice:** Information that a newer agent runtime version is available.
- **Summary:** One notification for availability notices collected during a fixed window.
- **Outcome notice:** A succeeded, failed, or interrupted update result.

## Requirements

### REQ-AGENTS-RUNTIME-NOTIFY-003: Delayed availability summary

**Intent:** Reduce interruptions from concurrent runtime checks, especially when a user opens Kandev.

#### Acceptance criteria

- **AC-AGENTS-RUNTIME-NOTIFY-003.1:** Kandev shall collect availability notices for 30 seconds before delivery. Later arrivals shall not extend that window.
- **AC-AGENTS-RUNTIME-NOTIFY-003.2:** Startup checks and reconnect replay shall produce one availability notification per window and enabled delivery channel. Periodic check bursts shall use the same behavior.
- **AC-AGENTS-RUNTIME-NOTIFY-003.3:** A summary shall show the number of distinct agent runtimes with newly announced versions. Its Review updates action shall open the expanded runtime section in Settings > Agents.
- **AC-AGENTS-RUNTIME-NOTIFY-003.4:** Settings shall retain each runtime's identity, current version, latest version, ownership, and supported update actions. A single eligible runtime shall retain its named notice and direct runtime link after the delay.
- **AC-AGENTS-RUNTIME-NOTIFY-003.5:** Delivery shall respect recipient and channel preferences. Reconnect, restart, reordered members, and changed summary membership shall not repeat successfully announced runtime versions.
- **AC-AGENTS-RUNTIME-NOTIFY-003.6:** Before delivery, Kandev shall exclude runtimes that are disabled, unavailable, already updated, or no longer known to have a newer version. An empty group shall produce no notification.
- **AC-AGENTS-RUNTIME-NOTIFY-003.7:** Update outcomes shall bypass the availability window. Kandev release notices, task completion notices, and questions shall retain their separate delivery behavior.
- **AC-AGENTS-RUNTIME-NOTIFY-003.8:** On desktop and phone, a summary shall use one compact notification and explicit navigation. Phone actions shall have at least 44px touch targets, with no horizontal page overflow. Web copy shall use every shipped locale.
- **AC-AGENTS-RUNTIME-NOTIFY-003.9:** A failed delivery or disconnected local subscriber shall leave affected versions eligible for later replay. Shutdown shall cancel pending delivery; restart shall rebuild eligibility from current status.

## Out of scope

Generic grouping rules, a notification inbox, new preferences, automatic runtime activation, persistent floating indicators, and modal-aware delivery are excluded.
The window starts with availability discovery, rather than page readiness. This package does not change existing claim-before-send crash guarantees.

## Design and delivery

See the [summary design](../system-design/runtime-update-summary.md) and [implementation plan](../../../plans/agent-runtime-update-summary/plan.md).
