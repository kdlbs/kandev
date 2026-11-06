---
status: active
system: platform
created: 2026-10-23
owners:
  - kandev
---

# Agentctl operator profiling requirements

## Overview

An operator needs an explicit local profiling listener for a running agentctl
hosting multiple sessions. Existing per-session routers retain their token
authentication; profiling must remain disabled unless explicitly requested.

## Requirements

### REQ-PLATFORM-AGENTCTL-OPERATOR-PROFILING-001: Optional loopback profiling

**Intent:** Allow local diagnosis without exposing profiles on a public address.

**User story:** As an operator, I can opt into a separate local profiling address
without changing the existing session API or its authentication.

#### Acceptance criteria

- **AC-PLATFORM-AGENTCTL-OPERATOR-PROFILING-001.1:** An unset or blank
  `KANDEV_AGENTCTL_PPROF_ADDR` starts no additional listener.
- **AC-PLATFORM-AGENTCTL-OPERATOR-PROFILING-001.2:** An explicit loopback
  host/port serves the standard pprof endpoints on a separate HTTP mux.
  Non-loopback or malformed addresses are refused.
- **AC-PLATFORM-AGENTCTL-OPERATOR-PROFILING-001.3:** Listener startup failure
  is logged without terminating agentctl or changing its session routers.

## Out of scope

- Enabling profiling in an operating installation, remote access, or a new API.
- Automatic profiling, a persistent collector, or session authentication changes.
