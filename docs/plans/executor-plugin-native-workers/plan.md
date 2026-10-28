---
created: 2026-10-28
status: draft
requirements:
  - REQ-EXECUTORS-NATIVE-WORKER-001
  - REQ-EXECUTORS-NATIVE-WORKER-002
  - REQ-EXECUTORS-NATIVE-WORKER-003
  - REQ-EXECUTORS-NATIVE-WORKER-004
  - REQ-EXECUTORS-NATIVE-WORKER-005
  - REQ-EXECUTORS-NATIVE-WORKER-006
  - REQ-EXECUTORS-NATIVE-WORKER-007
  - REQ-EXECUTORS-NATIVE-WORKER-008
  - REQ-EXECUTORS-PLUGIN-CAPABILITY-001
  - REQ-EXECUTORS-PLUGIN-CAPABILITY-002
  - REQ-EXECUTORS-PLUGIN-CAPABILITY-003
  - REQ-EXECUTORS-PLUGIN-CAPABILITY-004
  - REQ-EXECUTORS-PLUGIN-SHARED-001
  - REQ-EXECUTORS-PLUGIN-ADMISSION-001
  - REQ-EXECUTORS-PLUGIN-PLACEMENT-001
  - REQ-EXECUTORS-PLUGIN-OPS-001
  - REQ-EXECUTORS-PLUGIN-LAUNCHPOLICY-001
  - REQ-EXECUTORS-PLUGIN-PREPARATION-001
  - REQ-EXECUTORS-PLUGIN-MIGRATION-001
  - REQ-EXECUTORS-PLUGIN-PLACEMENT-002
  - REQ-EXECUTORS-PLUGIN-PROFILE-001
  - REQ-EXECUTORS-PLUGIN-PROFILE-002
  - REQ-EXECUTORS-PLUGIN-PROFILE-003
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-native-workers.md
  - ../../specs/executors/system-design/remote-executor-plugin-capability-versioning.md
  - ../../specs/executors/system-design/remote-executor-plugin-shared-admission-placement.md
  - ../../specs/executors/system-design/remote-executor-plugin-operations-migration.md
  - ../../specs/executors/system-design/remote-executor-plugin-profile-contract.md
legacy_specs: []
---

# Implementation plan: Native worker capabilities for executor plugins

## Overview

Add the generic, opt-in host capabilities a worker-pool provider needs, proven by the fixture provider without any
vendor, so the plugin is not blocked by a second contract change. Deliver test-first. Capability negotiation and
versioning (M7) comes first because every other capability is a registry entry. Then the first release in the order
workspace descriptor (R1), destroy and recover checkpoints (R4), drain receipt (R3), durable bootstrap and ready publication
(R5), transport (R2), repository intent (R7), crash barrier (R8). Then the medium-term items: shared environments (M1),
admission context (M2), pending placement (M3), provider operations and maintenance (M4), launch policy (M5), extensible
preparation (M6), migration (M8), and the fixture proof. Task 17 (profile schema, typed values, secret references) closes the plugin skeleton findings 1 to 3 and can proceed beside tasks 02 to 08. A provider that declares nothing keeps today's behavior.

Proposed upstream; branch from `origin/main`. The native executor on the contributor fork is a read-only reference.

## Work orders

| Order | Work order | Wave | Depends on | Size |
| --- | --- | --- | --- | --- |
| 00 | [Record the owner decisions as an ADR](task-00-record-owner-decisions.md) | 1 | - | S: ~120 doc lines, 0 code. |
| 01 | [Capability registry, negotiation, and versioning (M7)](task-01-capability-registry-versioning.md) | 1 | 00 | M: ~650 source, ~1000 test lines. |
| 17 | [Profile schema keywords, typed values, and secret references](task-17-profile-contract.md) | 2 | 01 | M: ~450 source, ~800 test lines. Runs beside 02 to 08. |
| 02 | [Workspace descriptor (R1)](task-02-workspace-descriptor.md) | 2 | 01 | M: ~450 source, ~700 test lines. |
| 03 | [Destroy- and recover-scoped checkpoints (R4)](task-03-destroy-recover-checkpoints.md) | 2 | 02 | M: ~250 source, ~500 test lines. |
| 04 | [Drain receipt into destroy (R3)](task-04-drain-receipt.md) | 3 | 03 | M: ~400 source, ~600 test lines. Port from the fork reference (see Reference). |
| 05 | [Durable bootstrap continuation and ready publication (R5)](task-05-durable-bootstrap-publication.md) | 3 | 04 | L: ~500 source, ~900 test lines. |
| 06 | [Negotiated transport and API forwarding (R2)](task-06-host-tunnel-api-forwarding.md) | 4 | 05 | L: ~600 source, ~900 test lines. |
| 07 | [Repository intent and provider-prepared workspace (R7)](task-07-repository-intent.md) | 4 | 06 | M: ~300 source, ~500 test lines. |
| 08 | [Provider-neutral crash barrier (R8)](task-08-crash-barrier.md) | 5 | 07 | S: ~150 source, ~300 test lines. |
| 09 | [Workspace reuse and shared environments (M1)](task-09-shared-environments.md) | 6 | 08 | L: ~500 source, ~900 test lines. |
| 10 | [Launch and resume admission context (M2)](task-10-admission-context.md) | 6 | 09 | L: ~500 source, ~900 test lines. |
| 11 | [Queued and pending placement and refused provision (M3)](task-11-pending-placement.md) | 7 | 10 | L: ~600 source, ~1000 test lines, with web and Playwright. |
| 12 | [Provider status, inventory, events, logs, and maintenance (M4)](task-12-provider-operations-maintenance.md) | 8 | 11 | L: ~650 source, ~1000 test lines. |
| 13 | [Launch-wrapper and sandbox requirement (M5)](task-13-launch-policy.md) | 8 | 12 | M: ~350 source, ~600 test lines. |
| 14 | [Extensible workspace preparation (M6)](task-14-preparation-hints.md) | 8 | 13, 07 | M: ~250 source, ~450 test lines. |
| 15 | [Migration and coexistence (M8)](task-15-migration-coexistence.md) | 9 | 14 | L: ~600 source, ~900 test lines. |
| 16 | [Fixture provider proving every capability (R8, M1-M8)](task-16-fixture-provider-proof.md) | 10 | 15, 17 | M: ~300 source, ~650 test lines, with one Playwright spec. |

Total forecast: about 7.5k source and 12.6k test lines, eighteen work orders (00 to 17). Task 00 records the owner decisions and the
answers to the medium-term open questions. Process-level interleaving tests in tasks 03 to 05 and 09 to 15 use fakes first and
gain their barrier versions in task 08. Each capability work order extends the fixture for its own capability, test-first;
task 16 is the cross-capability conformance run and the public documentation pass.

## Reference (read-only)

The native implementation lives on the contributor fork `nova28/kandev`, branch `feature/nomad-native-executor`, head `f0c3b0ab18` (fork PR 10). It is a source for ports (agentctl seal-and-drain, the launch-wrapper node-policy files `node_policy.go`, `node_policy_unix.go`, and `node_policy_other.go`) and is never merged. If it is unavailable, the work order writes the code from its design section. This is the only place in the plan that names it.

## Build step

The Build step keeps this one card and builds every work order in the order above (M7 first). Generic code comes from the closed upstream PR, preserved on the fork branch `feature/nomad-native-executor`, by cherry-pick: the agentctl drain, the crash barrier and guarded ready publication, the cleanup claim, and the local-worktree helpers only; nothing Nomad-specific. Record each cherry-picked hash and each deviation under "Build record" in this file, and give each new SQL path a SQLite and a PostgreSQL test.

## Release slices

Each slice is shippable alone, keeps contract version 1, and ends with its fixture additions and a conformance run, so a later slice never changes an earlier one. A requirement of an unshipped slice stays `draft`.

1. Capability and native worker core: 00, 01, 02, 03, 04, 05, 06, 07, 08, then 17 (profile contract, which needs only 01 and can ship with this slice).
2. Shared environments, admission, and pending placement: 09, 10, 11.
3. Operations, launch policy, and preparation hints: 12, 13, 14.
4. Migration and coexistence: 15.
5. Fixture proof and public documentation: 16 depends on 15 and 17, so it runs once after slices 1 to 4 for the cross-capability pass; each earlier slice ships with its own fixture additions and conformance run.

## Scope

### In scope

The six requirement documents (native workers, native worker recovery and proof, capability versioning, shared environments with admission and placement, operations and migration, and the profile contract): REQ-EXECUTORS-NATIVE-WORKER-001 to 008,
REQ-EXECUTORS-PLUGIN-CAPABILITY-001 to 004, SHARED-001, ADMISSION-001, PLACEMENT-001 and 002, PROFILE-001 to 003, OPS-001, LAUNCHPOLICY-001,
PREPARATION-001, and MIGRATION-001, with their acceptance criteria.

### Out of scope

Built-in removal, autoscaling or provisioning machines, per-person quotas, fleet-management UI beyond the status read,
hostile-workload isolation, a concrete per-node mirror, public relay, SDK module publication. See the requirements.

## Risks

- Cross-process lifecycle changes (R2, R5, M3): the handshake-consumed window (AC-EXECUTORS-NATIVE-WORKER-007.5) is a rare, explicit cleanup, not a recovery.
- Both SQL dialects: every new SQL path (checkpoints, publication, owner sets, admission reservations, migration steps) needs the advisory-lock pattern and a gated PostgreSQL test.
- Proto additions must stay additive; run the SDK round-trip tests in task 01 and keep contract version 1.
- The 32 KiB envelope holds owners, placement, capability snapshot, repositories, and provider state together; the per-field maxima (host reserve 20480, state at most 12288) are in the versioning design, and task 01 adds the combined worst-case test.
- Size: the forecast is far above the original estimate line recorded on the board card; the work orders are separable, and task 16 carries the proof.
- The plugin skeleton findings are mapped one by one in the profile-contract design ("Resolution of the plugin skeleton findings").
- Public docs (`plugins-authoring.md`, `plugins-manifest.md`) change in task 16 through `/docs-maintainer`.

## Verification strategy

Each work order lists exact commands, run from the repository root with `(cd apps/backend && ...)` because the only Go module is `apps/backend`. Task 11 adds a user-visible status and needs Playwright evidence; task 16 adds one parametrized Playwright spec for the remaining host-originated failure copy; the rest has no other UI
surface, and task 16 provides process-boundary evidence through the installed fixture plugin.
