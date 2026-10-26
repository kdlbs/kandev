# Contribution notes: Nomad executor and worker pool

Branch `feature/nomad-native-executor`.

## What it contains

- Native Nomad executor completion (work orders 05 to 07): readiness, disconnect and
  cleanup-negative behaviour, operator readmission.
- Worker pool (plan `docs/plans/nomad-worker-pool/`): placement by node pool (spread or
  binpack), node contract metadata, loopback and ssh transports, local git worktrees and
  remote checkouts, per-environment fences, scratch in the allocation directory, per-node
  quarantine.
- Two product fixes found by the live proof: allocation directory removal after proven
  cleanup, and a bounded wait in agentctl instance creation while the reserved port is still
  held.
- Self-creating runner `scripts/nomad_native_e2e*.py` with the pool scenarios, and its
  offline tests.

## What the evidence proves

Six pool scenarios passed on one build on a two-client laptop fixture
(`docs/evidence/nomad-pool-p3/README.md`). It does not prove a multi-machine fleet, a real
network partition (the partition scenario freezes the ssh client agent), Linux cgroup
enforcement, or startup timing.

## Open items for review

- Refused launches quarantine the node by contract (`runtime_identity_unrecorded`); kept by
  owner decision.
- The agentctl port wait rests on the reading that the previous holder of the single
  reserved port is still shutting down; the live rerun passed with it, the port holder was
  not captured.
- Phase 2 (sandbox wrapper, per-node mirror) is out of scope.

## Verification commands

- Runner offline suites: `cd scripts && python3 -m unittest discover -p 'test_nomad_native_e2e*.py'`
- Backend: `make -C apps/backend test`; lint with `bash scripts/lint-go-changed <files>`
- Live: `python3 scripts/nomad_native_e2e.py create-fixture --scenario <name> --config <config>`
