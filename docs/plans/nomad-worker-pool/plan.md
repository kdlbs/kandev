---
created: 2026-10-25
status: done
requirements:
  - REQ-EXECUTORS-NOMADPOOL-001
  - REQ-EXECUTORS-NOMADPOOL-002
  - REQ-EXECUTORS-NOMADPOOL-003
  - REQ-EXECUTORS-NOMADPOOL-004
  - REQ-EXECUTORS-NOMADPOOL-005
  - REQ-EXECUTORS-NOMADPOOL-006
system_design:
  - ../../specs/executors/system-design/nomad-worker-pool.md
  - ../../specs/executors/system-design/nomad-native-executor.md
legacy_specs: []
---

# Implementation Plan: Nomad worker pool

## Overview

Generalize the native Nomad executor from one pinned worker to a pool that the
Nomad server places on, including the Kandev host. A pinned node becomes an
optional constraint. This plan starts after the native executor's work orders
01-07 and replaces its work order 08: the first contribution handoff is
pool-capable. The optional sandbox wrapper and a per-node mirror are phase 2 in
the requirements and are not planned here.

## Scope

### In scope

- Pool profile, node metadata contract and onboarding script.
- Placement by node pool with spread (default) or binpack, optional pin, resume
  affinity, and the placed NodeID in durable identity.
- Loopback transport for the local node with host-identity verification; SSH
  transport resolved from the placed node's metadata.
- Local worktree workspaces; remote fresh single-branch checkouts.
- Per-environment fences, several environments per node, small reservations
  with memory maximum, per-node quarantine.
- Scratch inside the allocation directory; worktree or checkout removal on
  proven cleanup.
- Two-node live proof, built-runtime proof and contribution handoff.

### Out of scope

- Optional sandbox wrapper and per-node mirror (phase 2).
- Load-aware placement, autoscaling, workspace recreation on another node.
- Rendered UI, production rollout, hard isolation, Linux cgroup enforcement.

## Technical approach

Reuse the native executor's job, identity, recovery, drain and quarantine code;
change only what assumes one known worker. The job specification gains
node-pool and metadata constraints and loses the fixed node constraint unless a
pin is set. After registration, the allocation's NodeID selects the transport
and workspace strategy before any connection. Exclusivity moves from a
worker-wide fence to `<state_root>/environments/<environment-id>/active`.
Quarantine records the NodeID and touches only that node's eligibility.

## Tests

Offline: job specification constraints and affinity, profile validation, node
metadata validation, local identity mismatch, remote host-key mismatch, transport
selection, workspace strategy selection, two-environment isolation on one node,
per-node quarantine, scratch path assertions.

## E2E tests

The self-creating runner gains a two-client local fixture: one Nomad client
declared `local`, one declared `ssh` behind a fixture sshd, one throwaway Nomad
server. Pool scenarios cover spread placement, queueing at capacity, single-node
partition without effect on the other node, proven cleanup of worktree and
scratch, per-node quarantine with continued placement, and refusal of a wrong
host key and of a fake local node. Live runs need owner approval.

## Work orders

- [x] [Task 01: Pool placement and transport](task-01-pool-placement-transport.md)
- [x] [Task 02: Pool workspaces, fences and quarantine](task-02-pool-workspaces-fences.md)
- [x] [Task 03: Two-node proof and handoff](task-03-pool-proof-handoff.md)

## Evidence boundary

Completed work orders use the disposable laptop fixture and the evidence disclosures in
their Results sections. The 2026-10-28 closeout records 14 operator-reported default-mode
scenario passes and current local verification in
[`docs/evidence/nomad-e2e-coverage/`](../../evidence/nomad-e2e-coverage/README.md).
TLS/scoped-ACL live mode, live SFTP upload cancellation, and an actual native agent-to-Kandev
tool call after restart remain unproven. Completion does not certify the Tank/Neo pool or
authorize production rollout.

Execute 01 -> 02 -> 03 after native executor work orders 01-07 are done.

## Risks

- Several environments per node change assumptions in the native executor's
  start fence and cleanup; every per-worker check must be audited.
- The local node shares a machine with Kandev; reserved headroom and roots
  outside Kandev's directories are mandatory, not optional tuning.
- Placement is reservation-based; operators may expect live-load balancing.
- Memory oversubscription must be enabled in the Nomad scheduler configuration;
  preflight must detect when it is off.
