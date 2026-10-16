---
id: "03-recovery-and-lifecycle"
title: "Complete recovery and lifecycle parity"
status: done
wave: 3
depends_on:
  - "02-remote-launch-and-terminal"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-PASSTHROUGH-001
  - REQ-EXECUTORS-SURVIVAL-005
acceptance_criteria:
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.4
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.6
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.7
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.8
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.9
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.10
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.11
  - AC-EXECUTORS-SURVIVAL-005.3
system_design:
  - ../../specs/executors/system-design/ssh-passthrough.md
---

# Task 03: Complete recovery and lifecycle parity

## Summary

Route all passthrough lifecycle operations through the selected transport.
Recover retained SSH processes by authenticated identity, and preserve existing
local passthrough survival behavior.

## In scope

- Replace runner assumptions in `EnsurePassthroughExecution`, resume, fallback,
  stdin, resize, buffer, automatic injection, model relaunch, reset, and Stop.
- Correlate remote events with their captured execution and process generation.
  Duplicate and stale events must not run completion consumers.
- Persist allowlisted process/start/submission identifiers before relevant
  remote operations. Keep token values exclusively in SecretStore.
- Inspect passthrough snapshots before an ACP-only status result classifies a
  retained remote CLI as absent. Reattach the existing CLI without provider load.
- Recreate only after confirmed absence. Preserve workspace, session settings,
  selected profile, and provider resume flags. Unknown liveness fails closed.
- Graceful shutdown, failed launch rollback, terminal cleanup, and orphan sweep
  maintain ownership until the remote process tree stops or failure is retained.
- Keep local standalone guard/recovery exclusions and current auth selection.

## Out of scope

Automatic survival re-tracking for local passthrough, ACP journal semantics,
and a new provider-conversation format.

## Acceptance

1. `TestSSHPassthroughResumeRetainedProcess` proves the same remote PID and
   conversation after explicit resume. Separate tests prove confirmed absent
   CLI, confirmed absent controller, absent secret, failed secret reveal,
   unknown liveness, old helper, and mismatched generation never create duplicates.
2. `TestRemotePassthroughStaleEvents`, `TestRemotePassthroughInitialPromptBoundary`,
   and `TestRemotePassthroughLostPromptResponse` prove correlated completion and
   no repeated initial prompt after a lost response or backend interruption.
   A mixed inventory containing one live and one failed sibling preserves the live owner.
3. `TestRemotePassthroughResetAndModelChange` and Stop/shutdown regressions prove
   replacement occurs only after termination of the observed owner. Local
   passthrough regressions and all six `TestResolveLaunchAuthToken` cases pass.

## Verification

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test.*Passthrough|TestSSH.*Resume|TestResolveLaunchAuthToken|Test.*Passthrough.*Scope' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/gateway/websocket -run 'Test.*Passthrough|Test.*Remote.*Terminal' -count=1)
git diff --check
```

Run changed-file Go lint with exact paths. Record the command and result.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go`,
  `manager_execution.go`, `manager_startup.go`, `manager_interaction.go`,
  `manager_lifecycle.go`, and `manager_events.go` through focused helpers.
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh.go`,
  `executor_ssh_recovery.go`, and `executor_ssh_stop_resume_test.go` where
  retained controller status needs passthrough-aware inspection.
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go` for
  metadata allowlists and typed remote descriptors.
- `apps/backend/internal/agent/runtime/lifecycle/manager_ssh_passthrough_recovery_test.go`
  and `manager_ssh_passthrough_lifecycle_test.go` (new).
- `apps/backend/internal/agent/runtime/lifecycle/passthrough_transport.go`,
  Task 01's client APIs, and gateway remote-terminal tests for complete wiring.

## Dependencies

Task 02's transport selection, fresh launch, configuration claims, and streams.

## Risks

An alive controller with no ACP adapter is not evidence that its PTY is absent.
A missing token is not permission to replace a controller. A delayed callback
from a replaced process cannot consult mutable identity and act on its successor.
Inspect every write, cleanup, and reset path before declaring parity complete.

## Parallelism

`sequential`

## Inputs

- [Recovery and persistence design](../../specs/executors/system-design/ssh-passthrough.md#recovery-and-persistence).
- [Standalone survival scope](../../specs/executors/requirements/agent-survival-session-state.md).
- Existing SSH stop/resume/liveness/instance-identity tests, local initial-prompt
  tests, and `manager_auth_test.go`.
- Existing `remoteInstanceLifecycleMu`, `passthroughLifecycleMu`, session
  admission, and stream callback generation checks.

## Results

Authenticated SSH PTY snapshots now precede ACP status decisions. Retained
processes keep their PID, session, workspace, and prompt boundary. Missing
credentials, unknown liveness, missing start evidence, conflicting identity,
and unsupported helpers cannot admit a duplicate CLI. Lost launch and prompt
responses use the same operation or submission receipt. Stale events are
filtered, accepted cursors persist, and replay gaps require a fresh authenticated
snapshot without a synthesized completion.

Tests cover first-launch terminal admission, repeated first-launch callers,
failed reset ownership, fallback prompt persistence, initial submission
uncertainty, confirmed receipts, environment filtering, and safe launch details.
Browser evidence confirms the same physical remote PID after backend SIGKILL.
Existing local passthrough and six auth-token cases remain passing.

The work-order lifecycle race command passed after the final behavior changes.
The final helper extraction also passed focused lifecycle race checks:
`GOCACHE=/tmp/kandev-ssh-go-cache.7r9WV7 go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestSSHPassthrough.*|TestSSHMockPassthrough.*|Test.*Passthrough.*Initial|TestAttemptResumeFallbackClaimsFreshInitialPrompt' -count=1`.
Gateway race checks and changed Go lint passed with zero issues. Task 04 owns
final desktop and phone browser evidence.

Browser Stop/Resume exposed a nested client lease in stale cleanup. The cleanup
held the write lease while remote Stop acquired a read lease on the same client.
The browser stack proves the deadlock. The new regression timed out at the
same read lease before the fix. Remote Stop now precedes the final client
write lease. The new regression and existing stale/retired cleanup tests pass
(0.116 seconds).

The next browser run exposed ACP delivery acknowledgement on explicit
passthrough Resume. `TestNativeResumePassthroughDoesNotRequireACPDelivery`
failed before the fix. Passthrough now skips ACP conversation acknowledgement
and rejects unresolved SQL delivery history without retiring it. The expanded
race command passed in 56.455 seconds:

```bash
GOCACHE=/home/zeval/.cache/kandev-ssh-ee8214d6-go go test -C apps/backend -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestNativeResume|Test.*Passthrough|TestSSH.*Resume|TestResolveLaunchAuthToken|Test.*CleanupStale|Test.*Retired' -count=1
```

Changed-package Go lint passed with zero issues after the fix. A prior run
timed out while loading cold dependencies; the completed run is authoritative.
Task 04 owns final SSH browser revalidation.

The next Stop/Resume run proved two further defects. Stop removed the runtime
row used to infer conversation resume, and terminal attachment independently
created a controller during the admitted Resume. The new attach and request
regressions failed before the changes. `ResumePassthrough` now carries provider
resume intent through the executor adapter independently of runtime ownership.
An SSH attachment with no admitted execution returns workspace-not-ready and
waits for launch or Resume to register the owner. Standalone lazy creation keeps
its existing path. Sibling browser tests explicitly select the session tab and
ignore hidden terminal panels.

The expanded lifecycle, executor, and backend adapter race command passed:

```bash
GOCACHE=/home/zeval/.cache/kandev-ssh-ee8214d6-go go test -C apps/backend -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor ./internal/backendapp -run 'TestNativeResume|Test.*Passthrough|TestSSH.*Resume|TestResolveLaunchAuthToken|Test.*CleanupStale|Test.*Retired|TestBuildLifecycleLaunchRequest|TestLaunch_Promotes' -count=1
```

Results: lifecycle 39.872 seconds, executor 1.586 seconds, adapter 1.493 seconds.

Final changed-package lint passed with zero issues. The managed SSH browser
suite passed all 14 tests, including desktop and phone Stop/Resume, after these
changes. No provider conversation resume is inferred from an invented runtime
owner, and attachment no longer creates a competing SSH controller.


PR review regressions reproduced delayed startup-idle completion, output queue
closure without browser reconnect, config writes surviving a failed registry
save, and receipt limits accumulating across generations. The fix persists the
initial-submission event boundary, disconnects overflowed streams, rolls back
failed preparation, and bounds current-generation receipts without replaying
expired input. Focused controller, API, client, config, and lifecycle tests pass
with race detection. Additional regressions preserve pending receipts, the
expiry floor, process replacement, and external config replacements.

The follow-up review regressions also cover observer target release and verified
exit projection after a replay gap. Idle snapshots cannot fabricate completion,
and a failed remote probe preserves process ownership. The first-launch test
now uses the synchronized memory event bus after race detection exposed
concurrent writes to its test-only mock. All six packages passed the final
review-remediation race command:

```bash
GOCACHE=/home/zeval/.cache/kandev-ssh-ee8214d6-go go test -C apps/backend -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/common/passthroughconfig ./internal/gateway/websocket -run 'Test.*Passthrough|TestPrepare|TestWaitForFirstIdle|TestEmitTurnComplete|TestHandleTerminal|TestBridge' -count=1
```


CI remediation reproduced the read-only Resume timeout in the exact CI merge
checkout. The shard's original partition ran in the CI runtime image with one
worker, two CPUs, and 4 GiB memory. A goroutine capture showed stale-execution
cleanup waiting on the client lease held by readiness polling. Readiness now
holds the lease only during a health request, bounds each request to one
second, and ends quietly when cleanup detaches the client. Both unavailable
and unresponsive health checks failed the regression before the change. The
regressions and client-lease checks pass with race detection. The full lifecycle
package also passed with race detection in 303.752 seconds. A helper resolver
test now uses empty temporary directories so built remote helpers cannot
invalidate its missing-binary premise. Current-head CI remains pending until
the remediation is pushed and its checks finish.


A Windows CI auto-shell regression exceeded its two-second deadline while
starting native processes. The test now calls auto-shell setup under the same
start lock with a deliberately invalid Git environment. This reaches the
snapshot boundary and exits before shell spawning. Reintroducing the original
recursive lock in an isolated worktree makes the test fail in two seconds.
The corrected test and adjacent environment/shell tests pass with race detection.
Windows native execution remains an external CI gate until the new head passes.


The second Greptile review reproduced cleanup waiting on the source guard while
an event cursor save was pending. Remote event projection and gap reconciliation
now run without that guard. Captured execution, controller, process, generation,
and sequence checks continue to reject stale events. A WebSocket regression
checks that a failed persistence read releases its guards without holding the
client source guard. Event state writes and cursor saves use the runtime teardown guard and revalidate
the execution and target before writing. Real SQLite regressions race successful
and not-found reads with cleanup and replacement, preserving retired state and
the successor row. Separate positive and negative cases protect identity checks.
Focused lifecycle passthrough tests passed with race detection in 63.283 seconds.

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestSSHPassthrough|TestSSHMockPassthrough|TestPassthrough' -count=1)
```


The third review reproduced late cursor upserts with successful and not-found
reads against SQLite. Twelve cases cover started, idle, and submitted events across retirement and
replacement. A separate
regression changes execution, controller, generation, or cursor while the save
waits for its runtime guard. The current owner remains writable; stale owners
perform no write. These cases and the failed-read cursor rollback case pass
with race detection.

All six affected backend packages have passing complete race results. The
combined run passed lifecycle (238.552s), process (359.294s), API (148.365s),
client (6.838s), and config (1.074s). A gateway terminal marker hit its
three-second deadline once. Its isolated case and complete package passed
unchanged; the complete gateway repeat took 9.108s. SQLguard passed, and the
store-conformance race suite passed in 78.630s.

```bash
(cd apps/backend && GOMEMLIMIT=3GiB GOMAXPROCS=2 GOFLAGS=-p=2 go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/common/passthroughconfig ./internal/gateway/websocket -count=1)
(cd apps/backend && GOMEMLIMIT=3GiB GOMAXPROCS=2 GOFLAGS=-p=2 go test -trimpath -race -tags fts5 ./internal/gateway/websocket -count=1)
(cd apps/backend && go run -trimpath ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/persistence/storeconformance -count=1)
```


Expanding the same retirement matrix to idle and submitted events reproduced
eight additional state-write races. Their state writes now share the runtime
guard with cursor persistence. Ready and running payloads are captured during
preparation and published after all guards are released. A synchronous event
subscriber successfully binds a resume attempt and performs runtime cleanup.
The publication regression passed with race detection in 1.169 seconds.

After this change, the complete lifecycle race suite passed in 239.985 seconds,
the complete gateway race suite passed in 10.045 seconds, and SQLguard passed.
The other four package inputs and the store-conformance inputs did not change.

Resetting Cursor or Pi with changed profile MCP settings reproduced HTTP 400
from the previous generation's own claim. A second API regression reproduced
the same rejection after a confirmed Stop, including after a sibling released
its claim. Reset now resolves credentials before Stop, then prepares the new
configuration. The controller retires only the stopped current generation's
claim. Pending preparation and sibling claims remain protected.

Stale preparation reproduced an incorrect HTTP 200 and is now rejected with
HTTP 409. A failed preparation after Stop also reproduced a READY execution
with no CLI; that path now records FAILED. Regression coverage includes Cursor,
Pi, sibling conflicts, stale cleanup, profile-secret failure, and failed Stop.
The complete process race suite passed in 393.483 seconds and the API race
suite passed in 175.900 seconds. After preserving the local reset preparation
order, the final complete lifecycle race suite passed in 233.242 seconds.

```bash
(cd apps/backend && GOMEMLIMIT=3GiB GOMAXPROCS=2 GOFLAGS=-p=2 go test -trimpath -race -tags fts5 ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/lifecycle -count=1)
(cd apps/backend && GOMEMLIMIT=3GiB GOMAXPROCS=2 GOFLAGS=-p=2 go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -count=1)
```
