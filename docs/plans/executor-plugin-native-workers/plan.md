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
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-native-workers.md
legacy_specs: []
---

# Implementation plan: Native worker capabilities for executor plugins

## Overview

Add the generic, opt-in host capabilities a worker-pool provider needs, proven by the fixture provider without any
vendor. Deliver test-first in this order: workspace descriptor with negotiation (R1), destroy and recover
checkpoints (R4), drain receipt (R3), durable bootstrap and ready publication (R5), transport (R2), repository
intent (R7), crash barrier and fixture proof (R8). A provider that declares nothing keeps today's behavior.

Proposed upstream; branch from `origin/main`. The native executor on the contributor fork is a read-only reference.

## Work orders

| Order | Work order | Wave | Depends on | Size |
| --- | --- | --- | --- | --- |
| 00 | [Record the owner decisions as an ADR](task-00-record-owner-decisions.md) | 1 | - | S: ~120 doc lines, 0 code. |
| 01 | [Capability negotiation and workspace descriptor (R1)](task-01-capability-negotiation-workspace.md) | 1 | 00 | M: ~450 source, ~700 test lines. |
| 02 | [Destroy- and recover-scoped checkpoints (R4)](task-02-destroy-recover-checkpoints.md) | 2 | 01 | M: ~250 source, ~500 test lines. |
| 03 | [Drain receipt into destroy (R3)](task-03-drain-receipt.md) | 2 | 02 | M: ~400 source, ~600 test lines. Port from fork/feature/nomad-native-executor (read-only). |
| 04 | [Durable bootstrap continuation and ready publication (R5)](task-04-durable-bootstrap-publication.md) | 3 | 03 | L: ~500 source, ~900 test lines. |
| 05 | [Negotiated transport and API forwarding (R2)](task-05-host-tunnel-api-forwarding.md) | 3 | 04 | L: ~600 source, ~900 test lines. |
| 06 | [Repository intent and provider-prepared workspace (R7)](task-06-repository-intent.md) | 4 | 05 | M: ~300 source, ~500 test lines. |
| 07 | [Provider-neutral crash barrier (R8)](task-07-crash-barrier.md) | 4 | 06 | S: ~150 source, ~300 test lines. |
| 08 | [Fixture provider proving every capability (R8)](task-08-fixture-provider-proof.md) | 5 | 07 | M: ~250 source, ~400 test lines. |

Total forecast: about 2.2k source and 4.4k test lines, nine work orders. Task 00 waits for the owner answer on D1, D2, D3, D6.
Process-level interleaving tests in tasks 02 to 04 use fakes first and gain their barrier versions in task 07.

## Scope

### In scope

REQ-EXECUTORS-NATIVE-WORKER-001 to 008 and their acceptance criteria.

### Out of scope

Built-in removal, per-node mirror, fleet UI, hostile-workload isolation, provider maintenance operation, native-state migration,
public relay, SDK module publication. See the requirements.

## Risks

- Cross-process lifecycle changes (R2, R5): the handshake-consumed window (AC-EXECUTORS-NATIVE-WORKER-007.5) is a rare, explicit cleanup, not a recovery.
- Both SQL dialects: every new SQL path needs the advisory-lock pattern and a gated PostgreSQL test.
- Proto additions must stay additive; run the SDK round-trip tests in task 01.
- Public docs (`plugins-authoring.md`, `plugins-manifest.md`) change in task 08 through `/docs-maintainer`.

## Verification strategy

Each work order lists exact commands. A user-facing requirement needs end-to-end evidence: this package has no UI surface;
task 08 provides process-boundary evidence through the installed fixture plugin.
