---
id: "01-worktree-local-agent-survival"
title: "Survive a backend restart on the standalone runtime"
status: done
wave: 1
depends_on: []
plan: "plan.md"
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
acceptance_criteria:
  - AC-EXECUTORS-SURVIVAL-001.1
  - AC-EXECUTORS-SURVIVAL-001.2
  - AC-EXECUTORS-SURVIVAL-001.3
  - AC-EXECUTORS-SURVIVAL-001.4
  - AC-EXECUTORS-SURVIVAL-001.5
  - AC-EXECUTORS-SURVIVAL-001.6
  - AC-EXECUTORS-SURVIVAL-001.8
  - AC-EXECUTORS-SURVIVAL-001.9
  - AC-EXECUTORS-SURVIVAL-001.10
  - AC-EXECUTORS-SURVIVAL-001.7
  - AC-EXECUTORS-SURVIVAL-002.1
  - AC-EXECUTORS-SURVIVAL-002.2
  - AC-EXECUTORS-SURVIVAL-002.10
  - AC-EXECUTORS-SURVIVAL-002.3
  - AC-EXECUTORS-SURVIVAL-002.14
  - AC-EXECUTORS-SURVIVAL-002.4
  - AC-EXECUTORS-SURVIVAL-002.6
  - AC-EXECUTORS-SURVIVAL-002.13
  - AC-EXECUTORS-SURVIVAL-002.5
  - AC-EXECUTORS-SURVIVAL-002.15
  - AC-EXECUTORS-SURVIVAL-002.16
  - AC-EXECUTORS-SURVIVAL-002.7
  - AC-EXECUTORS-SURVIVAL-002.8
  - AC-EXECUTORS-SURVIVAL-002.9
  - AC-EXECUTORS-SURVIVAL-002.11
  - AC-EXECUTORS-SURVIVAL-002.12
  - AC-EXECUTORS-SURVIVAL-003.1
  - AC-EXECUTORS-SURVIVAL-003.7
  - AC-EXECUTORS-SURVIVAL-003.2
  - AC-EXECUTORS-SURVIVAL-003.3
  - AC-EXECUTORS-SURVIVAL-003.4
  - AC-EXECUTORS-SURVIVAL-003.5
  - AC-EXECUTORS-SURVIVAL-003.6
  - AC-EXECUTORS-SURVIVAL-004.1
  - AC-EXECUTORS-SURVIVAL-004.6
  - AC-EXECUTORS-SURVIVAL-004.4
  - AC-EXECUTORS-SURVIVAL-004.2
  - AC-EXECUTORS-SURVIVAL-004.5
  - AC-EXECUTORS-SURVIVAL-004.3
  - AC-EXECUTORS-SURVIVAL-005.1
  - AC-EXECUTORS-SURVIVAL-005.2
  - AC-EXECUTORS-SURVIVAL-005.3
  - AC-EXECUTORS-SURVIVAL-005.4
  - AC-EXECUTORS-SURVIVAL-005.5
  - AC-EXECUTORS-SURVIVAL-005.7
  - AC-EXECUTORS-SURVIVAL-005.6
  - AC-EXECUTORS-CONTROL-OWNERSHIP-001.1
  - AC-EXECUTORS-CONTROL-OWNERSHIP-001.2
  - AC-EXECUTORS-CONTROL-OWNERSHIP-001.3
  - AC-EXECUTORS-CONTROL-OWNERSHIP-001.4
  - AC-EXECUTORS-CONTROL-OWNERSHIP-001.5
  - AC-EXECUTORS-CONTROL-OWNERSHIP-001.6
  - AC-EXECUTORS-CONTROL-OWNERSHIP-001.9
  - AC-EXECUTORS-CONTROL-OWNERSHIP-001.10
  - AC-EXECUTORS-CONTROL-OWNERSHIP-001.11
  - AC-EXECUTORS-CONTROL-OWNERSHIP-001.7
  - AC-EXECUTORS-CONTROL-OWNERSHIP-001.8
  - AC-EXECUTORS-CONTROL-OWNERSHIP-002.1
  - AC-EXECUTORS-CONTROL-OWNERSHIP-002.7
  - AC-EXECUTORS-CONTROL-OWNERSHIP-002.6
  - AC-EXECUTORS-CONTROL-OWNERSHIP-002.10
  - AC-EXECUTORS-CONTROL-OWNERSHIP-002.5
  - AC-EXECUTORS-CONTROL-OWNERSHIP-002.9
  - AC-EXECUTORS-CONTROL-OWNERSHIP-002.2
  - AC-EXECUTORS-CONTROL-OWNERSHIP-002.8
  - AC-EXECUTORS-CONTROL-OWNERSHIP-002.3
  - AC-EXECUTORS-CONTROL-OWNERSHIP-002.4
  - AC-EXECUTORS-CONTROL-OWNERSHIP-003.1
  - AC-EXECUTORS-CONTROL-OWNERSHIP-003.2
  - AC-EXECUTORS-CONTROL-OWNERSHIP-003.8
  - AC-EXECUTORS-CONTROL-OWNERSHIP-003.3
  - AC-EXECUTORS-CONTROL-OWNERSHIP-003.4
  - AC-EXECUTORS-CONTROL-OWNERSHIP-003.6
  - AC-EXECUTORS-CONTROL-OWNERSHIP-003.7
  - AC-EXECUTORS-CONTROL-OWNERSHIP-003.9
  - AC-EXECUTORS-CONTROL-OWNERSHIP-003.5
  - AC-EXECUTORS-CONTROL-OWNERSHIP-004.1
  - AC-EXECUTORS-CONTROL-OWNERSHIP-004.3
  - AC-EXECUTORS-CONTROL-OWNERSHIP-004.2
  - AC-EXECUTORS-CONTROL-OWNERSHIP-004.7
  - AC-EXECUTORS-CONTROL-OWNERSHIP-004.4
  - AC-EXECUTORS-CONTROL-OWNERSHIP-004.5
  - AC-EXECUTORS-CONTROL-OWNERSHIP-004.6
system_design:
  - ../../specs/executors/system-design/agent-survival-across-restart-01.md
  - ../../specs/executors/system-design/agent-survival-across-restart-02.md
  - ../../specs/executors/system-design/agent-survival-across-restart-03.md
---

# Task 01: Survive a backend restart on the standalone runtime

## Summary

Keep the standalone agentctl control server alive across a backend restart or
upgrade, then re-adopt it safely and rebuild the executions it was supervising.
Delivered in layers so that a working product existed at every step: runtime flag
and configuration plumbing, the `ListInstances` response-envelope fix, a
read-only recovery-inventory port, the control-server record table, secret-store
credential handling, the agentctl server side, the backend adoption and recovery
guard with `RecoverInstances`, detach and turn-outcome handling, liveness, then
the frontend, localization and end-to-end coverage.

## In scope

- `apps/backend/internal/agent/runtime/lifecycle` (recovery, adoption, detach,
  liveness, `RecoverInstances` for the standalone backend)
- `apps/backend/internal/agentctl` (control server, adoption and ownership
  endpoints, durable terminal turn state)
- `apps/backend/internal/agent/runtime/agentctl` (control client, response
  envelope)
- `apps/backend/internal/backendapp` (startup recovery guard, ownership lock,
  launcher cleanup)
- `apps/backend/internal/runtimeflags` and `profiles.yaml` (the release toggle)
- `apps/web` session-recovery surfaces and their locale catalogs
- Go tests alongside each changed package, plus Playwright end-to-end specs

## Out of scope

Docker, SSH, Sprites and remote-docker survival, provider-native resume
semantics, host reboot survival, and transcript replay beyond the terminal turn
outcome. Those are separate cards.

## Acceptance

Every acceptance criterion listed in this work order's frontmatter is
implemented. Adoption proves installation ownership in both directions and is
fenced by an epoch enforced on the agentctl side, so two backends cannot drive
one control server. Re-tracking refuses an instance whose required fields cannot
be restored, and records which field was missing, rather than registering a
partially reconstructed execution. A turn that completes while no backend is
attached is recorded durably and is read back on adoption. With the feature flag
disabled the previous stop-everything behavior is reproduced exactly.
