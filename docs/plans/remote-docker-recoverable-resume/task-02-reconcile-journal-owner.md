---
id: "02-reconcile-journal-owner"
title: "Reconcile journal ownership and classify open errors"
status: completed
wave: 2
depends_on: ["01-preserve-remote-workspace"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
acceptance_criteria:
  - AC-EXECUTORS-REMOTE-DOCKER-001.12
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.6
system_design:
  - ../../specs/executors/system-design/remote-docker-executor.md
  - ../../specs/platform/system-design/durable-agent-journal-ownership.md
---

# Task 02: Reconcile journal ownership and classify open errors

## Summary

Resolve the previous same-session manager before a replacement opens its journal.
Preserve live-owner adoption and different-session creation. Make acquisition
failure explicit before publishing a new instance, with safe diagnostic evidence.

## In scope

- Separate lookup identity from requested creation identity in Docker reconnect.
- Validate prior owners with authenticated instance and delivery evidence.
- Reuse existing adoption for a live initialized owner without ACP initialization.
- Dispose of a proven stopped owner through exact instance teardown before replacement.
- Reconcile bounded same-session inventory after a failed intermediate attempt.
- Refuse takeover on incomplete, conflicting, or ambiguous evidence.
- Fail configured-journal instance creation before publication or tracker startup.
- Add typed acquisition reasons and sanitized structured diagnostics.

## Out of scope

New protocol versions, database schema, longer timeouts, lock-file removal,
shared journal-handle caches, unconditional previous-ID reuse, and new recovery UI.

## Acceptance

1. Different execution IDs for the same retained session cannot create competing
   journal owners. Adoption or confirmed exact disposal precedes replacement.
2. Every relevant candidate has a classified outcome. Unknown status, failed
   inventory/disposal, concurrency, or stale ownership preserves blocked recovery
   and retained data. Different-session creation remains supported.
3. Acquisition failure exposes a typed reason, retains its wrapped cause, and
   prevents publication of a broken instance. Diagnostics omit secrets and content.

## Regression evidence

First recreate the real-lock reconnect fixture from [the investigation](reproduction.md)
in `executor_docker_journal_reconnect_test.go`.
Use `TestDockerReconnectReconcilesPreviousJournalOwner` as the primary RED test.
Its old/requested IDs must differ, and the old manager must hold a real journal.

Add stopped-owner release/reopen and surviving live-owner cases.
Add failed DELETE, lost DELETE response, older-owner retry, and concurrent resume cases.
Include a mixed inventory with one verified owner and one unresolved candidate.
Assert no mutation of the live candidate or automatic creation in that case.
Keep `TestReconnectInstanceID` and broker preflight behavior covered.
Test distinct-session creation with different owner paths in the same container.

Add `TestJournalOpenFailureClassification` with a real lock plus controlled
permission, full-filesystem, quota, I/O, corrupt, and newer-format outcomes.
Assert `errors.Is` compatibility and stable safe error projection.
Do not infer lock-holder PID from `bolt.ErrTimeout`.

Add `TestCreateInstanceRejectsUnavailableJournal` and exact teardown/reopen
coverage in `manager_journal_admission_test.go`. Assert provisional cleanup,
no published instance/trackers, and continued operation of another instance.
Capture diagnostics with secret and prompt sentinels and assert their absence.
Add `TestJournalFailureCannotSelectLegacyReconnect`.

## Verification

Run from the repository root. Run the primary RED test before production edits.
Run the complete block after the correction.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -run 'Test(Docker.*(Reconnect|Journal)|ReconnectInstanceID|BuildReconnect|RemoteDocker.*(Reconnect|Recover)|JournalFailureCannotSelectLegacyReconnect)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agentctl/journal ./internal/agentctl/server/process ./internal/agentctl/server/instance ./internal/agentctl/server/api -run 'Test.*(Journal|Delivery|CreateInstance|StopInstance|Provisional)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/agentctl -run 'Test.*(GetInstance|ListInstances|CreateInstance|DeleteInstance|Delivery)' -count=1)
git diff --check
```

Run `bash scripts/lint-go-changed` with the actual changed Go files after these
tests. Keep new tests in separate files under the backend file-size limit.
Record every command and any skipped filesystem capability test.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker_journal_reconnect_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/executor_remote_docker_reconnect_delegate_test.go`
- `apps/backend/internal/agentctl/journal/journal.go`
- `apps/backend/internal/agentctl/journal/journal_open_error.go` and `journal_open_error_test.go` (new)
- `apps/backend/internal/agentctl/journal/metrics.go`, only for bounded classification.
- `apps/backend/internal/agentctl/server/process/manager.go` and a new journal diagnostic test file.
- `apps/backend/internal/agentctl/server/instance/manager.go`
- `apps/backend/internal/agentctl/server/instance/manager_journal_admission_test.go` (new)
- `apps/backend/internal/agentctl/server/api/control_server.go` and control admission tests, if error projection needs correction.
- `apps/backend/internal/agent/runtime/agentctl/control.go` and tests, only for existing control-client evidence transport.

## Dependencies

[Task 01](task-01-preserve-remote-workspace.md) protects the workspace while new
negative journal tests exercise failed recovery.

## Risks

- A prior manager can be stopped while its journal remains open.
- An old instance ID does not prove current ownership, process termination, or delivery settlement.
- Broker preflight can deliberately replace an instance. Preserve its ordering.
- An older peer can omit diagnostics. Omission is not permission to reclaim it.

## Parallelism

`sequential`

## Inputs

- Both listed designs and their requirement mappings.
- `process.Manager.Stop`, `StopForTeardown`, and `closeDeliveryJournal`.
- `instance.Manager` provisional cleanup and exact-instance teardown.
- `DockerExecutor.bringupAgentctl`, `findExistingInstance`, and
  `TestDockerManagedBrokerReconnectRecreatesInstanceWithFreshLease`.
- Existing durable adoption, delivery uncertainty, and wrapped-error contracts.

## Results

Completed on 2026-10-29 against upstream `1b0d0881a4108697b8c346d17c5493fd34dd3f36`.

- **RED:** Temporarily making `reconcilePreviousJournalOwner` a no-op reproduced
  the reported failure in `TestDockerReconnectReconcilesPreviousJournalOwner`
  and `TestDockerReconnectAdoptsLivePreviousJournalOwner`:
  `failed to create new instance: open delivery journal: lock_timeout`. The
  fixture holds a real bbolt journal and uses different previous/requested
  execution IDs.
- **GREEN:** The shared Docker reconnect path now lists authenticated instances,
  validates task/session ownership, classifies every relevant candidate, adopts
  one live owner, disposes stopped owners before replacement, and blocks on
  unknown, ambiguous, over-bound, or failed-disposal evidence. Distinct-session
  instances and their journals are left untouched. Requested-ID lookup and
  broker preflight ordering are unchanged. A blocked-owner error carries
  `errReconnectOwnerBlocked` so neither local nor remote Docker falls back to a
  fresh container that would abandon the retained workspace.
- **Live identity:** live adoption reports the prior agentctl instance ID for
  the per-instance client's `X-Instance-ID` header while the backend execution
  keeps the requested ID. The header guard therefore accepts requests to the
  adopted server, and the executor session/target maps stay keyed by the
  requested execution.
- **Durable adopted identity:** the actual agentctl instance ID is persisted in
  `remote_docker_agentctl_instance_id`, separately from the backend execution
  ID. Recovery uses it for control lookup, client headers, authentication
  refresh, and exact ownership checks. A fake-control regression adopts an old
  ID as a new execution, passes it through `ToAgentExecution` and
  `FilterPersistentMetadata`, then recovers the same live process without
  creating or deleting an instance.
- **Replacement admission:** a journal or instance-admission failure while
  opening a replacement after disposal is wrapped in `errReconnectOwnerBlocked`.
  Both local and remote Docker therefore keep the retained environment instead
  of falling back to a fresh container when `WorkspaceReuseRequired` is false.
  Regressions drive `DockerExecutor.CreateInstance` and
  `RemoteDockerExecutor.CreateInstance` with a retained container and a
  controlled POST journal failure, and assert the fresh launch callback and
  Engine create request never run.
- **Journal admission:** `process.NewManager` emits one bounded diagnostic with
  the reason, timeout, elapsed time, local PID, owner identifiers, and trusted
  path. `instance.Manager.CreateInstance` rejects a configured journal failure
  before tracker startup and publication. The provisional cleanup path retries
  the same exact identity after the owner closes.
- **Journal errors:** `journal.Open` wraps acquisition failures in `OpenError`
  with the closed reason set `lock_timeout`, `permission_denied`,
  `filesystem_full`, `disk_quota_exceeded`, `io_failure`, `corrupt`,
  `newer_version`, and `unavailable`. `Unwrap` preserves `bolt.ErrTimeout`,
  `ErrJournalCorrupt`, `ErrJournalNewerVersion`, and typed filesystem causes.
  The safe `Error()` omits paths and causes. Metric labels stay bounded.
- **Commands:** lifecycle reconnect suite passed; journal/instance/api suite
  passed; the process-manager Journal/Delivery selection passed after 98.201s;
  the agentctl control-client selection passed. `golangci-lint
  --new-from-rev=HEAD` reported 0 issues for the changed packages.
- **Skipped capability:** the permission-denied subtest skipped because the test
  process runs as root and bypasses permission bits. Filesystem-full, quota, and
  I/O classification use typed `PathError` causes rather than a mounted
  full/quota filesystem. Corruption, newer-format, and real bbolt lock outcomes
  ran for real.
- **Not covered:** concurrent cross-process resume against a real remote daemon
  and browser-visible blocked recovery belong to Task 03.
