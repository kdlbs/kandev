---
status: active
system: integrations
created: 2026-09-14
owners:
  - kandev
---

# Provider failure backoff and health Requirements

## Overview

Failed GitHub credentials and workflow-sync configuration currently cost a
provider call per poll or sync cycle. Invalid credentials and misconfiguration
are durable until an operator acts, so retrying them every cycle wastes quota
and obscures the real state. Operators need visible degraded, healthy, and
disabled integration states without exposing secrets.

## Requirements

### REQ-INTEGRATIONS-PROVIDER-BACKOFF-001: Generation-aware provider failure backoff

**Intent:** Authentication and configuration failures on background
integration loops shall enter exponential backoff with fingerprint-aware
reset, so a credential or configuration change resumes probing on the very
next cycle.

#### Acceptance criteria

- **AC-INTEGRATIONS-PROVIDER-BACKOFF-001.1:** A background loop observing an
  authentication or configuration failure shall open a per-target circuit
  keyed by the owning workspace or configuration and skip further provider
  calls for that target until its backoff expires.
- **AC-INTEGRATIONS-PROVIDER-BACKOFF-001.2:** A non-secret connection
  fingerprint (status and credential generation) shall reset an open circuit
  when it changes, forcing an immediate probe after a credential rotate,
  reconnect, or re-auth; an empty or unknown fingerprint shall never reset an
  open circuit.
- **AC-INTEGRATIONS-PROVIDER-BACKOFF-001.3:** While a circuit is open, the
  loop shall make zero additional provider calls for that target; after a
  reset or expiry, one probe resumes evaluation.
- **AC-INTEGRATIONS-PROVIDER-BACKOFF-001.4:** Recorded circuit state shall be
  bounded and never include secret material, tokens, or user-controlled values.
- **AC-INTEGRATIONS-PROVIDER-BACKOFF-001.5:** Failures shall be classified as
  auth, config, or transient; a client capability fallback (for example, no
  GraphQL support) is deliberately not a failure and shall not open any
  circuit.

### REQ-INTEGRATIONS-PROVIDER-BACKOFF-002: Visible integration health

**Intent:** Operators shall see at a glance whether an integration is
healthy, degraded, or disabled, and why, without secret exposure.

#### Acceptance criteria

- **AC-INTEGRATIONS-PROVIDER-BACKOFF-002.1:** Health and status output shall
  distinguish healthy, degraded, and disabled integration states.
- **AC-INTEGRATIONS-PROVIDER-BACKOFF-002.2:** Health aggregates shall count
  open circuits by failure class only, without per-workspace identifiers or
  secret material in messages.
- **AC-INTEGRATIONS-PROVIDER-BACKOFF-002.3:** Circuit skip, reset, and failure
  counters shall be exposed as bounded-label metrics (provider and class
  only).

## Out of scope

- Frontend settings presentation of integration status, owned by the UI
  system's settings surfaces.
- Storage and runtime gauge metrics, owned by the system-page and platform
  observability contracts.
