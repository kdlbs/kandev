---
id: "02-settle-stopped-agent"
title: "Settle the guard for an exact stopped agent"
status: done
wave: 2
depends_on:
  - "01-preserve-remote-docker-workspace"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
  - REQ-EXECUTORS-PLUGIN-004
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
acceptance_criteria:
  - AC-EXECUTORS-REMOTE-DOCKER-001.12
  - AC-EXECUTORS-REMOTE-DOCKER-001.18
  - AC-EXECUTORS-PLUGIN-004.1
  - AC-EXECUTORS-PLUGIN-004.6
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.1
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.2
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.6
system_design:
  - ../../specs/executors/system-design/remote-docker-executor.md
  - ../../specs/executors/system-design/remote-executor-plugins.md
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
  - ../../specs/platform/system-design/durable-agent-reattachment.md
---

# Task 02 Settle the guard for an exact stopped agent

## Summary

Use authenticated stopped-agent evidence to let a retained Remote Docker or plugin workspace reach ordinary Resume.
The startup pass and periodic retry must record the same outcome.

## In scope

- Add the authenticated `stopped` result from the
  [session proof](../../specs/platform/system-design/durable-restart-reconciliation.md#stopped-agent-in-a-retained-remote-environment).
  Keep the running-agent proof in use for live adoption.
- Use the existing retained replay path with lifecycle callbacks suppressed.
  Persist proven outcomes through the durable projector. Use existing
  interrupted-delivery reconciliation for an exact abandoned dispatch.
  Use the existing delivery fence for any separate live or unverifiable work.
- After replay, settle only the original turn. Use exact-turn abandonment
  for an unknown outcome. Reuse the owner-locked runtime repair and the
  normal session state publication. The stopped write and active session
  transition must survive restart together.
- Close the attempt transport before releasing the matching startup guard.
  Ordinary Resume owns agent startup, native restoration, and queue admission.

## Acceptance

1. An exact stopped agent settles during startup or retry. A matching
   active session reaches `WAITING_FOR_INPUT` and emits `session.state_changed`.
   Settlement publishes no agent readiness and triggers no launch or queue drain.
2. Replay projects the retained tail once. A matching terminal settles the
   original submission. With an open turn and an exact abandoned dispatch,
   settlement closes that turn as interrupted and preserves its unknown
   outcome. Explicit Resume loads the same native conversation and retires
   the matching interruption without resending the old prompt.
3. Cover rejected auth, wrong instance or session, conflicting native ID,
   wrong incarnation or generation, and mismatched stream or payload.
   Replay failures and stale owners keep the applicable protection.
   A valid idle `stopped` response with no native ID can settle.
4. A live agent still adopts. An unrelated guarded session stays protected.
   Repeated recovery keeps the same resource, including after interrupted
   replay and a plugin checkpoint refresh.
5. A failed tokenless Remote Docker session may settle runtime ownership when
   the exact recorded container is definitively exited. A missing agentctl
   instance inside a running container remains ambiguous, and a plugin without
   authenticated stopped-agent evidence remains guarded. Preserve the failed
   session state and independent journal fences.

## Regression and verification

Add `TestRemoteRecoverySettlesStoppedRetainedAgent` with both runtimes.
Use the authenticated fixtures in `remote_recovery_replay_test.go`.
For plugins, reuse the provider fixtures from
`TestPluginExecutorRestartRecovery` in `executor_plugin_recovery_test.go`.
Let inspection first report unavailable, then return the same resource on
periodic retry. Cover a refreshed checkpoint for the same resource and a
changed checkpoint for a different resource. Keep the stopped-plugin reopen
test as the control for resuming that exact resource.
Use a stopped case with one queued instruction and check that settlement
leaves its queue entry in place before Resume.

Add real-repository lifecycle cases for retained `failed`, `cancelled`, and
`completed` terminal submissions. Verify each matching projected terminal
effect settles the original open turn without creating recovery metadata,
starting an agent, or draining queued work. For an interrupted dispatch whose
notice was not written before the crash, verify exact owner and payload checks
persist the recovery notice and Resume block from the retained submission and
saved turn binding. Resume must retain the native conversation and must not
submit the prompt again. Include a nonterminal tool message and verify exact
turn settlement marks it terminal. Pause the owner-CAS after its SQL commit,
request Stop before transport closure, and prove Stop retains the exact row
and completes the stop.

Extend the executor regression to distinguish a definitively `exited` recorded
container from ambiguous `created`, `paused`, `restarting`, `removing`, `dead`,
and unknown states. Only the exited exact container can release the tokenless
failed-session guard. A missing instance inside a running container and every
ambiguous state remain guarded. Keep independent journal fences in place.

In the E2E auth helper, resolve only the fixture's agentctl and container
control secret IDs. Send tokens through stdin and suppress subprocess output
that could include token-bearing command data.

Add two cases to `remote-docker-recovery.spec.ts`. Both use
`prepareRecoverableTask` to create a disposable container, an unpushed
commit, and an untracked marker. Stop and restart only the fixture backend
using the existing `backend.pid()` and `backend.restart()` pattern.

- **Failed session without a resume token keeps its Remote Docker workspace.**
  While the backend is down, use `withDatabase` to set the fixture session
  to `FAILED` and clear the runtime row's token and resumable flag. Keep its
  recorded container and target. After restart, check the original container
  ID, HEAD, and untracked file. Use the existing `requestSessionResume` helper
  to verify that launch reaches that workspace even without native restore.
- **Failed tokenless session with an exited recorded container releases the
  startup guard.** Stop the exact fixture container while the backend is down,
  then verify startup retains its environment record and the session remains
  failed after the container exits.
- **Stopped agent resumes after restart in the same Remote Docker workspace.**
  Start the mock agent's `/slow 30` command and wait for its initial output.
  Once the test backend is down, use `docker exec` in the fixture container to
  kill only the exact mock-agent PID from agentctl's `/api/v1/status` response.
  Verify that the PID belongs to the fixture mock agent and wait for
  `/api/v1/agent/session` to report `stopped` before restarting the backend.
  Check that the journal has a tail beyond the SQL cursor. After restart,
  verify that the tail projects once and the session waits with the agent
  stopped. Resume must reach the original container and native session.

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestRemoteRecovery(SettlesStoppedRetainedAgent|SettlesRetainedTerminalOutcome|CreatesInterruptedNoticeWhenMetadataWasNotWritten|ConfirmedAbsenceSettlesGuard|RetriesAfterPartialReplayTimeout)|TestStopClaimsStoppedRemoteSettlementAfterSQLCommit|TestRemoteRecoveryFailedTokenlessDocker|TestRemoteDockerRecovery(KeepsAmbiguousContainerStatesGuarded|DoesNotStartStoppedContainer)|TestPluginExecutorRestartRecovery|TestManagerReopensStoppedPluginExecutorByAttachingRetainedEnvironment)$' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run '^TestNativeResumeAfterLateRecoveryNoticePreservesConversationWithoutPrompt$' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --shards 1 --project containers tests/remote-docker/remote-docker-recovery.spec.ts -- --grep 'failed session without a resume token|failed tokenless session releases recovery after its recorded container exits|stopped agent resumes after restart|same-session resume keeps the container' --retries=0)
```

## Likely files

- `apps/backend/internal/agent/runtime/lifecycle/remote_recovery.go`
- `apps/backend/internal/agent/runtime/lifecycle/remote_recovery_evidence.go`
- `apps/backend/internal/agent/runtime/lifecycle/remote_recovery_disposition.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_remote_docker_recovery.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_remote_docker_recovery_test.go`
- `apps/backend/internal/task/repository/sqlite/remote_recovery_owner_cas.go`
- `apps/backend/internal/agent/runtime/lifecycle/remote_recovery_stopped_agent_test.go` (new)
- `apps/web/e2e/tests/remote-docker/remote-docker-recovery.spec.ts`
- `docs/public/executors.md`

## Dependencies and risks

Work order 01 protects the workspace and attachment record until proof can run.
After attachment and replay, compare and set on the fresh row's execution ID
and `UpdatedAt`. Under the owner locks, validate the same environment
generation, resource handle, and agentctl identity. A plugin's checkpoint may
advance during this attempt while those identities stay fixed.

## Lifecycle evidence

| Scenario | Acceptance criterion and assertion |
| --- | --- |
| Failed Docker session without a token | Docker `.18` preserves the original container, commit, and untracked file. |
| Tokenless failed Docker session whose recorded container exited | Platform `.1.6` releases only the exact runtime owner while retaining the failed session and workspace. |
| Stopped agent with retained output | Platform `.1` and `.6` project the journal tail and persist the stopped and waiting states. |
| Open turn with no terminal evidence | Platform `.2` and `.6` preserve uncertainty and the queue through same-conversation Resume. |
| Conflicting journal, concurrent Stop, or successor | Platform `.3` allows only the exact current owner to settle. |
| Public Resume and plugin reattachment | Docker `.12`, plugin `.1` and `.6` reuse the original workspace. |

## Results

RED: `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestRemoteDockerRecoveryAttachesStoppedAgentctlForLifecycleSettlement$' -count=1)` failed as expected because the executor returned no instance for an authenticated stopped agentctl.

GREEN:

- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestRemoteRecovery(SettlesStoppedRetainedAgent|ConfirmedAbsenceSettlesGuard|RetriesAfterPartialReplayTimeout)|TestPluginExecutorRestartRecovery|TestManagerReopensStoppedPluginExecutorByAttachingRetainedEnvironment|TestRemoteDockerRecovery(AttachesStoppedAgentctlForLifecycleSettlement|AttachesOnlyTheRecordedLiveContainerAndInstance|DoesNotStartStoppedContainer|DoesNotRecreateMissingAgentctlInstance|AuthFailureWithoutNonceStaysUnknown|RefreshesAuthOnSameSavedInstance))$' -count=1)` passed.
- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestRemoteRecovery(SettlesStoppedRetainedAgent|SettlesRetainedTerminalOutcome|CreatesInterruptedNoticeWhenMetadataWasNotWritten|ConfirmedAbsenceSettlesGuard|RetriesAfterPartialReplayTimeout)|TestStopClaimsStoppedRemoteSettlementAfterSQLCommit|TestRemoteRecoveryFailedTokenlessDocker|TestRemoteDockerRecovery(KeepsAmbiguousContainerStatesGuarded|DoesNotStartStoppedContainer)|TestPluginExecutorRestartRecovery|TestManagerReopensStoppedPluginExecutorByAttachingRetainedEnvironment)$' -count=1)` passed.
- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run '^TestNativeResumeAfterLateRecoveryNoticePreservesConversationWithoutPrompt$' -count=1)` passed.
- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/task/repository/sqlite -run '^(TestSettleStoppedRemoteExecutionIfOwnerCurrent.*|TestPostgresSettleStoppedRemoteExecutionIfOwnerCurrent)$' -count=1)` passed. The PostgreSQL-specific case is environment-gated.
- `KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --no-build --shards 1 --project containers tests/remote-docker/remote-docker-recovery.spec.ts -- --grep 'failed session without a resume token|failed tokenless session releases recovery after its recorded container exits|stopped agent resumes after restart|same-session resume keeps the container' --retries=0` passed all four selected tests after rebuilding the backend and E2E fixtures.
- Changed-package `golangci-lint` with `--new-from-rev=d7d38f8e6f32dd4ceadb7c294416ba9f3ece70af` reported zero issues. SQL guard, persistence store conformance, public documentation validation/tests, backend build targets, and `git diff --check` passed.

The stopped-agent executor regression confirms that authenticated `stopped`
status reaches lifecycle ownership checks without starting or mutating the
container. The E2E assertions verify the same workspace, projected retained
tail, and Resume path. The test reads the projected cursor because replay does
not update `remote_high_water` from the journal descriptor.

The added repository regressions cover retained failed, cancelled, and
completed outcomes; interruption metadata recovery from the exact saved
submission and turn binding; pending tool-message cleanup; Stop arbitration
after the settlement write; and the distinction between an exited recorded
container and ambiguous Docker states. The E2E helper resolves only fixture
credential IDs and passes the selected token through stdin.

### PR review fixup verification

The fixup commit 4920de0e591ad17d87d13f8ea279a6a1f729229c passed the
focused lifecycle, SQLite, and orchestrator regressions on the exact synthetic
merge tree for base 018808f8d9494f0e614648a90861d85e4e1dc857. The tree matched
GitHub's synthetic merge tree ca9fa5150a902fa1a4fb9f40c70a23107a0d9e8b.

- Lifecycle tests passed:
  go test ./internal/agent/runtime/lifecycle -run
  'Test(RemoteRecoverySettles|RemoteRecoveryCreatesInterrupted|RemoteRecoveryAdopts|StoppedRemote|ApplyRemoteLegacy|ValidateRemoteSession|FailedRemote|FailedRecovery|StopClaimsStoppedRemote)' -count=1
- SQLite owner-CAS and delivery-retention tests passed:
  go test ./internal/task/repository/sqlite -run
  'Test(RepairExecutorRunningDeadIfOwnerCurrentValidatesOwnerBeforeRepair|SettleStoppedRemoteExecutionIfOwnerCurrent|ConfirmExecutorRunningAbsentIfOwnerCurrentRejectsReappearedRow|DeliveryRetentionWorkflowOutcome|DeliveryRetentionWorkflowOutcomeWriteFailure|DeliveryRetentionTurnlessTerminalBindsWorkflowTurn)' -count=1
- Native Resume regressions passed:
  go test ./internal/orchestrator -run
  'Test(NativeResumeAfterLateRecoveryNoticePreservesConversationWithoutPrompt|RecoverSessionUnknownPromptKeepsNativeHistory|NativeResumeStartFailureLeavesUnknownDeliveryUnacknowledged|AgentStoppedAfterNativeResumeStartFailureDoesNotCompleteTurn)' -count=1
- TestPostgresSettleStoppedRemoteExecutionIfOwnerCurrentSerializesOwnerChange
  passed against a disposable PostgreSQL 16 database.
- The focused mobile Claude Monitor test passed with retries disabled. The
  downloaded shard 16/28 manifest then passed on the merged tree with the same
  four-process partition and retries disabled: 185 passed, 1 skipped.
- The first current-head E2E attempt failed at the same mobile Monitor card
  assertion as main run 44934819406. The failed current-head jobs were rerun
  once; attempt 2 passed. At fixup head 4920de0, 92 checks passed, 15 were
  skipped, and none failed or remained pending.
