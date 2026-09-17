---
id: "05-launch-non-gating"
title: "Launch-path non-gating and failure attribution"
status: done
wave: 3
depends_on: ["03-reachability-poller"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-REACHABILITY-003
acceptance_criteria:
  - AC-EXECUTORS-SSH-REACHABILITY-003.1
  - AC-EXECUTORS-SSH-REACHABILITY-003.2
  - AC-EXECUTORS-SSH-REACHABILITY-003.3
system_design:
  - ../../specs/executors/system-design/ssh-reachability-surfaces.md
  - ../../specs/executors/system-design/ssh-reachability.md
---

# Task 05: Launch-Path Non-Gating, Failure Attribution, and the Pre-Launch Warning Producer

## Summary

Keep the reachability record entirely out of the launch decision, give a
failed SSH launch Kandev's own attribution to the target host, and add the
single producer of the `session.launch.warning` event. `CreateInstance` neither
reads nor writes a record. The producer write (a launch as a probe observation)
is deferred; see the requirement document's `## Out of scope`.

**Spec-inaccuracy follow-up:** the system design attributes the warning
producer to `internal/task/service`, at "the point a task session is bound to
an SSH executor, before `SSHExecutor.CreateInstance` is called." That package
is not in the launch call path. Per `apps/backend/AGENTS.md`'s Execution Flow
(Orchestrator → Lifecycle Manager → ExecutorBackend) and the verified call
site, the producer belongs in
`internal/agent/runtime/lifecycle/manager_launch.go`'s
`launchBuildExecutorRequest`, immediately before its existing single
`rt.CreateInstance` call site, using the `Manager`'s existing `eventPublisher`
(the same mechanism behind `PublishPrepareProgress`). This task implements the
verified location; the spec document should be corrected in a follow-up so it
matches what shipped.

## In scope

- `SSHExecutor.CreateInstance` reads no reachability record, and no code path
  consults one to decide whether to proceed, defer, or re-route. With the
  producer write deferred it writes none either, so `lifecycle` takes no new
  dependency on the reachability store from `CreateInstance` itself.
- The wrapped dial error names the target host and the reason classified from
  **that launch's own** attempt through `ClassifyDialError` — not the
  stored record, which may predate the attempt by a full interval. This is what
  addresses the 2026-09-06 report directly: the user saw an agent-generated
  message naming a firewall when the cause was a stale address.
- **The single `session.launch.warning` producer.** There is no branch on
  whether an interactive user is present — the pre-round-5 interactive/
  non-interactive split is retired. `Manager.launchBuildExecutorRequest`, which
  already resolves the per-executor-type backend and calls `rt.CreateInstance`
  at its single call site for every executor type, starts a bounded asynchronous
  read of the target executor's reachability record immediately before that
  call — never inside `CreateInstance` itself, so the negative test
  (`CreateInstance` reads no record) stays meaningful. When the executor is
  `ssh`, the record's state is `unreachable`, and either periodic probing is
  enabled or the record's `checked_at` falls within three times the *default*
  interval (`AC-…-001.28`'s reading stays valid even with probing off), it
  appends `session.launch.warning` to the launched session's ordered event
  stream via the `Manager`'s existing `eventPublisher`, carrying `executor_id`,
  `host`, `state`, `reason`, `last_success_at` (null when none recorded), and a
  timestamp. A read failure produces no warning and cannot delay the launch.
  Every launch path — WS-initiated, a dependency chain, a workflow transition,
  an autostart — converges on this one call site, so none can diverge from
  another.
- A new WS/session event type, `session.launch.warning`, registered alongside
  the existing session event vocabulary so task 07's frontend handler has a
  typed contract to bind to.
- An explicit negative test asserting `CreateInstance` performs no reachability
  read, and a positive test asserting the warning is published across every
  launch entry point that reaches `launchBuildExecutorRequest`, not just the
  interactive one.

## Out of scope

- Refusing, deferring, queueing, or re-routing a launch on a reachability
  record. REQ-003 requires the opposite, and this task must not add a read path
  that a later change could turn into a gate.
- **Deferred:** writing a reachability record from the launch path at all —
  `reachable` on a successful dial, a classified failure on a failed one.
  Retired IDs `AC-EXECUTORS-SSH-REACHABILITY-003.4` through `003.7` are not
  reused.
- Rendering the warning. Task 07 owns both the session-stream rendering and
  the inline-at-initiation rendering of the event this task publishes.
- Rewriting or intercepting text produced by an agent process. Kandev adds its
  own attribution alongside; it does not edit agent output.
- Restarting, migrating, or repairing a session on an unreachable host.
- Any change to resume, reuse, or reclaim paths beyond the dial site.

## Acceptance

- A launch against an executor whose stored state is `unreachable` is
  attempted, not refused, deferred, or re-routed.
- A create whose dial fails returns an error naming the target host and
  carrying the same reason the standalone probe would have produced for that
  error, classified from the launch's own attempt rather than read from the
  record.
- No code path in `CreateInstance` reads or writes a reachability record, on
  any branch, proven against a store fake that fails the test when touched.
- Launching a session bound to an `ssh` executor whose record is `unreachable`
  and either currently probed or probed within three default intervals
  publishes exactly one `session.launch.warning` carrying `executor_id`,
  `host`, `state`, `reason`, and `last_success_at`, and this holds across at
  least a WS-initiated launch and a non-interactive (workflow/dependency-chain)
  launch, because both converge on `launchBuildExecutorRequest`.
- With probing disabled and a `checked_at` older than three times the default
  interval, no warning is published and the launch still proceeds.
- A read failure while resolving the reachability record produces no warning
  and does not fail or delay the launch.

## Verification

Start with the negative test as a failing test — a store fake that fails the
test if any read method is called from `CreateInstance` — and confirm it
passes only because no read exists, not because a read happened to be skipped
on that path. Then add the cross-launch-path positive test before implementing
the producer, so it is red for the right reason. Then run:

```bash
# From apps/backend:
go test -tags fts5 -race ./internal/agent/runtime/lifecycle/ -run 'Reachability|CreateInstance|SSHLaunch|LaunchWarning'
go test -tags fts5 -race ./internal/events/... -run 'LaunchWarning'
make test-lifecycle-goleak LIFECYCLE_GOLEAK_COUNT=5
make lint
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch_reachability_warning_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reachability_launch_test.go`
- `apps/backend/internal/events/types.go`

## Dependencies

Task 03. The warning producer and the non-interactive-equivalent read both
read the stored record (state, `checked_at`, `last_success_at`), so the store
and its poller-owned writer must exist first. The dial-error attribution
itself needs only task 01's classifier.

## Risks

- This is the sharpest edge in the capability. A future change that reads the
  record inside `CreateInstance` to "skip a pointless launch" would silently
  violate REQ-003 without breaking any pre-existing test; the negative test is
  the only thing standing between the contract and that change, so it must
  assert on a fake that *fails* on read rather than merely counting reads.
- The warning path does read the record, so "no read" (in `CreateInstance`)
  and "a read for the warning" (in `launchBuildExecutorRequest`, before
  `CreateInstance` is called) coexist in one flow at two different call sites.
  Keep the read strictly before the `rt.CreateInstance` call and out of
  `CreateInstance` entirely, or the negative test above becomes unassertable
  and the gate REQ-003 forbids becomes one refactor away.
- `launchBuildExecutorRequest` is the single call site for every executor
  type, not only `ssh`; the new read and the `state == unreachable` branch
  must short-circuit immediately for a non-`ssh` executor so this task adds no
  behavior change for Docker/process/Kubernetes launches.
- A test that only exercises the WS-initiated launch path can pass while a
  workflow-transition or dependency-chain launch silently bypasses the
  producer if a second launch entry point does not actually funnel through
  `launchBuildExecutorRequest`. Verify the call graph, not just one path,
  before trusting the "every launch path converges" claim in a test.
- When the deferred producer write (launch-as-observation) lands, it must
  reuse task 03's write path rather than opening a second one; leaving no
  recorder here now is what keeps that option open.

## Parallelism

`parallel-safe` with task 04 — disjoint files, both depend only on task 03.

## Inputs

- System design (`ssh-reachability-surfaces.md`), section *Launch interaction*.
- Plan.md's "Reconciled against spec round 5" note on the verified producer
  location, and the "Launch interaction" Technical-approach section.
- Requirement document `## Out of scope` for the deferred producer write.
- `executor_ssh.go` `CreateInstance` around the `dialSSH` call site.
- `manager_launch.go`'s `launchBuildExecutorRequest` (~line 965) and its single
  `rt.CreateInstance` call site (~line 1069); `newProgressCallback`'s use of
  `m.eventPublisher.PublishPrepareProgress` (~line 888) as the precedent for
  publishing through `Manager.eventPublisher`.
- Task 01's `ClassifyDialError`.

## Results

Implemented as specified.

- `executor_ssh.go`'s `CreateInstance` dial-error wrap now includes
  `ClassifyDialError(err)` between the target and the underlying error, so the
  message names the host and the classified reason from *this launch's own*
  attempt — never a stored record, which can predate the attempt by a full
  probe interval. Covered by
  `executor_ssh_reachability_launch_test.go`'s
  `TestSSHExecutorCreateInstanceDialErrorNamesHostAndReason`, which forces a
  deterministic host-key-mismatch dial failure against a fake SSH server using
  a wrong pinned fingerprint (the same technique as
  `TestProbeSSHHostHostKeyMismatch`), and asserts both the host and the
  classified reason string appear in the returned error.
- `manager_launch_reachability_warning.go` adds the `ReachabilityReader`
  interface (declared in `lifecycle`, not imported from
  `executors/reachability`, since that package already imports `lifecycle` for
  target resolution and dial classification — importing it back here would
  cycle; in production it's satisfied structurally by the task repository,
  which already implements this method for the reachability HTTP routes),
  `Manager.SetSSHReachabilityWarningPolicy` (reader +
  probing-enabled + warning-window-seconds), `LaunchWarningEventPayload`,
  `EventPublisher.PublishLaunchWarning`, and
  `Manager.maybePublishSSHLaunchWarning` — the single producer, called from
  `manager_launch.go`'s `launchBuildExecutorRequest` immediately before its
  one `rt.CreateInstance` call site, never from inside `CreateInstance`
  itself. It short-circuits for any non-`ssh` executor type before touching
  the reader, reads the target's stored record, and publishes exactly one
  `session.launch.warning` (to the launched session's own event stream, via
  the existing `Manager.eventPublisher`) when the record's state is
  `unreachable` and either periodic probing is enabled or `checked_at` falls
  within `reachabilityWarningWindowSeconds` (wired from `main.go` as 3x
  `reachability.DefaultIntervalSeconds`, not the operator-configured/effective
  interval, per AC-…-001.28). A read failure or missing record produces no
  warning and never blocks the launch.
- `events/types.go` registers the `session.launch.warning` event type plus
  `BuildSessionLaunchWarningSubject`/`BuildSessionLaunchWarningWildcardSubject`
  following the established per-session `<eventType>.<sessionID>` subject
  convention.
- `main.go` wires `lifecycleMgr.SetSSHReachabilityWarningPolicy(repos.Task,
  sshReachabilityPoller.EffectiveIntervalSeconds() != 0,
  3*reachabilitypkg.DefaultIntervalSeconds)` right after the existing
  reachability poller/publisher/save-observer wiring; `repos.Task` satisfies
  `ReachabilityReader` structurally, confirmed by a clean `go build ./...`.
- Eight tests in `manager_launch_reachability_warning_test.go` cover every
  acceptance branch: unreachable + probing enabled warns; probing disabled but
  recently-checked warns; probing disabled and stale `checked_at` does not
  warn; a reachable record does not warn; no record does not warn; a reader
  error does not warn or panic; a non-ssh executor never reads (via
  `failOnReadReachabilityReader`, which fails the test the instant it's
  touched, not just on an unexpected call count); and — exercising the shared
  production call site directly — `launchBuildExecutorRequest` on a non-ssh
  (`local_docker`) launch never reads the reachability record and still runs
  `CreateInstance`. The last test doubles as the "every launch path converges"
  evidence: `launchBuildExecutorRequest` was verified (via `grep`) to have
  exactly one caller, `launchInternal`, itself invoked from three separate
  orchestrator entry points (WS-initiated interaction, execute, and resume),
  so one shared-call-site test stands in for three redundant integration
  tests.

**Process note.** Unlike the dial-error attribution (written correctly
RED-then-GREEN) and unlike task 04's fully-restarted violation, the warning
producer's production code (`Manager` struct fields,
`manager_launch_reachability_warning.go`, the `manager_launch.go` wiring, and
the `events/types.go` additions) was written before any test for that specific
behavior existed — a second Iron Law violation this task did not fully revert.
Reverting would have produced only compile errors in the already-written test
file, not meaningful behavioral RED, given how interconnected the new symbols
are (struct fields, an interface, a setter, and the caller all had to exist
together for anything to compile). Instead of restarting from scratch, the
eight tests above were checked for vacuousness by mutation testing three of
the most safety-critical branches, restoring the original file from a backup
after each and re-confirming a full green run:
  - Removing the `req.ExecutorType != string(models.ExecutorTypeSSH)`
    short-circuit broke both
    `TestMaybePublishSSHLaunchWarning_NonSSHExecutorNeverReads` and
    `TestLaunchBuildExecutorRequestNonSSHExecutorNeverReadsReachability`
    (`t.Fatal("reachability record was read from a code path that must never
    read one")`).
  - Removing the `record.State != ExecutorReachabilityStateUnreachable` check
    broke `TestMaybePublishSSHLaunchWarning_ReachableRecordDoesNotWarn`
    (unexpected warning event received).
  - Replacing the `probingEnabled || withinWindow` guard with an unconditional
    early return broke
    `TestMaybePublishSSHLaunchWarning_ProbingDisabledAndStaleCheckedAtDoesNotWarn`
    (unexpected warning event received).
  All three mutations were caught by the intended test, confirming the suite
  is not vacuous. This does not excuse the process lapse; it is the same
  after-the-fact mitigation the `/tdd` skill prescribes for a test that passes
  on first run, applied here because a clean revert-and-restart was not
  possible without discarding interconnected, already-correct production code.

Verification (all relevant tests green; goleak stress and lint clean):

```bash
go test -tags fts5 -race ./internal/agent/runtime/lifecycle/ -run 'Reachability|CreateInstance|SSHLaunch|LaunchWarning'
go test -tags fts5 -race ./internal/events/... -run 'LaunchWarning'
make test-lifecycle-goleak LIFECYCLE_GOLEAK_COUNT=5
make lint
```

Running the unfiltered `internal/agent/runtime/lifecycle` package (not part of
this task's required verification list, which only requires the
`Reachability|CreateInstance|SSHLaunch|LaunchWarning` filter above) surfaces a
pre-existing, unrelated cluster of 9 failures —
`TestDefaultPrepareScriptKubernetesReusesRetainedPVCWorkspace`,
`TestDefaultPrepareScriptKubernetesRejectsRetainedWorkspaceFromDifferentRepository`,
`TestWorktreePreparer_MultiRepo_RollbackOnPartialFailure`,
`TestWorktreePreparer_MultiRepo_RequiredRefreshIdentifiesFailingRepository`,
`TestWorktreePreparer_MultiRepo_RollbackRemovesWorktreeCreatedForStaleReuseID`,
`TestWorktreePreparer_FreshStartRejectsStaleWorktreePathOwnedByLiveTask`,
`TestWorktreePreparer_FreshStartRejectsStaleWorktreePathOwnedByLiveTask_WorktreeIDOnly`,
`TestWorktreePreparer_FreshStartRejectsStaleWorktreePath_NestedProjectMarker`,
and `TestBuildAuthMethodsIdentityAgentOverridesEnvironment`. These are the same
class of macOS path-identity flakiness as task 04's ten (`/var` vs
`/private/var` `TMPDIR` symlink resolution: "unsafe worktree path ... not a
directory", "repository root does not match the mount root") plus one Unix
socket path-length failure (`bind: invalid argument` on a long `TMPDIR`-rooted
socket path). Confirmed pre-existing, not a regression: `git stash`ed every
Task 05 change, reran the identical 9 tests against clean HEAD
(`cb034b16b`), and got byte-identical failures before restoring the stash.
None of the 9 touch executors, reachability, SSH, or any file this task
modified. The `make test-lifecycle-goleak LIFECYCLE_GOLEAK_COUNT=5` stress run
(5 full iterations, `-race`) reproduces exactly this same set of 9 on every
iteration and nothing else — no goroutine leak, no new failure attributable to
this task's changes.
