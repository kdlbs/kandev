---
status: active
system: executors
created: 2026-09-09
owners:
  - kandev
---

# Kubernetes Startup Timing Requirements

## Overview

Operators need to locate slow Kubernetes environment starts before they choose
an optimization. The executor system owns these stage boundaries and their results.
This capability measures environment creation, not the first agent response.

## Requirements

### REQ-EXECUTORS-K8S-TIMING-001: Attributable launch durations

**Intent:** An operator can identify which completed or failed environment stage
accounts for a launch delay.

#### Acceptance criteria

- **AC-EXECUTORS-K8S-TIMING-001.1:** Each fresh environment attempt shall expose
  durations for entered storage, Pod readiness, bootstrap, and agentctl-connection stages in backend diagnostic logs.
- **AC-EXECUTORS-K8S-TIMING-001.2:** Each returned attempt shall expose one
  total duration and outcome. Records shall identify the attempt and its task/session without relying on log order.
- **AC-EXECUTORS-K8S-TIMING-001.3:** A failed, canceled, or timed-out stage shall
  retain its elapsed duration and outcome. Stages that never start shall have no fabricated measurements.
- **AC-EXECUTORS-K8S-TIMING-001.4:** Documentation shall distinguish Pod readiness
  wait from image-pull time, environment readiness from agent readiness, and fresh attempts from reconnects.

### REQ-EXECUTORS-K8S-TIMING-002: Passive diagnostics

**Intent:** Timing evidence must not change the workload or expose task secrets.

#### Acceptance criteria

- **AC-EXECUTORS-K8S-TIMING-002.1:** Timing collection shall preserve launch,
  cancellation, timeout, reconnect, checkpoint, and cleanup outcomes without additional cluster requests.
- **AC-EXECUTORS-K8S-TIMING-002.2:** Timing records shall exclude credentials,
  bootstrap nonces, repository URLs, scripts, environment values, Pod templates, and raw error messages.
- **AC-EXECUTORS-K8S-TIMING-002.3:** Timing durations shall remain non-negative
  under wall-clock changes. Concurrent attempts shall retain separate measurements.

## Out of scope

- New UI progress stages, historical dashboards, metrics endpoints, or tracing dependencies.
- Timings for provider inference, agent installation, repository preparation, or the first response outside environment creation.
- Reconnect, Pod replacement, and container-restart timing in this first package.
- New retry, timeout, autoscaling, or warm-pool behavior.
- Guaranteed final timing records after process termination or a machine crash.
