---
created: 2026-09-27
status: planned
requirements:
  - REQ-EXECUTORS-SSH-EXECUTOR-001
system_design:
  - ../../specs/executors/system-design/ssh-executor.md
legacy_specs: []
---

# Implementation Plan: Sweep Orphaned agentctl off SSH Hosts

## Overview

PR #3527 reaps a remote `agentctl` only when a stop runs against a surviving
`executors_running` row. Stops on a lost SSH transport skip the remote kill,
and reconciliation paths delete rows without a remote stop, so the pid is
forgotten while the process keeps running. On one SSH host, 6 of 8 live
`agentctl` processes (about 1.6 GB) belonged to terminal sessions with no
runtime row. Add a reachability-triggered and interval sweep that reconciles
the host's process table against Kandev's session state, and fix the macOS
absence probe that makes every persisted stop of an exited pid fail.

## Work orders

| Order | Title | Requirements |
| --- | --- | --- |
| [task-01](task-01-sweep-orphaned-agentctl.md) | Sweep orphaned agentctl on SSH reachability | `REQ-EXECUTORS-SSH-EXECUTOR-001` (`AC-EXECUTORS-SSH-EXECUTOR-001.13` to `.17`) |

## Out of scope

- Remote Docker and Sprites executors: `agentctl` runs inside a container or
  sandbox whose own lifecycle removes it. Not measured on this card.
- Remote task directory removal (owned by remote task directory reclamation).
- The detached-agent `agent.pgid` record (card `bc8b1647`); not on `main`.
