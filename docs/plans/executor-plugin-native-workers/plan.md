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
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-native-workers.md
  - ../../specs/executors/system-design/remote-executor-plugin-capability-versioning.md
  - ../../specs/executors/system-design/remote-executor-plugin-shared-admission-placement.md
  - ../../specs/executors/system-design/remote-executor-plugin-operations-migration.md
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
preparation (M6), migration (M8), and the fixture proof. A provider that declares nothing keeps today's behavior.

Proposed upstream; branch from `origin/main`. The native executor on the contributor fork is a read-only reference.

## Work orders

| Order | Work order | Wave | Depends on | Size |
| --- | --- | --- | --- | --- |
| 00 | [Record the owner decisions as an ADR](task-00-record-owner-decisions.md) | 1 | - | S: ~150 doc lines, 0 code. |
| 01 | [Capability registry, negotiation, and versioning (M7)](task-01-capability-registry-versioning.md) | 1 | 00 | M: ~550 source, ~900 test lines. |
| 02 | [Workspace descriptor (R1)](task-02-workspace-descriptor.md) | 2 | 01 | M: ~350 source, ~600 test lines. |
| 03 | [Destroy- and recover-scoped checkpoints (R4)](task-03-destroy-recover-checkpoints.md) | 2 | 02 | M: ~250 source, ~500 test lines. |
| 04 | [Drain receipt into destroy (R3)](task-04-drain-receipt.md) | 3 | 03 | M: ~400 source, ~600 test lines. Port from fork/feature/nomad-native-executor (read-only). |
| 05 | [Durable bootstrap continuation and ready publication (R5)](task-05-durable-bootstrap-publication.md) | 3 | 04 | L: ~500 source, ~900 test lines. |
| 06 | [Negotiated transport and API forwarding (R2)](task-06-host-tunnel-api-forwarding.md) | 4 | 05 | L: ~600 source, ~900 test lines. |
| 07 | [Repository intent and provider-prepared workspace (R7)](task-07-repository-intent.md) | 4 | 06 | M: ~300 source, ~500 test lines. |
| 08 | [Provider-neutral crash barrier (R8)](task-08-crash-barrier.md) | 5 | 07 | S: ~150 source, ~300 test lines. |
| 09 | [Workspace reuse and shared environments (M1)](task-09-shared-environments.md) | 6 | 08 | L: ~500 source, ~900 test lines. |
| 10 | [Launch and resume admission context (M2)](task-10-admission-context.md) | 6 | 09 | L: ~500 source, ~900 test lines. |
| 11 | [Queued and pending placement (M3)](task-11-pending-placement.md) | 7 | 10 | L: ~600 source, ~1000 test lines, with web and Playwright. |
| 12 | [Provider status, inventory, events, logs, and maintenance (M4)](task-12-provider-operations-maintenance.md) | 8 | 11 | L: ~650 source, ~1000 test lines. |
| 13 | [Launch-wrapper and sandbox requirement (M5)](task-13-launch-policy.md) | 8 | 12 | M: ~350 source, ~600 test lines. |
| 14 | [Extensible workspace preparation (M6)](task-14-preparation-hints.md) | 8 | 13, 07 | M: ~250 source, ~450 test lines. |
| 15 | [Migration and coexistence (M8)](task-15-migration-coexistence.md) | 9 | 14 | L: ~600 source, ~900 test lines. |
| 16 | [Fixture provider proving every capability (R8, M1-M8)](task-16-fixture-provider-proof.md) | 10 | 15 | M: ~450 source, ~700 test lines. |

Total forecast: about 7.2k source and 11.6k test lines, seventeen work orders. Task 00 records the owner decisions and the
answers to the medium-term open questions. Process-level interleaving tests in tasks 03 to 05 and 09 to 15 use fakes first and
gain their barrier versions in task 08. Each capability work order extends the fixture for its own capability, test-first;
task 16 is the cross-capability conformance run and the public documentation pass.

## Scope

### In scope

The three new requirement documents and the native worker requirements: REQ-EXECUTORS-NATIVE-WORKER-001 to 008,
REQ-EXECUTORS-PLUGIN-CAPABILITY-001 to 004, SHARED-001, ADMISSION-001, PLACEMENT-001, OPS-001, LAUNCHPOLICY-001,
PREPARATION-001, and MIGRATION-001, with their acceptance criteria.

### Out of scope

Built-in removal, autoscaling or provisioning machines, per-person quotas, fleet-management UI beyond the status read,
hostile-workload isolation, a concrete per-node mirror, public relay, SDK module publication. See the requirements.

## Risks

- Cross-process lifecycle changes (R2, R5, M3): the handshake-consumed window (AC-EXECUTORS-NATIVE-WORKER-007.5) is a rare, explicit cleanup, not a recovery.
- Both SQL dialects: every new SQL path (checkpoints, publication, owner sets, admission reservations, migration steps) needs the advisory-lock pattern and a gated PostgreSQL test.
- Proto additions must stay additive; run the SDK round-trip tests in task 01 and keep contract version 1.
- The 32 KiB envelope holds owners, placement, capability snapshot, repositories, and provider state together; task 01 adds a combined-size test.
- Size: the forecast is far above the original estimate line recorded on the board card; the work orders are separable, and task 16 carries the proof.
- Public docs (`plugins-authoring.md`, `plugins-manifest.md`) change in task 16 through `/docs-maintainer`.

## Verification strategy

Each work order lists exact commands. Task 11 adds a user-visible status and needs Playwright evidence; the rest has no UI
surface, and task 16 provides process-boundary evidence through the installed fixture plugin.
