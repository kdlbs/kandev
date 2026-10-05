---
status: draft
system: platform
created: 2026-10-05
owners:
  - kandev
---

# Runtime diagnostic signal requirements

## Overview

Platform owns shared diagnostic logging and runtime log forwarding.
Operators need actionable failures to remain visible amid routine debug activity.
The existing [severity contract](expected-runtime-log-severity.md) and [logging contract](diagnostic-logging.md) remain authoritative.

## Requirements

### REQ-PLATFORM-DIAGNOSTIC-SIGNAL-001: Structured child severity

**Intent:** Agentctl JSON failures remain visible at the parent's normal file threshold.

#### Acceptance criteria

- **AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.1:** A recognized agentctl JSON record shall retain its declared severity in the parent log.
  Error-class records shall remain visible with an info file threshold.
- **AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.2:** Malformed JSON, payload-only JSON, and unknown levels shall retain the existing stream-specific fallback.
  A level word inside message content shall not determine severity.
- **AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.3:** Existing console and slog records shall retain their severity.
  Forwarding shall not convert a fatal child record into parent-process termination.

### REQ-PLATFORM-DIAGNOSTIC-SIGNAL-002: Quiet routine diagnostics

**Intent:** Repetitive normal activity does not displace failure evidence.

#### Acceptance criteria

- **AC-PLATFORM-DIAGNOSTIC-SIGNAL-002.1:** An unchanged required-store health result shall not emit another info-level state-transition entry.
  Changes to health state or affected store identities shall remain visible.
- **AC-PLATFORM-DIAGNOSTIC-SIGNAL-002.2:** A routine idle-reclaim refusal shall not emit a per-session debug entry on every call.
  Successful reclaim and genuine liveness-probe failures shall retain their diagnostic evidence and behavior.
- **AC-PLATFORM-DIAGNOSTIC-SIGNAL-002.3:** When a supported MCP transport survives for a server name, rejecting its unsupported alternative shall use debug severity.
  If no supported transport survives, the capability refusal shall remain a warning.
- **AC-PLATFORM-DIAGNOSTIC-SIGNAL-002.4:** Warning and error records shall not enter a global sampling or suppression policy.
  Explicit debug thresholds, log paths, retention, and diagnostic bundles shall retain their existing contracts.

## Out of scope

- Changing production log thresholds or globally sampling event and WebSocket records.
- New user settings, log retention limits, raw ACP collection, or metric label sets.
- Changing reclaim decisions or MCP transport selection.

## Related documents

- [System design](../system-design/runtime-diagnostic-signal.md)
- [Implementation plan](../../../plans/runtime-log-reliability/plan.md)
