---
id: "01-pool-placement-transport"
title: "Pool placement and transport"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NOMADPOOL-001
  - REQ-EXECUTORS-NOMADPOOL-002
acceptance_criteria:
  - AC-EXECUTORS-NOMADPOOL-001.1
  - AC-EXECUTORS-NOMADPOOL-001.2
  - AC-EXECUTORS-NOMADPOOL-001.3
  - AC-EXECUTORS-NOMADPOOL-001.4
  - AC-EXECUTORS-NOMADPOOL-001.5
  - AC-EXECUTORS-NOMADPOOL-002.1
  - AC-EXECUTORS-NOMADPOOL-002.2
  - AC-EXECUTORS-NOMADPOOL-002.3
  - AC-EXECUTORS-NOMADPOOL-002.4
system_design:
  - ../../specs/executors/system-design/nomad-worker-pool.md
---

# Task 01: Pool placement and transport

## Summary

Let the Nomad server choose the worker and let Kandev reach whichever node it chose.

## In scope

- Profile fields: `node_pool`, `placement`, `pin_node_id`, `exclude_local`, `prefer_local`; host, port and host key leave the profile.
- Node metadata contract (`kandev.attach`, `kandev.state_root`, SSH keys for remote, `kandev.host_identity_sha256` for local) and an operator onboarding script.
- Job specification: node-pool and metadata-presence constraints, scheduler algorithm, optional pin, local exclusion or affinity.
- Read the single allocation's NodeID into durable identity before any transport step; recovery verifies it.
- Loopback transport for local nodes after host-identity token verification; SSH transport with host, port and pinned key from the placed node's metadata, re-verified on recovery.
- Local Nomad client reserved headroom documented and checked in preflight.

## Out of scope

- Workspace strategy changes, per-environment fences and quarantine (task 02).

## Acceptance

- A pool profile places without naming a node, a pin reproduces native-executor behavior, unprepared nodes are never eligible, and both transports attach through one code path.
- A fake local node and a mismatched remote host key both fail closed.

## Verification

Focused Go tests for profile validation, job specification constraints, metadata validation, NodeID identity and both transports; runner offline tests for the two-client fixture builder.

## Files likely touched

- `apps/backend/internal/agent/nomad/` (profile, job, state)
- `apps/backend/internal/agent/runtime/lifecycle/executor_nomad*.go`
- `scripts/nomad_native_e2e*.py`

## Dependencies

Native executor work orders 01-07 done.

## Risks

Recovery must treat a metadata change between launch and recovery as fail-closed.

## Parallelism

None; task 02 builds on the placed-node identity.

## Inputs

- [Requirements](../../specs/executors/requirements/nomad-worker-pool.md)
- [System design](../../specs/executors/system-design/nomad-worker-pool.md)

## Results

Complete (2026-10-26). Real-worker proof in `docs/evidence/nomad-pool-p1/`: run 3 passed `pool-placement` on build `e7515aca9f` after two runs that each found a defect.

- Delivered: pool profile and node contract (94983e4851), pool job without a node name (4bc6f5ac79), preflight of node pool, scheduler algorithm and memory oversubscription (13a3f8efb2), pool launch, recovery and cleanup over one SSH or local transport with local-node verification (f8a5ac0a64).
- Runner: `pool-placement` create-fixture scenario (`scripts/nomad_native_e2e_pool.py`) uses the single-client fixture with a pool profile; it proves the job names no node, the allocation sits on a node carrying the Kandev contract, durable state is pool-placed, then runs the cleanup-positive assertions. The two-client fixture is deferred to task 03, which needs two nodes.
- Operator onboarding: `scripts/nomad-pool-onboard`; public docs in `docs/public/executors.md`.
- Live run 1 and 2 (operator, b14dcd28b5): preflight correctly refused disabled memory oversubscription; run 2 placed, attached and started the agent but the runner looked for the fence and processes at legacy paths. Runner fix: the pool fence path derives from the node's `kandev.state_root`, and cleanup-fixture releases a fence that names the recorded job even when the run failed before recording it.
- Resolved by run 3: Nomad interpolates `${meta.kandev.state_root}` in task env and args on the placed node.
