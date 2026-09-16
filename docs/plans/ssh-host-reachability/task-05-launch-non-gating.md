---
id: "05-launch-non-gating"
title: "Launch-path non-gating and failure attribution"
status: pending
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
  at its single call site for every executor type, reads the target executor's
  reachability record through a narrow read-only accessor *immediately before*
  that call — never inside `CreateInstance` itself, so the negative test
  (`CreateInstance` reads no record) stays meaningful. When the executor is
  `ssh`, the record's state is `unreachable`, and either periodic probing is
  enabled or the record's `checked_at` falls within three times the *default*
  interval (`AC-…-001.28`'s reading stays valid even with probing off), it
  publishes `session.launch.warning` to the launched session's own event
  stream via the `Manager`'s existing `eventPublisher`, carrying `executor_id`,
  `host`, `state`, `reason`, and `last_success_at` (null when none recorded). A
  read failure produces no warning rather than blocking or retrying the
  launch. Every launch path — WS-initiated, a dependency chain, a workflow
  transition, an autostart — converges on this one call site, so none can
  diverge from another.
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

Pending.
