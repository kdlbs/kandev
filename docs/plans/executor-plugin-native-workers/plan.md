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
  - REQ-EXECUTORS-PLUGIN-ADMISSION-001
  - REQ-EXECUTORS-PLUGIN-PLACEMENT-001
  - REQ-EXECUTORS-PLUGIN-OPS-001
  - REQ-EXECUTORS-PLUGIN-LAUNCHPOLICY-001
  - REQ-EXECUTORS-PLUGIN-PREPARATION-001
  - REQ-EXECUTORS-PLUGIN-PLACEMENT-002
  - REQ-EXECUTORS-PLUGIN-PROFILE-001
  - REQ-EXECUTORS-PLUGIN-PROFILE-002
  - REQ-EXECUTORS-PLUGIN-PROFILE-003
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-native-workers.md
  - ../../specs/executors/system-design/remote-executor-plugin-capability-versioning.md
  - ../../specs/executors/system-design/remote-executor-plugin-admission.md
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
(R5), transport (R2), repository intent (R7), crash barrier (R8). Then the medium-term items: admission context (M2), pending placement (M3), provider operations and maintenance (M4), launch policy (M5), extensible
preparation (M6), and the fixture proof. Task 17 (profile schema, typed values, secret references) closes the plugin skeleton findings 1 to 3 and can proceed beside tasks 02 to 08. A provider that declares nothing keeps today's behavior.

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
| 10 | [Launch and resume admission context (M2)](task-10-admission-context.md) | 6 | 08 | L: ~500 source, ~900 test lines. |
| 11 | [Queued and pending placement and refused provision (M3)](task-11-pending-placement.md) | 7 | 10 | L: ~600 source, ~1000 test lines, with web and Playwright. |
| 12 | [Provider status, inventory, events, logs, and maintenance (M4)](task-12-provider-operations-maintenance.md) | 8 | 11 | L: ~650 source, ~1000 test lines. |
| 13 | [Launch-wrapper and sandbox requirement (M5)](task-13-launch-policy.md) | 8 | 12 | M: ~350 source, ~600 test lines. |
| 14 | [Extensible workspace preparation (M6)](task-14-preparation-hints.md) | 8 | 13, 07 | M: ~250 source, ~450 test lines. |
| 16 | [Fixture provider proving every capability (R8, M2-M7)](task-16-fixture-provider-proof.md) | 9 | 14, 17 | M: ~300 source, ~650 test lines, with one Playwright spec. |

Total forecast: about 7k source and 11.7k test lines, sixteen work orders (00 to 08, 10 to 14, 16, and 17; 09 was shared environments and 15 was profile migration, both cut, see Out of scope). Task 00 records the owner decisions and the
answers to the medium-term open questions. Process-level interleaving tests in tasks 03 to 05 and 10 to 14 use fakes first and
gain their barrier versions in task 08. Each capability work order extends the fixture for its own capability, test-first;
task 16 is the cross-capability conformance run and the public documentation pass.

## Reference (read-only)

The native implementation lives on the contributor fork `nova28/kandev`, branch `feature/nomad-native-executor`, head `f0c3b0ab18` (fork PR 10). It is a source for ports (agentctl seal-and-drain, the launch-wrapper node-policy files `node_policy.go`, `node_policy_unix.go`, and `node_policy_other.go`) and is never merged. If it is unavailable, the work order writes the code from its design section. This is the only place in the plan that names it.

## Build step

The Build step keeps this one card and builds every work order in the order above (M7 first). Generic code comes from the closed upstream PR, preserved on the fork branch `feature/nomad-native-executor`, by cherry-pick: the agentctl drain, the crash barrier and guarded ready publication, the cleanup claim, and the local-worktree helpers only; nothing Nomad-specific. Record each cherry-picked hash and each deviation under "Build record" in this file, and give each new SQL path a SQLite and a PostgreSQL test.

## Release slices

Each slice is shippable alone, keeps contract version 1, and ends with its fixture additions and a conformance run, so a later slice never changes an earlier one. A requirement of an unshipped slice stays `draft`.

1. Capability and native worker core: 00, 01, 02, 03, 04, 05, 06, 07, 08, then 17 (profile contract, which needs only 01 and can ship with this slice).
2. Admission and pending placement: 10, 11.
3. Operations, launch policy, and preparation hints: 12, 13, 14.
4. Fixture proof and public documentation: 16 depends on 14 and 17, so it runs once after slices 1 to 3 for the cross-capability pass; each earlier slice ships with its own fixture additions and conformance run.

**Slice freeze (owner decision, 2026-10-28, round 7).** If Spec Review round 8 is not clean, whole-package review stops. Slice 1 (capability registry, R1 to R8, and the profile contract) freezes for Build, and Spec Review then runs per later slice just before that slice is built. Resource classes, labels and provider-reported admission values are cut to follow-up card 11c92feb-2b4c-4d05-8318-9aff24e2fa10 (admission keeps its hook seam with a default allow-all).

## Scope

### In scope

The six requirement documents (native workers, native worker recovery and proof, capability versioning, admission and placement, operations and migration, and the profile contract): REQ-EXECUTORS-NATIVE-WORKER-001 to 008,
REQ-EXECUTORS-PLUGIN-CAPABILITY-001 to 004, ADMISSION-001, PLACEMENT-001 and 002, PROFILE-001 to 003, OPS-001, LAUNCHPOLICY-001,
and PREPARATION-001, with their acceptance criteria.

### Out of scope

Profile migration from a built-in executor to a plugin (M8, formerly MIGRATION-001 and work order 15; cut by the owner on 2026-10-28; follow-up card a8546b99-a348-461e-b39f-23993e118b73, which also carries the deferred review findings L5, L9, L10, L11 and the migration part of L3), shared environments and workspace reuse (M1, formerly SHARED-001 and work order 09; cut by the owner on 2026-10-28, deferring it means a second contract change later; follow-up card 99beef6d-530e-4afd-949b-1424fc9245df, which must decide the reuse candidate set against `WorkspaceReuseRequired` on main first), built-in removal, autoscaling or provisioning machines, per-person quotas, fleet-management UI beyond the status read,
hostile-workload isolation, a concrete per-node mirror, public relay, SDK module publication. See the requirements.

## Risks

- Cross-process lifecycle changes (R2, R5, M3): the handshake-consumed window (AC-EXECUTORS-NATIVE-WORKER-007.5) is a rare, explicit cleanup, not a recovery.
- Both SQL dialects: every new SQL path (checkpoints, publication, admission reservations) needs the advisory-lock pattern and a gated PostgreSQL test.
- Proto additions must stay additive; run the SDK round-trip tests in task 01 and keep contract version 1.
- The 32 KiB envelope holds placement, capability snapshot, repositories, and provider state together; the per-field maxima (host reserve 20480, state at most 12288) are in the versioning design, and task 01 adds the combined worst-case test.
- Size: the forecast is far above the original estimate line recorded on the board card; the work orders are separable, and task 16 carries the proof.
- The plugin skeleton findings are mapped one by one in the profile-contract design ("Resolution of the plugin skeleton findings").
- Public docs (`plugins-authoring.md`, `plugins-manifest.md`) change in task 16 through `/docs-maintainer`.

## Verification strategy

Each work order lists exact commands, run from the repository root with `(cd apps/backend && ...)` because the only Go module is `apps/backend`. Task 11 adds a user-visible status and needs Playwright evidence; task 16 adds one parametrized Playwright spec for the remaining host-originated failure copy; the rest has no other UI
surface, and task 16 provides process-boundary evidence through the installed fixture plugin.

## Build record

Slice 1 only, on top of c76077772b. The fork branch holds one mixed commit (478831ab84), so generic code is ported by explicit path and adapted; no cherry-pick hash applies.

| Commit | Work order | Content |
| --- | --- | --- |
| b97f9e8f4b | 00 | ADR recording D1, D2, D3, D6 |
| d5ba501d0d, e8e5d9690b, 1bc9ea87eb, 921ed172ef | 01 | Capability registry, manifest syntax, echo gate, allocation gate, envelope budget, readability lock |
| a328d60fd3 | 17 | Profile schema keywords, typed values, secret references, profile budget |
| c50f3a8c34 | 02 | Workspace descriptor, immutability and per-installation uniqueness |
| 2350fdaa32 | 03 | Claim-scoped destroy and recover checkpoints with sequence fencing |
| 657224d1e4 | 04 | Runtime seal and drain, `drain_receipt`, `destroy_timeout_seconds`, idle-reaper bootstrap field |
| 6c550b7070, f4532a54f9 | 05 | Guarded ready publication under the recovery claim; `durable_bootstrap` resume with repeated handshake, `bootstrap_consumed` cleanup |
| 439a988a13 | 06 (API forwarding half) | `runtime_api_url`, public ping route, agentctl `probe-host-api`, forwarded-launch probe with `api_unreachable` rollback |

### Deviations
- WO 03, AC 006.8: envelope claim state is first written by the first checkpoint's transaction (pending owner review).
- WO 03: settlement requires at least one checkpoint stored for the claim, not the latest one (pending owner review).
- WO 04: the receipt message `ExecutorDrainReceipt` and the extra envelope field `drain_claim` are assumptions. The runtime identity is bound by the authenticated control token plus the freshly read server identity. Only a refused connection and 401/403/409 classify as `unreachable`; every other failure is `uncertain`. The `after_drain` crash barrier belongs to WO 08. The provider maps `idle_timeout_seconds: 0` to `KANDEV_ACP_IDLE_TIMEOUT=0`. The fork's Nomad port-busy wait is not ported.
- WO 05: recovery creates a minimal agentctl instance when none exists and adopts an existing one otherwise; the nonce secret is deleted at ready or settle and both runtime secrets at record removal; a cleanup that loses the claim race records `cleanup_requested` by CAS and runs after the claim is released; ready publication is an optional-interface assertion on the inventory store and fails closed when absent; durable recovery makes no provider Inspect or Attach call; the lifecycle passes nil repositories, which the repository refuses for a task with repositories, so such a launch stays retained until WO 07.
- WO 06 (API forwarding): the probe route is `POST /runtime/probe-host-api` on the agentctl control server; the 10 s budget spans both attempts; a forwarded launch sends `host_api_url` and omits `api_url`; the host API URL is the backend's loopback listener; the other-environment token case is handled by token lookup because the request carries no environment ID. A lost reply counts as success when the host saw its token. Pre-existing unrelated failures in `internal/agentctl/server/api` git-status tests are proven on the merge-base in the final gauntlet.
- PostgreSQL tests for WO 01 to 03 and 05 are written and env-gated but not run locally (no PostgreSQL or Docker).
