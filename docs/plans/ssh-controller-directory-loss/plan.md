---
created: 2026-11-07
status: completed
requirements:
  - REQ-EXECUTORS-SSH-EXECUTOR-001
system_design:
  - ../../specs/executors/system-design/ssh-executor.md
  - ../../specs/executors/system-design/ssh-agentctl-reconciliation.md
legacy_specs: []
---

# Fix plan: SSH controller reconciliation after directory loss

## Overview

Resolve [issue #5145](https://github.com/kdlbs/kandev/issues/5145) for launches
with durable process proof. Record launch identity first. Then add a guarded
remote reconciliation helper and prove cleanup recovery across restart.

The issue is assigned to `carlosflorencio`. All three work orders are
implemented; Task 03's real SSH/container E2E is recorded separately below.

## Confirmed cause and evidence

Investigation used commit `52309e613fc9a76d38a9f891b74b4d8b32bff265`, also the
remote `main` head observed on 2026-11-07. Issue #5145 cites an earlier source
snapshot. Its guard description still matches this checkout.

`SSHExecutor.StopInstance` selects `stopPersistedRemoteAgentctl` when no local
session state survives. The identity verifier first accepts the controller's
command/workdir, then requires the session-owned pidfile. A missing directory
returns an error before any stop command. Persisted metadata has no original
process birth token or independently checked launch generation.

A temporary `TestReproIssue5145LiveAgentctlMissingSessionDirectory` used the real
persisted stop path and the package's fake SSH server. It supplied a matching
live controller, missing pidfile, and positively absent directory. Two retries
returned `disappeared before pidfile verification`. Six SSH probes ran, with
zero signal or removal commands. The temporary test was removed after capture.

Existing identity, dead-PID, replacement-directory, and sibling-session tests
passed. This establishes a backend lifecycle defect and the current guard's
behavior. It does not claim a new live-host reproduction.

Issue #4593 concerns a confirmed-dead controller during idle reclaim. Its
completed [package](../ssh-agentctl-absence-idle-reclaim/plan.md) does not supply
authority over this issue's live process. The
[orphan sweep package](../ssh-orphaned-agentctl-sweep/plan.md) is independent.
this repair does not implement or expand that sweep.

## Scope

In scope: AC-EXECUTORS-SSH-EXECUTOR-001.18 through AC-001.24, immutable receipt
production, persistence/reset rules, Linux pidfd SIGTERM, exact-owner retry
settlement, explicit unresolved reasons, targeted tests, and operational docs.

Automatic legacy proof creation, unrecorded-process discovery, and Darwin
signalling extensions are out of scope. So are forced termination, new UI,
public recovery APIs, SQL schema changes, task-directory removal, and
pre-handshake launch-crash recovery.

## Technical approach

### Launch and durable identity

Task 01 adds shared receipt/binding types in `internal/common/sshlaunch` and an
optional managed launch binding in the existing `AgentctlStartupConfig` type
in `internal/common/config/agentctl.go`. This extends the current
managed child contract without adding a second parser.

`ControlServer.handleHandshake` returns self-captured proof for managed SSH
launches. `startAgentctlAndHandshake`, `remoteControlHandshake`, `buildInstance`,
and recovered SSH state carry it into `executors_running.metadata` through
`persistentMetadataKeys` and `buildRunningFromExecution`.

Reset paths clear the receipt and block prior-metadata fallback. No receipt is
sampled from an arbitrary PID during resume or cleanup.

### Remote signal and owner fencing

Task 02 adds the private `agentctl ssh-reconcile` utility and its package under
`internal/agentctl/sshreconcile`. It reuses `processidentity` capture/parsing and
adds a narrow pidfd process operation. It receives JSON through SSH stdin.

The helper admits only a complete receipt with a confirmed missing session
path. It checks birth identity and generation after opening the process handle,
rechecks the path before SIGTERM, and waits five seconds for exit. It removes
no paths and never falls back to a numeric-PID signal.

The lifecycle path uses typed outcomes, exact runtime ownership checks, and
the existing bounded cleanup context. Failed or lost replies preserve retry
evidence. New receipt metadata must also participate in recovery comparisons.

### Recovery and operational guidance

Task 03 proves receipt preservation in cleanup inventory and exact-owner
settlement across restart. It covers mixed targets and existing retry behavior.
It adds a real SSH/container E2E regression and updates the SSH cleanup section
of `docs/public/executors.md` when implementation exists.

The public explanation must distinguish new supported launches, legacy
unresolved entries, permission/probe failures, and unconfirmed termination.
It must not publish PID-only kill instructions or imply an upgrade proves
ownership of already-running legacy processes.

### Compatibility matrix

| Runtime/transport | Proof | Behavior | Evidence |
| --- | --- | --- | --- |
| Linux SSH, new helper | Self-captured birth identity and generation | Guarded pidfd SIGTERM and retry | Real subprocess plus SSH E2E |
| Linux SSH, legacy process | PID/workdir/pidfile only | Existing guard, missing-directory outcome remains unresolved | Fake SSH and cleanup integration |
| Linux SSH, pidfd unavailable | Complete receipt, unsupported kernel/policy | No weaker signal fallback | Injected syscall errors |
| Darwin SSH | Existing metadata | Existing guarded stop and absence behavior | Existing platform-shaped probe tests |
| Docker, Kubernetes, Sprites, local | Existing runtime contract | No new helper dispatch | Dispatch-boundary regression |

## Tests

Names below are the implemented targets. Each is present in the worktree.

| Criteria | Test file and regression target |
| --- | --- |
| AC-001.18 | `executor_ssh_launch_receipt_test.go`: `TestSSHLaunchReceiptSurvivesPersistenceAndResume`, `TestSSHLaunchReceiptReplacementDoesNotInheritPriorProof` |
| AC-001.18, AC-001.24 | `handshake_ssh_launch_test.go`: `TestHandshakeSSHLaunchReceiptBindsSelfAndOwner` |
| AC-001.19, AC-001.22 | `executor_ssh_directory_loss_test.go`: `TestSSHDirectoryLossReconciliationOriginalProcessAndRetry` |
| AC-001.20 | `reconcile_linux_test.go`: `TestReconcileRejectsIdentityChanges`, `TestReconcileReplacementBeforeSignalPreservesTargets` |
| AC-001.21, AC-001.23 | `executor_ssh_directory_loss_test.go`: `TestSSHDirectoryLossReconciliationPreservesUnknown` |
| AC-001.23, AC-001.24 | `resource_cleanup_ssh_reconciliation_test.go`: `TestSSHCleanupReconciliationRetainsMixedAndSuccessorTargets` |

For the primary RED test, give the persisted stop a valid launch receipt,
matching original process, and missing session directory. Require successful
bounded stop. The current code fails with the captured missing-directory error.
Do not replace that assertion with an expectation that the old error persists.

Each work order includes additional contract and failure cases plus exact checks.
Use synchronization hooks for identity races. Do not depend on actual PID reuse.

## E2E tests

`apps/web/e2e/tests/ssh/cleanup-reconciliation.spec.ts` runs in project
`containers` against the existing isolated SSH fixture. It starts real SSH
tasks, removes only the main session runtime directory, restarts the backend,
and archives through the task API (the same cleanup path the archive control
invokes). It requires the original controller to exit while a sibling
controller and both task workspaces remain intact.

The companion replacement-directory scenario recreates the session path with a
marker and asserts that an archived cleanup attempt preserves both the
directory and the recorded controller. This covers AC-001.19, AC-001.20,
AC-001.23, and AC-001.24. No rendered UI changes are planned, so no new
desktop/phone composition or UI preview is required.

## Work orders

- [x] [Task 01: Record immutable SSH launch proof](task-01-record-launch-proof.md)
- [x] [Task 02: Reconcile the original remote controller safely](task-02-reconcile-controller.md), after Task 01
- [x] [Task 03: Prove durable cleanup and document recovery](task-03-prove-cleanup-recovery.md), after Task 02

All work orders are sequential. No delegation is authorized.

## Verification results

Investigation passed against
`52309e613fc9a76d38a9f891b74b4d8b32bff265` and established the missing-directory
refusal. The implementation then added the launch-receipt gate, the Linux
pidfd helper, exact-owner revalidation, and durable cleanup coverage.

Backend suites passed:

```sh
(cd apps/backend && go test -trimpath -tags fts5 ./internal/common/sshlaunch ./internal/common/processidentity ./internal/agentctl/sshreconcile ./internal/common/config ./internal/common/launchwrapper ./internal/agentctl/server/config ./cmd/agentctl -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/api -run 'TestHandshake' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/service -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/sshreconcile -count=1)
(cd apps/backend && GOOS=darwin GOARCH=arm64 go build -trimpath ./cmd/agentctl ./internal/agentctl/sshreconcile ./internal/common/sshlaunch ./internal/common/processidentity)
KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e:run --host --shards 1 --project containers tests/ssh/cleanup-reconciliation.spec.ts
```

`bash scripts/lint-go-changed` (run as
`golangci-lint run --allow-parallel-runners` because another workspace held the
default lock) reported `0 issues` for every changed Go package.

Documentation checks passed:

- `node scripts/validate-public-docs.mjs`: 55 published pages validated.
- `python3 scripts/list-docs.py validate`.
- `python3 scripts/lint-spec-files.py --all`.
- `apps/web` lint for the new E2E spec: clean.

The two container E2E scenarios in Task 03 passed in the `containers` project
(2 passed in 1.4m) against real sshd containers. The success scenario now also
waits for exact-target settlement (the durable executor row disappears), and
the replacement scenario waits for a target-correlated cleanup refusal before
asserting preservation.

A code-only review of the first implementation identified five findings; all
were fixed and covered:

1. Present receipts are validated, including PID, before every destructive or
   adoption branch. A null/malformed/foreign/PID-mismatched receipt never
   degrades to legacy evidence. A verify-only helper operation re-proves the
   live birth identity and generation before the guarded pidfile stop and
   before recovery adoption.
2. The pending-recovery exception is limited to the exact archive delta (same
   workspace, environment ownership/generation, route, and delivery revision,
   session cancelled under a running cleanup claim). Unrelated snapshot
   changes remain fenced.
3. The helper settles `stopped` or `absent` only after the recorded session's
   children are gone. A surviving child, including on a retry after controller
   exit, stays unresolved; no unverified process-group kill is introduced.
4. Helper response transport is capped on both stdout and stderr while still
   draining; overflow returns `probe_unknown`.
5. The E2E waits for durable settlement or target-correlated refusal evidence
   and keeps fixture cleanup in `finally`.

## Risks

- Linux pidfd policy and procfs visibility vary. Unsupported or denied access
  must retain evidence without numeric-PID fallback.
- Metadata merging can resurrect a prior generation unless the new key has
  explicit reset semantics. Capture this at the production persistence boundary.
- A dropped reply can follow successful SIGTERM. Retry must inspect absence
  without repeating authority against a recycled PID.
- New proof cannot safely repair already-running legacy controllers. Their
  unresolved outcome is deliberate and must be actionable.
- The first version does not force-kill an unresponsive controller. Child
  shutdown remains with agentctl. Unconfirmed completion preserves inventory.

## Decisions

See the [proposed launch-proof ADR](../../decisions/2026-11-07-ssh-controller-launch-proof.md).
The existing guarded pidfile path remains the compatibility boundary.
