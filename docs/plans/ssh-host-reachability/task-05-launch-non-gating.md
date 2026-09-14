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

# Task 05: Launch-Path Non-Gating and Failure Attribution

## Summary

Keep the reachability record entirely out of the launch decision, and give a
failed SSH launch Kandev's own attribution to the target host. `CreateInstance`
neither reads nor writes a record. The producer write is deferred; see the
requirement document's `## Out of scope`.

## In scope

- `SSHExecutor.CreateInstance` reads no reachability record, and no code path
  consults one to decide whether to proceed, defer, or re-route. With the
  producer write deferred it writes none either, so `lifecycle` takes no new
  dependency on the reachability store.
- The wrapped dial error names the target host and the reason classified from
  **that launch's own** attempt through `ClassifyDialError` — not the
  stored record, which may predate the attempt by a full interval. This is what
  addresses the 2026-09-06 report directly: the user saw an agent-generated
  message naming a firewall when the cause was a stale address.
- The non-interactive half of the pre-launch warning: where no interactive user
  is present, the warning naming the host and the last-success age is recorded
  against the launched session rather than shown as a prompt, and is suppressed
  entirely while probing is disabled. This read happens
  in the launch orchestration that emits the warning, never inside
  `CreateInstance`'s decision path, so it cannot become a gate.
- An explicit negative test asserting `CreateInstance` performs no reachability
  read.

## Out of scope

- Refusing, deferring, queueing, or re-routing a launch on a reachability
  record. REQ-003 requires the opposite, and this task must not add a read path
  that a later change could turn into a gate.
- **Deferred:** writing a record from the launch path at all — `reachable` on a
  successful dial, a classified failure on a failed one. Retired IDs
  `AC-EXECUTORS-SSH-REACHABILITY-003.4` through `003.7` are not reused.
- The interactive pre-launch warning UI. Task 07 owns that half of
  `AC-EXECUTORS-SSH-REACHABILITY-003.2`.
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
- With no interactive user, the warning is recorded against the launched
  session and the launch still proceeds; with probing disabled, nothing is
  recorded and the launch still proceeds.

## Verification

Start with the negative test as a failing test — a store fake that fails the
test if any read method is called — and confirm it passes only because no read
exists, not because a read happened to be skipped on that path. Then run:

```bash
# From apps/backend:
go test -tags fts5 -race ./internal/agent/runtime/lifecycle/ -run 'Reachability|CreateInstance|SSHLaunch'
go test -tags fts5 -race ./internal/task/ -run 'LaunchWarning'
make test-lifecycle-goleak LIFECYCLE_GOLEAK_COUNT=5
make lint
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reachability_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reachability_launch_test.go`

## Dependencies

Task 03. The non-interactive warning reads the stored record for the
last-success age, so the store and its poller-owned writer must exist first.
The dial-error attribution itself needs only task 01's classifier.

## Risks

- This is the sharpest edge in the capability. A future change that reads the
  record to "skip a pointless launch" would silently violate REQ-003 without
  breaking any pre-existing test; the negative test is the only thing standing
  between the contract and that change, so it must assert on a fake that
  *fails* on read rather than merely counting reads.
- The warning path does read the record, so "no read" and "a read for the
  warning" coexist in one flow. Keep the read in the orchestration that emits
  the warning and out of `CreateInstance` entirely, or the negative test above
  becomes unassertable and the gate REQ-003 forbids becomes one refactor away.
- When the deferred producer write lands it must reuse task 03's writer rather
  than opening a second one; leaving no recorder here now is what keeps that
  option open.

## Parallelism

`parallel-safe` with task 04 — disjoint files, both depend only on task 03.

## Inputs

- System design, section *Launch interaction* and the *Control flow* diagram.
- Requirement document `## Out of scope` for the deferred producer write.
- `executor_ssh.go` `CreateInstance` around the `dialSSH` call site.
- Task 01's `ClassifyDialError`.

## Results

Pending.
