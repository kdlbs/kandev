---
id: "01-reconcile-standalone-journal-owner"
title: "Reconcile standalone owners and preserve live adoption"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.6
system_design:
  - ../../specs/platform/system-design/durable-agent-journal-ownership.md
  - ../../specs/platform/system-design/durable-agent-reattachment.md
---

# Task 01: Reconcile standalone owners and preserve live adoption

## Summary

Resolve the session's retained managers before standalone instance creation.
Adopt a verified live owner through existing lifecycle reconciliation.
Dispose of stopped managers only after complete ownership and admission checks.

## In scope

- Add an authenticated pre-creation resolver on the current runtime lease.
- Inspect the requested ID, recorded predecessor, and all same-session candidates.
- Bound candidates at eight. Reject incomplete or conflicting evidence before mutation.
- Keep different-session instances operational throughout recovery.
- Return the actual retained ID and port, with immutable existing-process provenance.
- Carry that identity through launch and workspace construction, registration, callbacks, tracing, persistence, and Stop.
- Reject substitution of a different live ID into a pinned restore candidate.
- Reuse the current native association, SQL generation, cursor, submission, and replay checks.
- Protect all rollback paths from disposing of an adopted peer or revoking its MCP ownership.
- Reconcile partial DELETE outcomes before another attempt opens the same journal.
- Reuse existing session serialization and admission for concurrent callers.

## Out of scope

- Changes to survival policy, shared-server replacement, and startup orphan policy.
- Journal deletion, timeout changes, alternate journal paths, or legacy downgrade on error.
- Native conversation replacement, automatic prompt resend, and queue release.
- Public endpoints, schema changes, flags, and rendered UI changes.
- Real restart integration owned by Task 02.

## Acceptance

1. Real-lock tests fail before the correction and pass afterward. Stopped owners
   release their handles before POST. Live owners retain their actual execution,
   stream, native session, and process without ACP lifecycle calls or dispatch.
2. Every relevant candidate passes ownership and liveness checks before mutation.
   Mixed valid/unknown inventory, conflicts, excess candidates, failed disposal,
   epoch changes, and cancelled callers preserve uncertain work and unrelated sessions.
3. Lifecycle tests prove actual-ID persistence and Stop, pinned-candidate refusal,
   single-owner concurrency, and peer preservation across every selected rollback stage.

## Regression sequence

Start from [the diagnostic source](reproduction.md#temporary-test-source).
Complete its task/workspace, native association, generation, and SQL delivery
fixtures before requiring successful live adoption. The diagnostic alone proves
duplicate acquisition, not sufficient authority for adoption.
Write permanent `TestStandaloneReconnectDisposesStoppedJournalOwner` and
`TestStandaloneReconnectAdoptsLiveJournalOwner` in the new reconnect test file.
Run them before production edits. Record `lock_timeout` as behavioral RED.

Seed one committed event and an unresolved submission in the old journal.
After stopped-manager replacement, reopen and assert their identities and hashes.
For live adoption, assert no CREATE or DELETE and no initialize/load/resume/new/prompt.

Add `TestStandaloneReconnectBlocksUnresolvedJournalOwners` with these cases:

- Requested ID already exists, or the predecessor names a failed intermediate attempt.
- Multiple live owners, or a matching owner plus an unknown candidate.
- Conflicting task, workspace, or delivery identity.
- Unavailable inventory or status, failed DELETE, or a lost DELETE response.
- Different-session controls.

An absent exact ID needs authenticated absence.
One matching candidate cannot override another unresolved candidate.

Add these lifecycle tests in `standalone_retained_owner_lifecycle_test.go`:

- `TestStandaloneRetainedOwnerUsesActualExecutionIdentity`.
- `TestStandaloneRetainedAdoptionFailurePreservesPeer`.
- `TestStandaloneRetainedOwnerConcurrentAdmission`.
- `TestStandaloneRetainedOwnerRejectsPinnedCandidateSubstitution`.

Drive both `Launch` and workspace creation. Cover failures before execution
construction, MCP preparation, registration, persistence, and stream adoption.
Assert unchanged peer environment, source roots, durable inventory, and MCP claims.
Verify that Stop remains usable and native identity mismatches remain blocked.
Block stale cleanup until a successor exists, then assert that it survives.
Cancel one joined waiter while another caller still owns the admitted operation.

## Verification

Run from the repository root. The first command is the focused RED gate.
After implementation, run the complete block and record every result.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestStandaloneReconnect(DisposesStoppedJournalOwner|AdoptsLiveJournalOwner)$' -count=1 -v)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestStandaloneReconnect|TestStandaloneRetainedOwner|TestStandaloneRetainedAdoption|TestReuseExisting|TestDockerReconnect)' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agentctl/server/instance -run '^TestCreateInstance(RejectsUnavailableJournal|TeardownReleasesJournal)$' -count=1)
mapfile -t standalone_changed_go < <(
  {
    git diff --name-only HEAD -- apps/backend
    git ls-files --others --exclude-standard -- apps/backend
  } | rg '\.go$' | sort -u
)
if ((${#standalone_changed_go[@]})); then
  bash scripts/lint-go-changed "${standalone_changed_go[@]}"
fi
git diff --check
```

The lint command includes tracked and untracked Go changes.
Record its expanded paths in Results. Do not run full-backend lint by default.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_standalone.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_standalone_reconnect.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_startup.go`
- `apps/backend/internal/agent/runtime/lifecycle/recovery_stop.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_standalone_journal_reconnect_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/standalone_retained_owner_lifecycle_test.go` (new)

Use existing `live_adoption.go`, `durable_adoption.go`, and
`startup_disposition.go` as integration contracts. Change them only if the
standalone regression requires a shared correction, then cover that correction.

## Dependencies

None. Existing Docker reconciliation and live adoption are available on the baseline.

## Risks

- An ordinary process Stop leaves the journal open. Only exact teardown releases it.
- An instance ID and an unused requested execution ID are different ownership claims.
- A stopped process does not prove that an uncertain instruction completed.
- Returning a retained peer into fresh-allocation cleanup can terminate valid work.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), including compatibility and test mappings.
- [Journal ownership](../../specs/platform/system-design/durable-agent-journal-ownership.md#standalone-ownership-resolution).
- [Reattachment](../../specs/platform/system-design/durable-agent-reattachment.md#surviving-agent-reattachment).
- [Delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md), criteria listed above.
- Existing `executor_docker_reconnect_owner.go`, `executor_docker_journal_reconnect_test.go`, and `manager_startup_adoption_test.go`.
- Existing instance teardown contract and `manager_journal_admission_test.go`.

## Results

Implemented.

Production:
- Added `executor_standalone_reconnect.go`. `StandaloneExecutor.CreateInstance`
  resolves the requested ID, recorded predecessor, and every same-session
  candidate through the current control binding before opening a configured
  journal. It verifies task, session, workspace, identity, and delivery
  ownership, bounds candidates at eight, and blocks incomplete, duplicate,
  conflicting, or unknown evidence before mutation.
- A verified live owner is returned with its actual ID in both
  `ExecutorInstance.InstanceID` and `StandaloneInstanceID`, its verified port
  and authenticated client, retained environment and source roots, and
  `MetadataKeyReuseExistingProcess`.
- Proven-stopped owners are disposed through the exact DELETE contract. A lost
  DELETE response is reconciled through an authenticated exact-instance read;
  only confirmed absence authorizes the replacement POST. The resolver rechecks
  inventory after any disposal.
- `ToAgentExecution` prefers the retained instance's environment and source
  roots. Execution construction, tracing, registration, persistence, and Stop
  use the selected identity, and the captured resource-circuit identity moves
  to the selected execution. `buildExecutionFromInstance` refuses a different
  identity when `CandidateExecutionID` pins a restore candidate.
- Adoption skips project MCP materialization and MCP provider promotion, so a
  launch request cannot overwrite the retained peer's MCP ownership. A recoverable
  failure before adoption completes closes only the attempt-local client.
- Adoption provenance freezes rollback behavior: every post-selection failure
  closes only attempt-owned transport, preserving the peer process, journal,
  durable row, inventory, and MCP claims. A recoverable failure before adoption
  completes follows a nonterminal detach: it keeps the authoritative runtime row
  and unresolved delivery state, publishes no `agent.stopped`, and leaves
  recovery/Stop authority intact. Explicit Stop remains destructive.
- Workspace restore carries the persisted built-in MCP binding through a
  dedicated `RetainedBuiltinBinding` field, restores only that exact value on a
  selected peer, and verifies it without running fresh-helper preparation or
  rewriting the binding. A retained peer with a required binding that is absent
  blocks instead of preparing or rebinding. The launch path also reads the
  persisted binding from the live runtime row when a predecessor ID is absent,
  and never substitutes current launch policy.
- Retained resource-circuit identity is restored from persisted evidence only;
  current-policy credentials are never assigned to a surviving process, and
  missing evidence leaves the resource unknown.
- Uncertain agent states (`error`, `stopping`, unknown) block recovery instead
  of being treated as stopped. Successful delivery descriptors must carry the
  requested session, stream, incarnation, and generation evidence.
- A detached failed adoption remains stop-capable: `StopBySessionID` and
  `GetExecutionIDForSession` resolve the live standalone runtime row, and an
  exact persisted standalone stop path tears down the peer and deletes the row.
- Empty retained workspace allowlists and environments remain authoritative for
  adoption. Missing required roots block when the replacement request declares
  roots; request overrides are never substituted for retained evidence. The
  control inventory always serializes empty collections so empty and unavailable
  are distinguishable.

Tests and verification (from the repository root):
- `go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestStandaloneReconnect|TestStandaloneRetainedOwner|TestStandaloneRetainedAdoption|TestStandaloneRetainedJournal|TestAdoptedStandalone|TestNewWorkspaceHelper|TestReuseExisting|TestDockerReconnect|TestToAgentExecution)' -count=1` passed.
- `go test -trimpath -race -tags fts5 ./internal/agentctl/server/instance -run '^TestCreateInstance(RejectsUnavailableJournal|TeardownReleasesJournal)$' -count=1` passed.
- `go test -trimpath -tags fts5 ./internal/task/service -run '^TestGetWorkspaceInfoForSession' -count=1` passed.
- Full `./internal/agent/runtime/lifecycle` package passed (`go test -trimpath -tags fts5`, final run 192.2s).
- `go build -trimpath -tags fts5 ./...` passed.
- `bash scripts/lint-go-changed` over all changed and untracked Go files: 0 issues.
- `git diff --check`: clean.
- RED evidence is the recorded real-lock `lock_timeout` in
  [the reproduction record](reproduction.md); the permanent reconnect tests
  were added with the correction and exercise a real bbolt handle.
- New regressions cover a real executor-running row plus captured events for
  nonterminal adoption detach and persisted Stop reachability, a workspace
  restore through `GetOrEnsureExecution` -> `Launch` -> `StartAgentProcess`
  with GET-only built-in verification, exact persisted-binding restoration,
  blocked missing binding, launch binding lookup without a predecessor ID,
  current-policy binding rejection, retained resource identity, uncertain
  status rejection, incomplete delivery descriptors, and empty/nil retained
  roots and environments over the control JSON round trip.
