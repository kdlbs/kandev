---
id: "01-sweep-orphaned-agentctl"
title: "Sweep orphaned agentctl on SSH reachability"
status: planned
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-EXECUTOR-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-EXECUTOR-001.13
  - AC-EXECUTORS-SSH-EXECUTOR-001.14
  - AC-EXECUTORS-SSH-EXECUTOR-001.15
  - AC-EXECUTORS-SSH-EXECUTOR-001.16
  - AC-EXECUTORS-SSH-EXECUTOR-001.17
system_design:
  - ../../specs/executors/system-design/ssh-executor.md
---

# Task 01: Sweep Orphaned agentctl on SSH Reachability

## Summary

Add a per-SSH-executor sweep, triggered when the executor becomes reachable
and on a slow interval, that lists remote `agentctl` processes, attributes
each to a session through its `agentctl.pid` file or to a task through its
`--workdir`, and stops the ones whose session or task is terminal. Fix the
macOS absence probe in `remoteProcessCommandLineCommand`.

## Files

- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_orphan_sweep.go`
  (new): inventory command and parser, ownership decision, stop ladder.
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_operations.go`:
  macOS absence in `remoteProcessCommandLineCommand`.
- Sweep scheduler wiring (reachability event subscription plus interval) at
  the startup composition point that already owns the reachability publisher.

## Validation

- Unit tests with the fake SSH runner: each decision branch in
  `AC-EXECUTORS-SSH-EXECUTOR-001.14` and every preserve branch in `.15`.
- Stop-ladder command test including child process groups (`.16`).
- Absence probe test for a macOS-shaped host (`.17`).
- `make -C apps/backend test lint`.
