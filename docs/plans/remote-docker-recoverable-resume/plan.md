---
created: 2026-10-29
status: implemented
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
system_design:
  - ../../specs/executors/system-design/remote-docker-executor.md
  - ../../specs/platform/system-design/durable-agent-journal-ownership.md
legacy_specs: []
---

# Fix plan: retain remote workspaces after recoverable resume errors

## Overview

Correct [issue #4880](https://github.com/kdlbs/kandev/issues/4880) without
discarding a retained workspace or bypassing durable-delivery admission.
First protect failure cleanup. Then reconcile the journal owner and expose
diagnostic errors. Finally prove recovery against a disposable remote daemon.
Execute work orders sequentially in the primary session after an explicit
implementation request. This package does not authorize implementation or delegation.

## Evidence and root cause

Investigation baseline: upstream main
`1b0d0881a4108697b8c346d17c5493fd34dd3f36`, verified against the remote on
2026-10-29. The issue was open and is now assigned to `carlosflorencio`.
Read the [reproduction record](reproduction.md) for the exact command and fixture.

Three source paths explain the upstream failure mechanisms:

1. `handleRecoverableFailureLockedState` records recovery and schedules
   `cleanupAgentExecutionWithReason`. That helper calls `StopExecution` with
   `force=true`. `shouldPreserveRecoverableRuntime` excludes `remote_docker`.
   `RemoteDockerExecutor.StopInstance` therefore removes the container.
2. `bringupAgentctl` passes the requested new execution ID to
   `findExistingInstance`. It can miss the previous same-session manager and
   create another manager with the same owner-scoped journal. The previous
   manager retains its bbolt handle after ordinary agent Stop.
3. `stopRegisteredLaunchRuntime` and `finishRegisteredLaunchRollback` protect
   resumed Kubernetes resources but do not protect a reused remote-Docker
   workspace and its recovery inventory.

A throwaway Go reproduction ran against unchanged production code. Both
recoverable stop reasons passed destructive force to the remote backend.
The reconnect fixture recorded this sequence:

```text
GET /health
GET /api/v1/instances/new-execution
POST /api/v1/instances
open delivery journal: timeout
```

The fixture used a real bbolt lock and different previous/requested execution
IDs. It did not use the reporter's deployment, credentials, or daemon.
This evidence proves upstream defects, not the historical incident's exact
lock holder. Real container preservation remains an implementation gate.

## Requirement conformance and ownership

The executor system owns workspace retention. Platform owns exclusive journal
access and admission. Existing requirements cover the intended repair:

- [Remote Docker](../../specs/executors/requirements/remote-docker-executor.md):
  `REQ-EXECUTORS-REMOTE-DOCKER-001`, criterion `.12`.
- [Durable delivery](../../specs/platform/requirements/durable-agent-delivery.md):
  `REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001`, criteria `.2` and `.3`;
  requirement `002`, criterion `.1`. Requirement `007`, criteria `.4` and `.6`.

Reuse these criteria without rewriting their behavior or adding a repair-only
requirement. The paired designs define the missing failure paths.
The existing durable-session and environment-ownership ADRs remain authoritative.

The assumption check found no material product choice that requires a question.
Preserve existing work, retain blocked uncertainty, and keep explicit destructive
cleanup available. These outcomes follow the issue and existing contracts.

## Scope

### In scope

- Recoverable remote-Docker failures and bootstrap of an existing workspace.
- Registered-launch rollback, failed agent stop, and exact-execution cleanup races.
- Same-session journal-owner reconciliation in the shared Docker reconnect path.
- Typed, sanitized journal-open diagnostics and atomic failure of instance creation.
- Disposable real-container regressions and current executor recovery guidance.

### Out of scope

- Recovery of data from an already removed container or volume.
- Checkpoint branches, automatic Git commits, or automatic pushes.
- New persistence, a new feature flag, or changes to native conversation policy.
- Global adoption of every prior instance ID or automatic deletion of lock files.
- A change to ordinary local-Docker teardown policy or unrelated executor policy.
- The archived-task prompt-edit symptom without a separate upstream reproduction.

## Technical approach

### Retention and cleanup ownership

Extend `recovery_stop.go` using the recorded runtime and workspace-reuse provenance.
Retained compute without a prior execution ID also needs protection. Keep
conversation-resume intent separate from authority to remove a container.
Protect both `StopAgentWithReason` and registered-launch rollback in
`manager_launch.go`. An unconfirmed stop retains exact cleanup ownership and
returns an error. It must not reach the remote executor's destructive
`AgentStopFailed` fallback or dispatch a workflow restart.
Preserve the authoritative environment, credentials, journal, and restart
evidence. Retain the existing destructive cleanup authority for fresh allocation,
Reset Environment, archive, and delete.

### Journal owner and diagnostics

In `executor_docker.go`, separate prior-owner lookup from the requested
replacement ID. Preserve different-session creation within an existing container.
Use authenticated instance information and delivery evidence. Adopt a matching
live process through existing recovery, or dispose of a proven stopped owner
before creating a replacement. Unknown or conflicting ownership blocks recovery.

In `server/instance/manager.go`, reject a configured journal error before
instance publication and tracker startup. Use provisional cleanup.
Add typed acquisition evidence at `journal.Open` and one sanitized process
diagnostic. Keep the default timeout, wrapped causes, and existing metric family.

| Runtime or transport | Identity and capability | Expected result | Evidence and fallback |
| --- | --- | --- | --- |
| Remote Docker over SSH | Same task/session, retained journal, compatible peer | Retain compute. Reconcile prior owner | Unit and real-daemon tests. Unknown evidence blocks |
| Local Docker | Shared reconnect, distinct session journal | Keep distinct-session creation | Reconnect regression. No blanket prior-ID reuse |
| Surviving initialized peer | Authenticated delivery identity | Adopt without ACP initialization or dispatch | Live-owner regression. Mismatch blocks |
| Legacy agentctl | Positively identified compatible legacy response | Retain existing eligible legacy behavior | Missing fields never authorize takeover |
| Kubernetes | Existing resume retention | Preserve current behavior | Existing retention controls |
| Unsupported or unavailable peer | Incomplete evidence | Preserve blocked recovery | No journal deletion or legacy downgrade |

No new provider or native-harness capability is claimed by this package.

## Tests

Permanent tests below are implementation targets. They do not exist yet.

| Acceptance criterion | Regression and location |
| --- | --- |
| `AC-EXECUTORS-REMOTE-DOCKER-001.12` | `TestRemoteDockerRecoverableFailurePreservesRuntime`, `TestRemoteDockerFailedResumePreservesRuntime`, and rollback/stop-failure controls in `manager_remote_docker_recoverable_failure_test.go` |
| `AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1` | `TestDockerReconnectReconcilesPreviousJournalOwner` and retained-file reopen assertions in `executor_docker_journal_reconnect_test.go` |
| `AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.6` | Live-owner, distinct-session, mixed-inventory, and concurrent-retry cases in the same reconnect file |
| `AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2` | `TestJournalOpenFailureClassification` in `journal_open_error_test.go`. `TestCreateInstanceRejectsUnavailableJournal` in `manager_journal_admission_test.go` |
| `AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.3` | Journal release after exact instance teardown. Failed-teardown retry in the same instance test file |
| `AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4` | `TestJournalFailureCannotSelectLegacyReconnect` in the reconnect file |

Each work order specifies exact commands and its required RED evidence.

## E2E tests

Task 03 adds `tests/remote-docker/remote-docker-recovery.spec.ts` in the
`containers` project. Use the existing SSH-to-Docker fixture and real UI Resume.
Write an unpushed marker commit and an untracked marker.
Induce a bounded bootstrap/journal conflict.
Assert unchanged container ID, commit, and files.
Release only the fixture's lock owner, resume successfully, and repeat after a
backend restart. Finish with explicit cleanup controls.
This proves `.12`, delivery `001.2`, `002.1`, `007.4`, and `007.6` together.

There is no rendered UI change, so no ASCII preview or new phone composition
is required. The existing session recovery controls remain the entry point.
Run `session-error-recovery-ui.spec.ts` and its `mobile-` counterpart for desktop
and phone feedback, reload persistence, blocked retry, and cleanup guidance.
Those browser checks supplement the real-daemon test. They do not prove storage
retention. Do not add per-test device overrides to simulate a mobile project.

## Work orders

- [x] [Task 01: Preserve established remote workspaces during failed recovery](task-01-preserve-remote-workspace.md)
- [x] [Task 02: Reconcile journal ownership and classify open errors](task-02-reconcile-journal-owner.md)
- [x] [Task 03: Prove real remote recovery and document its limits](task-03-remote-recovery-evidence.md)

## Companion implementation records

- [Remote Docker foundation](../remote-docker-executor/plan.md), especially tasks 04 and 06.
- [Durable agent sessions](../durable-agent-sessions/plan.md), especially tasks 08 through 10.
- [Delivery reconciliation](../durable-agent-session-reconciliation/plan.md).
- [Journal shutdown recovery](../agentctl-journal-shutdown-recovery/plan.md).

This package owns the additional #4880 regression matrix. Historical task
statuses and previous results remain unchanged. They do not prove these new
failure paths. During implementation, update affected companion evidence if a
shared contract or scenario changes. Do not mark unrelated open work complete.

## Verification results

- Investigation: throwaway lifecycle reproduction exited 1 as expected.
  Two destructive-force assertions and the real journal-lock assertion failed.
- Task 01: implementation complete. Permanent manager and fake-Engine regressions
  pass; the temporary RED predicate reversion failed the same assertions. All
  post-attachment launch rollback paths use the same reuse-aware policy, not
  only registered rollback.
- Task 02: implementation complete. The real-lock reconnect fixture reproduced
  `open delivery journal: lock_timeout` with reconciliation disabled and passes
  with it enabled. Journal admission, teardown/reopen, classification, bounded
  diagnostics, durable adopted-agentctl identity, and the no-fresh-fallback
  fence for replacement creation pass their tests.
- Task 03: implementation and real-daemon verification complete.
  `KANDEV_E2E_CONTAINERS=1 pnpm e2e:raw --project=containers tests/remote-docker/remote-docker-task.spec.ts`
  passed 3/3. The same command for `remote-docker-recovery.spec.ts` passed 2/2:
  same-session resume with a distinct marker turn (18.9s) and blocked journal
  with backend restart, persisted failure/log assertions, and a successful
  marker retry (22.1s). Desktop and phone recovery feedback specs each passed
  10/10. The image's `flock` replaces the planned Go helper.
- Go verification: the targeted Task 01 and Task 02 suites pass under `-race`;
  the full `internal/agent/runtime/lifecycle` package suite passed in 439.405s.
  `golangci-lint --new-from-rev=HEAD` reports 0 issues for the changed
  packages.
- `python3 scripts/list-docs.py validate`: passed, 416 decisions and 1838 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Documentation coverage preflight over the actual diff: covered, three work orders, zero reference errors.
- `node scripts/validate-public-docs.mjs`: validated 54 published docs pages.
- `git diff --check`: passed.
- Package status and new-file whitespace checks: passed. All artifacts remain unstaged and uncommitted.

## Risks

- Changing only the force predicate leaves destructive stop-failure and rollback paths.
- Reusing every prior instance ID breaks valid different-session creation.
- Disposing of a live or ambiguous owner can terminate work and corrupt recovery.
- Failed cleanup must retain enough ownership evidence for restart and explicit removal.
- The issue describes a downstream deployment. Its exact incident sequence remains unverified.
- Docker/SSH and browser fixtures must actually run before implementation is complete.

## Secondary observation

The workflow edit calls `AuthorizeWorkflowExecutorMutation`, which visits task
sessions through `authorizeTaskExecutorMutation` and `authorizeSessionExecutor`.
A stale session environment reference can propagate `GetTaskEnvironment` errors.
The HTTP handler maps any error containing `not found` to `Step not found`.
This source path is consistent with the reported secondary symptom. No focused
reproduction established its archived-task preconditions during this investigation.
Authorization semantics require a separate repair scope. Do not bypass them
as part of journal recovery. This observation is outside these work orders.
