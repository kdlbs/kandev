---
created: 2026-09-13
status: done
requirements:
  - REQ-EXECUTORS-SURVIVAL-001
  - REQ-EXECUTORS-SURVIVAL-002
  - REQ-EXECUTORS-SURVIVAL-003
  - REQ-EXECUTORS-SURVIVAL-004
  - REQ-EXECUTORS-SURVIVAL-005
  - REQ-EXECUTORS-CONTROL-OWNERSHIP-001
  - REQ-EXECUTORS-CONTROL-OWNERSHIP-002
  - REQ-EXECUTORS-CONTROL-OWNERSHIP-003
  - REQ-EXECUTORS-CONTROL-OWNERSHIP-004
system_design:
  - ../../specs/executors/system-design/agent-survival-across-restart-01.md
  - ../../specs/executors/system-design/agent-survival-across-restart-02.md
  - ../../specs/executors/system-design/agent-survival-across-restart-03.md
legacy_specs: []
---

# Implementation plan: agent survival across a backend restart

## Overview

Make agent work survive a Kandev backend restart or upgrade for the worktree and
local executor types. Both map to the standalone runtime, which supervises every
instance from a single agentctl control server, so exactly one process has to
survive rather than one per session.

Before this work every backend restart stopped every running agent. In-flight
work was lost and each task fell back to a cold resume. The executors system owns
the package because it owns executor-specific failure and recovery contracts.

The [durable session reconciliation package](../durable-agent-session-reconciliation/plan.md)
extends this process-survival contract with authenticated delivery adoption,
ordered replay, and recovery guards. Durable delivery remains independent of
this plan's process-survival toggle.

## Scope

In scope: detached agentctl lifetime across the three kill paths (parent-liveness
pipe, manager stop, launcher cleanup), adoption with installation-ownership proof
and fencing epochs, the `RecoverInstances` producer for the standalone backend,
the `GET /api/v1/instances` response-envelope fix, durable terminal turn state so
a turn that ends while unattached is not lost, liveness that never reports a
failed enumeration as healthy, and a runtime feature flag whose disabled path
reproduces the previous behavior exactly.

Out of scope: Docker, SSH, Sprites and remote-docker survival, provider-native
resume semantics, host reboot survival, and transcript replay beyond the terminal
turn outcome.

## Work packages

- [x] [Task 01: Survive a backend restart on the standalone runtime](task-01-worktree-local-agent-survival.md)

## Validation

The capability ships behind a runtime feature flag. With the flag off, behavior is
byte-for-byte the previous behavior, which is what REQ-EXECUTORS-SURVIVAL-005 and
its acceptance criteria pin. With the flag on, the acceptance criteria listed in
the work order are covered by Go unit and integration tests alongside the changed
packages and by Playwright end-to-end coverage for the session-recovery surfaces.
