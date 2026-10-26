---
created: 2026-10-24
status: in_progress
requirements:
  - REQ-EXECUTORS-NOMAD-001
  - REQ-EXECUTORS-NOMAD-002
  - REQ-EXECUTORS-NOMAD-003
  - REQ-EXECUTORS-NOMAD-004
  - REQ-EXECUTORS-NOMAD-005
system_design:
  - ../../specs/executors/system-design/nomad-native-executor.md
legacy_specs: []
---

# Implementation Plan: Native Nomad executor

## Overview

Build one Nomad executor for native processes directly on the worker's host OS
through Kandev's existing ExecutorBackend seam. Validate macOS first; this does
not make native execution Mac-only or advertise unverified platform support.
Use the built-in runtime path, reuse SSH artifact/connection primitives, let
Nomad alone launch agentctl, and persist a successful cooperative drain receipt
before whole-job stop. Prove retry, disconnect and cleanup before supporting
the executor for general use.

Implementation proceeds through the sequential work orders below. Completion
of an individual work order does not establish full executor support or deployment.

## Scope

### In scope

- REQ-EXECUTORS-NOMAD-001 through REQ-EXECUTORS-NOMAD-005 in the paired
  [requirements](../../specs/executors/requirements/nomad-native-executor.md).
- Built-in runtime/profile registration, durable operation/environment identity,
  Nomad reservations and native job, existing pinned SSH transport, selected
  runtime artifact and normal ACP/terminal/file/Git flows.
- Generic authenticated seal-and-drain agentctl capability with internal durable
  receipt, cooperative cleanup and fail-closed dedicated-node quarantine.
- Real no-model session, A/B capacity, response loss/restart, client partition,
  managed child/grandchild cleanup and reproducible built-binary installation.

### Out of scope

- Docker, VM, custom scheduler/supervisor, automatic Linux migration.
- Plugin repository/package, public provider SDK/protobuf changes, Caddy or a
  mandatory public HTTPS gateway. Plugin migration is a separate future design.
- Rendered UI, production rollout, and model-backed workload tests.
- Hard isolation of hostile native workloads.

## Technical approach

The [system design](../../specs/executors/system-design/nomad-native-executor.md)
contains the source comparison and exact seam. Add NomadExecutor to
`runtime/lifecycle`, consistent RuntimeNomad/NameNomad/ExecutorTypeNomad taxonomy,
`backendapp/agents.go` registration, backend config validation/projections and
checkpoint/recovery/cleanup routing. A new string identity need not imply a SQL
schema migration: inspect existing constraints first and add a migration only if
required, with its scope and tests explicitly recorded. Use existing canonical
environment/executors_running inventory and secret references.

Reuse `ResolveSSHTarget`, `dialSSH`, `ensureAgentctlOnHostWithProgress`,
`StartPortForward`, `remoteControlHandshake`, `createRemoteAgentInstance` and
`openSSHRuntimeAPITunnelForRequest`. Do not call SSHExecutor.CreateInstance,
startAndForwardAgentctl or startAgentctlAndHandshake: those start another
controller. Nomad alone launches and stops the immutable job. Extract only a
small shared helper where existing code combines connection with SSH-owned
launch/kill behavior, and retain focused SSH regression coverage.

Add runtime-seal-drain.v1 plus a bounded authenticated endpoint/client backed by
existing instance.Manager.Shutdown and process admission. Establish ServerIdentity
through authenticated control, seal all process-start paths, persist exact
resource/generation/claim receipt before stop, then prove whole-job allocation
exit on the known live node. Unknown native cleanup quarantines the dedicated
node in Nomad and persists a non-GCed admission interlock.

| Runtime / transport | Identity and capability | Intended behavior | Evidence | Unsupported fallback |
| --- | --- | --- | --- | --- |
| New Nomad / raw_exec + pinned SSH forwarding | Immutable job/op/env generation; allocation UUID/NodeID; agentctl ServerIdentity and seal/drain v1 | Trusted native managed groups, normal retained lifetime | New actual Mac matrix below | Reject missing prerequisites; unknown cleanup quarantines |
| Existing SSH / pinned SSH | Existing remote process and workspace identity | Existing behavior unchanged; connection primitives reused | Focused shared-helper regressions in tasks 01-03 | No unmanaged second controller |
| Existing Kubernetes / port-forward and other built-ins | Existing exact resources and inventory | Existing behavior unchanged | Targeted tests when common taxonomy/lifecycle code changes | No blanket containerized classification for Nomad |
| Existing plugin_remote / HTTPS leases | Existing SDK resource/lease contract | No changes | No new SDK work required | Not selected for native contribution |
| Optional bare-metal Linux exec/cgroups | Separate capability profile required | Deferred | None in this package | Not advertised or silently selected |

## Tests

The table identifies the implementation's verification deliverables; completed
evidence is recorded in the corresponding work orders. TDD applies to changed
logic. No-tests-to-run and an empty result set fail a targeted check.

| Criteria (prefix AC-EXECUTORS-NOMAD-) | Planned exact evidence |
| --- | --- |
| 003.1, 003.3 | `server/api/runtime_drain_test.go:TestRuntimeSealAndDrain`; `server/instance/manager_runtime_drain_test.go:TestRuntimeDrainAdmission`; task 01 |
| 001.1, 001.3, 002.1, 002.2, 005.1 | `lifecycle/executor_nomad_inventory_test.go:TestNomadInventory`; `internal/agent/nomad/profile_test.go:TestNomadProfile`; taxonomy/registration tests; task 02 |
| 004.1-004.3, 001.1 | `lifecycle/executor_nomad_connection_test.go:TestNomadSSHConnection`; `bootstrap_test.go:TestNomadArtifactTransfer`; task 03 |
| 001.1, 001.3, 004.1-004.3 | `internal/agent/nomad/integration/session_test.go:TestNomadNativeSession`; task 04 |
| 002.1, 002.2, 004.2 | `integration/recovery_test.go:TestNomadNativeRecovery`; task 05 |
| 001.2, 002.3, 003.3, 003.4 | `integration/admission_disconnect_test.go:TestNomadNativeAdmissionDisconnect`; task 06 |
| 003.1-003.4, 002.2, 004.3 | `integration/cleanup_test.go:TestNomadNativeCleanup`; task 07 |
| 005.1, 005.2 | `integration/distribution_test.go:TestNomadNativeDistribution`; final strict matrix; task 08 |

## E2E tests

No rendered UI changes or browser test is needed. Headless end-to-end tests use
an actual disposable Kandev backend/database, real Nomad server/client, native
agentctl and an inert ACP binary through the normal agent path.

| Flow | Criteria | New runner scenario / test |
| --- | --- | --- |
| Durable allocation accepted by Kandev; pinned SSH control handshake; ACP prompt/result and child/grandchild; terminal/file/Git | 001.1, 001.3, 004.1-004.3 | `session` / TestNomadNativeSession |
| Drop register response after commit; restart backend at allocating/staging/bootstrap/ready; same resource and no duplicate prompt | 002.1, 002.2, 004.2 | `recovery` / TestNomadNativeRecovery |
| A reserves over half configured resource; separate B stays unallocated; safe release A allows B | 001.2 | `admission` / TestNomadNativeAdmissionDisconnect |
| Partition actual client/server while ACP runs; no restart/replacement; reconcile original UUID/ServerIdentity | 002.3, 003.3 | `disconnect` / TestNomadNativeAdmissionDisconnect |
| ACP child/grandchild and managed shell/LSP/Git owners drain, receipt persists, whole job stops, exact processes gone | 003.1, 003.2 | `cleanup-positive` / TestNomadNativeCleanup |
| Failed/lost drain, runtime crash/identity change, prequeued B while A groups survive/backend down, lost node/history or stale claim retain cleanup_pending and worker ineligibility | 002.2, 003.3, 003.4 | `cleanup-negative` / TestNomadNativeCleanup |
| Ordinary built Kandev binary starts with fresh data/profile and completes strict matrix; no source-only bypass | 005.1, 005.2 | `distribution` / TestNomadNativeDistribution |

### Prerequisites and runner contract

Future commands set absolute `KANDEV_ROOT` to this worktree and
`KANDEV_NOMAD_TEST_CONFIG` to a fixture-only JSON file. New
`scripts/nomad-native-e2e` owns preflight, isolated setup, named scenarios and
sanitized evidence. Task 02 creates the runner contract; later tasks implement
the named scenarios. An unimplemented scenario must fail explicitly.

Required native tooling: Go matching apps/backend/go.mod, Nomad initially 2.0.7,
POSIX shell/Git, SSH server/client with pinned key, accessible dedicated worker
account and native process identity probes. Colocation initially still uses
SSH. No Docker, VM, Caddy, public HTTPS certificate, frontend install or model
credentials are required. Nomad control API remains authenticated/TLS as the
operator deployment requires; SSH tunnel protection is not a reason to weaken
Nomad control-plane security.

The runner uses fresh backend DB, unique test namespace/job prefix, exclusive
ports, dedicated NodeID, isolated workspace/repository and inert ACP binary.
It records actual source/binary versions, operation, node/allocation/runtime
identities, receipt/checkpoint evidence and child/grandchild start identities. Record idle
Nomad-client and agentctl RSS plus startup durations using existing native ps/
timing tools; report observed values without inventing performance thresholds
or adding a benchmark framework.
Normal `go test` without opt-in may explicitly skip real-worker tests; strict
`run --require-real` fails on missing tooling, no matching tests, skips, missing
requested scenarios, unexpected replacements/leaks or missing proof. Skip is
unverified, never a successful work order.

Actual disconnect uses separate Nomad server/client processes with all client
RPC traffic routed through a fixture-owned loopback TCP relay. The server
advertises that relay address; both agents disable Consul auto-join. Closing
existing relay connections and refusing new connections must disconnect the
client while leaving its workload alive. Prove there is no direct RPC bypass.
Closing only Kandev's stream is insufficient. Preflight validates exclusive
fixture controls and refuses production endpoints; if partition control is
unavailable, report that exact unverified scenario. This fixture does not
change host firewall rules, production workers, or global SSH settings.

Positive cleanup removes only fixture-owned resources and verifies absence.
Unknown cleanup stops owned jobs where possible, retains sanitized evidence and
quarantine and exits failed with exact operator recovery instructions. Never
silently erase records, kill unidentified processes or re-enable a node.

## Work orders

- [x] [Task 01: Authenticated sealed runtime drain](task-01-runtime-drain.md)
- [x] [Task 02: Built-in identity and durable inventory](task-02-builtin-inventory.md)
- [x] [Task 03: Native launch and secure SSH connection](task-03-native-connection.md)
- [x] [Task 04: Real no-model session](task-04-real-session.md)
- [ ] [Task 05: Ambiguous-operation and restart recovery](task-05-recovery.md)
- [x] [Task 06: Capacity and actual client disconnect](task-06-admission-disconnect.md)
- [x] [Task 07: Cooperative cancellation and quarantine](task-07-cleanup-quarantine.md)
- [ ] [Task 08: Built runtime and contribution handoff](task-08-package-handoff.md) (cancelled; replaced by the [Nomad worker pool plan](../nomad-worker-pool/plan.md))

Execute 01 -> 02 -> 03 -> 04 -> 05 -> 06 -> 07 sequentially, then continue with the
[Nomad worker pool plan](../nomad-worker-pool/plan.md), whose final work order carries
the built-runtime proof and contribution handoff. The single pinned worker is the
first slice; the pool is the destination. Each has an
independent verification boundary; only the complete real matrix proves the
advertised supported executor.

## Verification results

Work orders 01–03 passed focused drain, inventory, registration, connection,
creation and recovery contract tests, including targeted race checks. A
supplemental full process-package race run timed out with environment-dependent
npm, parent-process and Git-helper failures; it is not a passing result.

Work order 04 passed an attached real-worker session through normal Kandev API
configuration: selected-repository materialization, inert ACP messaging,
file/tree/Git/terminal operations and negative SSH connection checks. Ordinary
Stop ended ACP descendants and retained the original allocation, controller,
workspace, fence and credentials. Backend shutdown closed host-side listeners
while preserving the remote environment. Idle RSS measured 85,968 KiB for the
Nomad client and 45,728 KiB for agentctl. Trustworthy startup timing and fresh
installation automation remain part of work order 08.

Work order 05 is in progress. Real post-commit response loss and ready-state
backend restart preserved the original allocation and completed-delivery
identity. A real pre-ready crash exposed generic Created-session inventory
pruning while its allocation remained live. Retention and guarded workspace-only
continuation regressions pass. A fresh real crash retained the original row
and allocation but exposed gaps in pre-ready admission and canonical workspace
projection. Corrections cover the Creating environment, canonical executor
ID/type and retained immutable workspace path. The continuation regression now
uses a durable admission reader rather than bypassing it; only the exact guarded
in-flight recovery entry can admit Creating, while ordinary callers remain
blocked. Full offline provider/matcher checks and focused continuation tests
pass. Offline digest verification additionally exposed original workspace-input
loss when recovery projects the resolved runtime path. The startup support
contract now requires explicit pre-dispatch input capture, strict immutable
validation and operator-only repair of legacy fixture evidence. That correction
is implemented with focused state/lifecycle regressions and an offline
actual-provider input-digest check. Canonical generation propagation now passes
focused lifecycle/SQLite checks, and the repaired operation reached runtime
phase Ready with original identities and no ACP/messages. A normal workspace
read still rejects its Creating environment: fenced canonical environment
readiness publication remains required before usable recovery is claimed.
The fresh captured-input automatic crash/restart proof also remains required;
a repaired operation cannot substitute for the clean proof. The previously
damaged fixture is excluded from automatic recovery evidence.
The damaged fixture was subsequently cleared through explicitly repaired
operator cleanup: the original saved auth token was reused without another
nonce handshake, a durable drain receipt preceded whole-job stop without purge,
and terminal allocation, exact process/port absence and fence release were
verified. This establishes fixture cleanup only; it does not prove automatic
recovery or the unfinished production cleanup path in work order 07.

Deliberate deviation (work order 05). The startup-continuation design has no
`task_environments` `creating`→`ready` write. Work order 05 adds one, owned by
`PublishRecoveredNomadEnvironmentReady`, for `nomad` environments only, plus a
Nomad-only re-election live-evidence guard and a typed recovery-guard code on
workspace reads. Specs are frozen on this card, so the owed follow-up is an
amendment of the startup-continuation design (step 4) adding the environment
publication, the re-election rule and the error mapping, plus the
task-launch-failure-recovery design ("Abandoned claims", "Re-election", physical
evidence list) which the Nomad re-election guard narrows. The PR body names the
deviation, and the amendment blocks marking work order 05 complete.

Work order 05 real proof passed on 2026-10-25 (run 10, local throwaway Nomad
2.0.7 fixture, supersedes runs 7 and 9): a pre-ready crash at the fixture-only barrier, an automatic
restart, environment `ready` and a normal workspace read before any prompt with
the recovered checkout HEAD asserted over pinned SSH, and the same execution, job, allocation and fence. Evidence is in
`docs/evidence/nomad-wo05/`. Its cleanup was an operator teardown; production
whole-job stop remains work order 07's.

Documentation catalog/specification checks have passed during implementation;
they do not establish runtime behavior. Capacity, actual client/server
partition, production cleanup/quarantine and the complete fresh built-runtime
matrix remain unverified. No supported-executor completion is claimed until
work orders 05–08 pass their required real scenarios.

## Risks

- Native Mac proof is cooperative managed-group teardown, not hostile-workload
  containment; a crash/escaped group may require operator recovery.
- Existing Shutdown WaitGroups need bounded/concurrent-drain tests before exposing
  completed receipt; all process-start surfaces must join admission.
- Runtime-specific inventory hooks, taxonomy and recovery predicates need an
  explicit Nomad audit; adding a Name alone is incomplete.
- Calling SSHExecutor's launch/kill workflow would split ownership and start an
  unmanaged second controller. Reuse transport primitives only.
- Nomad batch/disconnect/no-replacement behavior must pass the actual pinned
  release tests; defaults and service health are insufficient.
- The first worker has one active productive environment via a non-expiring
  start fence. It is separate from CPU/memory reservations; more concurrency
  needs another complete safety design.
- Worker-wide quarantine requires dedicated-node ownership and real Nomad
  eligibility enforcement before release; retain markers through history GC.
- Confirm Nomad CLI/client version, SSH access, and controlled partition
  capability during preflight. Do not silently install tools or start services.
- Keep vendor logic cohesive around internal boundaries; do not implement both
  built-in and plugin paths.
