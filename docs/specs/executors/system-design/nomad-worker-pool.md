---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-NOMADPOOL-001
  - REQ-EXECUTORS-NOMADPOOL-002
  - REQ-EXECUTORS-NOMADPOOL-003
  - REQ-EXECUTORS-NOMADPOOL-004
  - REQ-EXECUTORS-NOMADPOOL-005
  - REQ-EXECUTORS-NOMADPOOL-006
---

# Nomad worker pool system design

## Purpose and boundaries

Generalize the [native Nomad executor](nomad-native-executor.md) from one pinned
worker to a pool selected by the Nomad server. Nomad already knows every
client's resources and liveness and places allocations; Kandev must not
duplicate that. Kandev's new work is limited to: resolving how to reach the node
Nomad chose, building the workspace there, making exclusivity and quarantine
per environment and per node, and moving scratch into the allocation directory.
A pinned node is an optional placement constraint, so part-one behavior stays
available unchanged.

## Requirement mapping

| Requirement | Mechanism |
|---|---|
| REQ-EXECUTORS-NOMADPOOL-001 | Job targets `node_pool` plus a metadata-presence constraint; optional `pin_node_id` constraint; scheduler algorithm per profile; NodeID read from the allocation into durable identity |
| REQ-EXECUTORS-NOMADPOOL-002 | Node metadata selects loopback or SSH; local identity token check; per-node pinned host key from metadata |
| REQ-EXECUTORS-NOMADPOOL-003 | Local worktree from Kandev's checkout; remote fresh single-branch checkout as in the SSH executor; resume affinity to the original node |
| REQ-EXECUTORS-NOMADPOOL-004 | Per-environment start fence; small reservations with `memory_max`; isolation tests for two environments on one node |
| REQ-EXECUTORS-NOMADPOOL-005 | Per-node quarantine marker and eligibility; existing disconnect contract with profile `lost_after`; no replacement |
| REQ-EXECUTORS-NOMADPOOL-006 | `TMPDIR` and agent scratch under the allocation directory; worktree or checkout removal in proven cleanup |

## Responsibilities

| Concern | Nomad server | Kandev |
|---|---|---|
| Node inventory, free resources | Clients register and report | Reads only |
| Placement | Node pool, constraints, spread or binpack | Supplies the policy |
| Liveness, disconnect, lost | Heartbeats, `lost_after`, no replacement | Recovery and fail-closed reaction |
| Process lifecycle, allocation GC | Yes | Drain receipt before whole-job stop |
| Transport to agentctl | No | Loopback or SSH to the placed node |
| Workspace | No | Worktree on the Kandev host, fresh checkout elsewhere |
| Quarantine decision | Eligibility flag only | Decides, records marker per node |

## Profile

```
address, token ref, namespace, region       unchanged
node_pool          default "kandev"
placement          spread (default) | binpack
pin_node_id        optional; part-one behavior
exclude_local      default false
prefer_local       default false (affinity, not constraint)
cpu_mhz            small default (for example 100)
memory_mb          small default; memory_max_mb larger
lost_after         default 24h, bounded 1m-7d
ssh_user, ssh_identity   used only for remote nodes
```

Host, port and host key move out of the profile into node metadata.

## Node onboarding and metadata

Each worker runs a Nomad client in the profile's node pool and publishes:

```
kandev.attach            local | ssh
kandev.state_root        absolute path for fences, mirrors, worktrees
kandev.ssh_host          remote only
kandev.ssh_port          remote only
kandev.ssh_host_key_sha256   remote only
kandev.host_identity_sha256  local only
```

The job carries a constraint that `kandev.attach` and `kandev.state_root` are
set, so unprepared nodes are never eligible. Onboarding is an operator step,
documented with a script; Kandev never edits node configuration.

The local node's `kandev.host_identity_sha256` is the hash of a random token
written under Kandev's own data directory at onboarding. When an allocation
lands on a node declaring `local`, Kandev hashes its own token and compares. A
mismatch rejects the environment; there is no SSH fallback, because a remote
node claiming to be local would otherwise receive loopback trust.

## Placement and identity

The job specification drops the node-ID constraint unless `pin_node_id` is set,
adds the node-pool and metadata constraints, sets the scheduler algorithm, and
for resume adds a soft affinity to the recorded node. After registration Kandev
reads the single allocation and records its NodeID before any transport or
workspace step; the node becomes part of the immutable identity that part-one
recovery already verifies (job, allocation, node, fence, operation).

## Transport

- **Local:** agentctl binds loopback on the dynamic port Nomad reserved. Kandev
  connects directly with the same authenticated bootstrap handshake as part one,
  without the SSH hop.
- **Remote:** unchanged part-one SSH transport, except host, port and pinned key
  come from the placed node's metadata, read at attach time and re-verified on
  recovery. A metadata change between launch and recovery fails closed.

Both transports share one attach path; only the dial step differs.

## Workspace

- **Local:** a git worktree from Kandev's repository checkout under the local
  node's `kandev.state_root`, on a per-environment branch, as the Worktree
  executor does today.
- **Remote:** a fresh per-environment checkout under the node's state root:
  `git init`, add origin, fetch only the base branch, check out, as the SSH
  executor's prepare step does today. A per-node mirror is a later optimization.
- **Resume:** affinity to the recorded node. If the node is gone or the
  worktree is missing, report workspace unavailable. Re-creating elsewhere is
  out of scope.

## Exclusivity and capacity

Part one's worker-wide start fence becomes a fence per environment under
`<state_root>/environments/<environment-id>/active`, acquired by the bootstrap
before agentctl starts and released only by a fenced drain receipt. Capacity is
entirely Nomad's: the job declares small CPU, `memory_mb` and `memory_max_mb`;
memory oversubscription must be enabled in the scheduler configuration and is
checked during preflight. On macOS `raw_exec` none of these is enforced, which
the profile and diagnostics state plainly.

The local node's Nomad client declares `client.reserved` CPU and memory so
agent work cannot starve Kandev.

## Failure, quarantine and cleanup

Quarantine records a marker with the NodeID and sets only that node ineligible;
the pool keeps serving. Disconnect follows the part-one contract with the
profile's `lost_after`, `replace=false` and keep-original reconciliation.

Scratch: the bootstrap sets `TMPDIR` and agent temporary roots to the
allocation's local directory, so proven whole-job stop and Nomad allocation
garbage collection remove them. Proven cleanup also removes the environment
worktree and prunes the repository or mirror. Uncertain cleanup retains both
with the quarantine inventory.

## Alternatives considered

- **Per-node mirror plus worktrees for remote nodes:** faster starts and shared
  objects, but adds a mirror lifecycle (locking, pruning, corruption recovery).
  Deferred until fresh checkouts prove too slow.
- **SSH executor with a host list and a picker script:** simplest for one or
  two stable machines. It would reimplement placement, capacity queueing,
  lost-node handling and cleanup proof. Preferred only when the fleet is that
  small.
- **Kubernetes:** per-pod reservations hold capacity while sessions idle (the
  reference scenario's reason for leaving it), and it requires containers.
- **Kandev-side scheduler:** rejected; Nomad already owns placement and
  liveness.

## Verification

Offline: placement constraints and affinity in the job specification, metadata
validation, local identity mismatch, host-key mismatch, two-environment
isolation, per-node quarantine, scratch path assertions.

Live, on two local throwaway Nomad clients (one `local`, one `ssh` through a
fixture sshd), with the self-creating runner:

1. Spread places each environment on the eligible node with the lowest reserved-resource utilisation (binpack fills a node first; placement is reservation-based, not live-load-based), so two environments land on different nodes.
2. With one node full, a third environment queues, then places after release.
3. Partitioning one node leaves the other node's environments untouched; no
   replacement is created.
4. Proven stop removes worktree and scratch; an induced uncertain cleanup
   quarantines only that node and placement continues on the other.
5. A remote node with a wrong host key and a fake `local` node are both refused.

## Phase 2 (future work)

- **Optional sandbox wrapper.** Reuse Kandev's existing `launchwrapper` spec,
  which agentctl already applies to every agent launch path: the profile carries
  the wrapper (for example nono allowing only the environment's workspace and
  allocation directory), nodes declare `kandev.wrappers`, a required wrapper adds
  a placement constraint, and agentctl's `LaunchWrapperRequired` fails closed.
- **Per-node mirror plus worktrees** for remote nodes, if fresh checkouts are too
  slow.
