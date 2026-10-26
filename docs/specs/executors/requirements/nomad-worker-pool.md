---
status: draft
system: executors
created: 2026-10-25
owners:
  - kandev
---

# Nomad worker pool requirements

## Overview

Operators can point one Nomad executor profile at a Nomad server and let the
server place each environment on any eligible worker in a pool, including the
machine that runs Kandev. A pinned single worker becomes a pool of one. Nomad
owns placement, capacity accounting and worker liveness; Kandev owns how it
reaches the placed worker, the workspace, the agent lifecycle, and cleanup proof.

This extends the [native Nomad executor](nomad-native-executor.md). Every
guarantee there (exact ownership and recovery, cooperative cleanup and
quarantine, authenticated delivery) applies per environment on whichever worker
Nomad selects. The reference scenario is a small fleet in which one machine
hosts Kandev and also contributes compute, the others are workers only, idle
sessions must not consume peak capacity, and a finished environment must leave
no agent scratch files behind.

## Terminology

- **Pool:** The set of Nomad client nodes a profile may place on, selected by
  Nomad node pool and constraints.
- **Local node:** A pool node that is the Kandev host itself, reached over
  loopback without SSH.
- **Remote node:** A pool node reached through the pinned SSH transport.
- **Placement:** Nomad's selection of a node for one environment's allocation.
- **Scratch:** Agent temporary files, as distinct from the retained workspace.

## Requirements

### REQ-EXECUTORS-NOMADPOOL-001: Pool placement

**Intent:** Operators add machines to a pool once and stop choosing a target
machine for each task.

#### Acceptance criteria

- **AC-EXECUTORS-NOMADPOOL-001.1:** When an environment launches from a pool
  profile, the system shall submit it to the profile's Nomad server and node
  pool without naming a node, and shall record the node Nomad selected as part
  of the environment's durable identity.
- **AC-EXECUTORS-NOMADPOOL-001.2:** Where a profile sets a pinned node, the
  system shall constrain placement to that node and otherwise behave exactly as
  the native Nomad executor.
- **AC-EXECUTORS-NOMADPOOL-001.3:** Where a profile selects spread placement
  (the default), Nomad shall place each environment on the eligible node with
  the lowest reserved-resource utilisation; where it selects binpack, Nomad
  shall fill a node before using another. Placement stays reservation-based,
  not live-load-based.
- **AC-EXECUTORS-NOMADPOOL-001.4:** If no pool node can satisfy an environment's
  reservations, the system shall leave it queued, show it as waiting for
  capacity, and not place it on a node outside the pool.
- **AC-EXECUTORS-NOMADPOOL-001.5:** If a node lacks valid Kandev node metadata,
  the system shall never place an environment on it.

### REQ-EXECUTORS-NOMADPOOL-002: Local and remote transport

**Intent:** One profile runs work on the Kandev host and on other machines
without separate executors.

#### Acceptance criteria

- **AC-EXECUTORS-NOMADPOOL-002.1:** When an environment is placed on a node
  declared local, the system shall reach agentctl over loopback without SSH.
- **AC-EXECUTORS-NOMADPOOL-002.2:** Before using a node as local, the system
  shall verify the node's host identity against an identity token on the Kandev
  host's own disk. If they differ, the system shall refuse the environment and
  shall not fall back to SSH.
- **AC-EXECUTORS-NOMADPOOL-002.3:** When an environment is placed on a remote
  node, the system shall resolve the SSH host, port and host-key fingerprint
  from that node's metadata and connect only if the presented key matches. A
  missing or mismatched key shall fail closed.
- **AC-EXECUTORS-NOMADPOOL-002.4:** When the local node is in the pool, its
  Nomad client shall reserve CPU and memory headroom for Kandev, and agent
  workspace and allocation roots shall lie outside Kandev's home and data
  directories.

### REQ-EXECUTORS-NOMADPOOL-003: Worktree workspaces

**Intent:** Workspaces behave like today's executors: a worktree on the Kandev
host, a fresh clone on other machines.

#### Acceptance criteria

- **AC-EXECUTORS-NOMADPOOL-003.1:** When an environment is placed on the local
  node, its workspace shall be a git worktree of Kandev's own repository checkout
  on a per-environment branch.
- **AC-EXECUTORS-NOMADPOOL-003.2:** When an environment is placed on a remote
  node, its workspace shall be a fresh per-environment checkout that fetches only
  the selected base branch, as the SSH executor does today.
- **AC-EXECUTORS-NOMADPOOL-003.3:** When an environment resumes after its
  allocation ended, placement shall prefer its original node. If that node is
  unavailable or no longer holds the workspace, the system shall report the
  workspace unavailable and shall not silently recreate it elsewhere.

### REQ-EXECUTORS-NOMADPOOL-004: Concurrent environments per node

**Intent:** Many idle sessions share a machine instead of reserving it whole.

#### Acceptance criteria

- **AC-EXECUTORS-NOMADPOOL-004.1:** The system shall allow more than one
  environment on a node; exclusivity shall be enforced per environment, not per
  worker.
- **AC-EXECUTORS-NOMADPOOL-004.2:** Pool profiles shall default to small CPU
  reservations and a memory reservation with a higher memory maximum, and shall
  state that on macOS native execution these values guide placement only.
- **AC-EXECUTORS-NOMADPOOL-004.3:** When two environments run on one node,
  stopping, recovering or cleaning up one shall not change the other's
  allocation, process tree, workspace or fence.

### REQ-EXECUTORS-NOMADPOOL-005: Per-node failure and quarantine

**Intent:** One bad machine does not take the pool down.

#### Acceptance criteria

- **AC-EXECUTORS-NOMADPOOL-005.1:** When cleanup on a node is uncertain, the
  system shall quarantine only that node, retain the uncertain environment's
  cleanup inventory, and keep placing new environments on other eligible nodes.
- **AC-EXECUTORS-NOMADPOOL-005.2:** When a node disconnects, its environments
  shall follow the native executor's disconnect contract within the profile's
  lost-after window, and environments on other nodes shall be unaffected.
- **AC-EXECUTORS-NOMADPOOL-005.3:** The system shall never create a replacement
  allocation for a lost environment on another node.

### REQ-EXECUTORS-NOMADPOOL-006: Scratch cleanup

**Intent:** Finished environments leave no agent junk on workers.

#### Acceptance criteria

- **AC-EXECUTORS-NOMADPOOL-006.1:** Agent temporary files shall be written
  inside the environment's Nomad allocation directory, so that proven job stop
  and Nomad garbage collection remove them.
- **AC-EXECUTORS-NOMADPOOL-006.2:** When cleanup is proven, the system shall
  remove the environment's worktree and prune it from its repository or mirror;
  when cleanup is uncertain, it shall retain the worktree with the quarantine
  inventory.

## Phase 2 (future work)

- **Optional agent sandbox wrapper.** A pool profile may set a launch wrapper
  (for example the nono sandbox) using Kandev's existing launch-wrapper
  mechanism; nodes declare installed wrappers in metadata, a required wrapper
  restricts placement to those nodes, and a missing wrapper fails closed. It is
  additional confinement, not hard isolation. Deferred until the pool itself is
  proven.
- **Per-node repository mirror** for remote workspaces, if fresh checkouts prove
  too slow.

## Implementation plans

- [Nomad worker pool](../../../plans/nomad-worker-pool/plan.md)

## Out of scope

- Load-aware placement using live CPU or memory.
- Recreating a workspace on another node during resume.
- Autoscaling or provisioning machines.
- Hard isolation of hostile native workloads and macOS resource enforcement.
- Linux exec with cgroups (a later capability profile).

## Decisions

- The Kandev host is included in placement by default with reserved headroom;
  `exclude_local` and `prefer_local` are profile options. (Owner, 2026-10-25.)
- Remote workspaces are fresh per-environment checkouts, as the SSH executor
  does today; a per-node mirror is a later optimization. (Owner, 2026-10-25.)
- The optional sandbox wrapper (for example nono) is phase 2, after the pool is
  proven. (Owner, 2026-10-25.)
