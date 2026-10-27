---
id: "03-pool-proof-handoff"
title: "Two-node proof and handoff"
status: done
wave: 3
depends_on:
  - "02-pool-workspaces-fences"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NOMADPOOL-001
  - REQ-EXECUTORS-NOMADPOOL-002
  - REQ-EXECUTORS-NOMADPOOL-003
  - REQ-EXECUTORS-NOMADPOOL-004
  - REQ-EXECUTORS-NOMADPOOL-005
  - REQ-EXECUTORS-NOMADPOOL-006
acceptance_criteria:
  - AC-EXECUTORS-NOMADPOOL-001.3
  - AC-EXECUTORS-NOMADPOOL-001.4
  - AC-EXECUTORS-NOMADPOOL-002.2
  - AC-EXECUTORS-NOMADPOOL-002.3
  - AC-EXECUTORS-NOMADPOOL-004.3
  - AC-EXECUTORS-NOMADPOOL-005.1
  - AC-EXECUTORS-NOMADPOOL-005.2
  - AC-EXECUTORS-NOMADPOOL-006.1
  - AC-EXECUTORS-NOMADPOOL-006.2
system_design:
  - ../../specs/executors/system-design/nomad-worker-pool.md
  - ../../specs/executors/system-design/nomad-native-executor.md
---

# Task 03: Two-node proof and handoff

## Summary

Prove the pool live on two local Nomad clients, prove the ordinary built runtime, and prepare one cohesive contribution. This absorbs the native executor's cancelled work order 08.

## In scope

- Two-client fixture in the self-creating runner: one `local`, one `ssh` behind a fixture sshd, one throwaway server advertising the relay address for partition tests.
- Live scenarios: spread placement, queue at capacity then place on release, single-node partition with the other node untouched and no replacement, proven cleanup of worktree and scratch, per-node quarantine with continued placement, refusal of a wrong host key and a fake local node.
- Built-runtime proof with a fresh data directory and profile, startup timing, install and onboarding documentation (native executor work order 08 scope).
- Public docs and scoped AGENTS updates where architecture changed; contribution notes. Publishing stays with the owner.

## Out of scope

- Phase-2 items; production rollout.

## Acceptance

- Every live scenario passes on a normally built binary with evidence in `docs/evidence/`; zero or skipped scenarios fail the work order.
- The contribution is ready for review and claims only what the evidence proves.

## Verification

Runner live commands per scenario with owner approval, plus the native executor's full offline suites and documentation checks.

## Files likely touched

- `scripts/nomad_native_e2e*.py`
- `docs/evidence/`, `docs/public/`

## Dependencies

Task 02.

## Risks

Local two-client fixture differs from a real multi-machine fleet; record that boundary honestly.

## Parallelism

None.

## Inputs

- [Native executor work order 08 (cancelled)](../nomad-native-executor/task-08-package-handoff.md)

## Results

Complete (2026-10-26). All six pool scenarios (`pool-placement`, `pool-two-node`, `pool-node-quarantine`, `pool-queue`, `pool-refusals`, `pool-partition`) passed rc 0 on one build (`0d8ba484e9`, v0.98.0-64) on the laptop two-node fixture; evidence, run ids, defects found per round and the fixture boundary are in `docs/evidence/nomad-pool-p3/README.md`. Contribution notes: `docs/evidence/nomad-pool-p3/contribution-notes.md` (not published). Not covered: startup timing, a multi-machine fleet and a real network partition.

### Closeout (2026-10-28)

The later coverage round has 14 default-mode laptop-fixture passes reported by the operator;
raw live JSON was not committed. The latest tested code candidate is `1222b56224`.
The full backend suite exits 2 with 262 failed top-level tests and one interrupted test.
Exact-name reruns on the candidate, clean previous-card tip `73c8155caf`, and clean upstream
merge base `baa92e5dcf` reproduce the same 262 failed roots (452 identities with subtests);
the interrupted test passes on all three. No failed-name difference, skip or missing test
remains in that selection. Current focused Go and offline Python receipts and the exact
comparison command/names are recorded in
[closeout evidence](../../evidence/nomad-e2e-coverage/closeout.md).

The contribution remains a draft. TLS/scoped-ACL mode has not run live; fix 7 has Go tests
only; native post-restart reattachment has not proved an actual agent-to-Kandev tool call.
[Remaining fixture instructions](../../evidence/nomad-e2e-coverage/remaining-fixtures.md)
describe the supported commands and the missing harness extensions. Actual Tank/Neo pool
evaluation remains a separate approved window.
