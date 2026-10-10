---
id: "06-pr-delivery"
title: "Deliver silent recovery and clear PR checks"
status: in_progress
wave: 6
depends_on:
  - "05-silent-restart-recovery"
  - "07-long-outage-retention"
  - "08-remote-agent-lifetime"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.20
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.21
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.22
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
system_design:
  - ../../specs/platform/system-design/durable-agent-reattachment.md
---

# Task 06: Deliver silent recovery and clear PR checks

## Summary

Deliver Tasks 05, 07, and 08 on PR 4380 and resolve every current-head CI or review failure.
Keep the live description and screenshots consistent with the revised behavior.

## Scope and ownership

Own PR delivery, conflict resolutions, CI remediation, screenshots, and package result synchronization.
Use the commit, push, PR, and PR-fixup skills for their respective operations.
Do not merge the PR or mutate the user's live sessions.

## Historical evidence for previously pushed revisions

The following snapshots concern earlier pushed heads and are retained as remediation history. They
do not describe the current local source or a green PR. The later terminal waiter result and
subsequent local gates are recorded under Results.

The revised implementation passed normal commit hooks before rebasing onto current main.
The rebase reconciled upstream closed-journal transaction helpers with physical-capacity reservations and retained both sets of shutdown tests.
The frontend recovery-service conflict preserves upstream native-resume behavior and the revised recovery tests.
The six-package backend race gate, six desktop browser cases, and both SSH scenarios passed before the second base advance. The second rebase completed onto `43f55a6aad38c7f53c4b7eb56682016639f74405` at `1135b58f77913035aebf2b35fefbf3fee2d595ab`. It preserves main's same-generation acknowledgment for already interrupted-unknown submissions, idle-dispatch guard, and requirement identities alongside physical-capacity accounting, exact ownership, and payload retention. Final post-rebase browser gates passed four phone, three focused desktop, and two real SSH cases with retries disabled. The merged frontend recovery contracts passed 358 tests and type checking. Journal and process package race suites, focused lifecycle adoption/native Resume checks, and the SQLite/PostgreSQL silent-restore identity matrix passed. Commit `78065a90aa8f25aaaed4486225ee2b689b636d53` passed normal hooks and was pushed with an explicit lease; the local branch, remote branch, and PR head match. The catalog validated 370 decisions and 1,520 specifications, and full specification lint passed.

The then-current CI run `38066667357` failed the `PR documentation coverage` check. The trusted validator at base `43f55a6aad38c7f53c4b7eb56682016639f74405` identified missing plan links for requirements 002 and 004 and a missing Task 08 design reference for requirement 002. The plan and Task 08 now reference the existing durable-agent-delivery design and requirements. The same trusted validator's local preflight reports `covered` with no errors. The later bounded waiter result below supersedes the pending status recorded at that time.

The first remote snapshot listed these failed jobs in E2E run `37988372921`:

- Containers 2/6: job `114037186865`; Docker image build failed in `fixtures/docker-probe.ts:73`.
- Containers 3/6: job `114037186903`; failure cause not yet inspected.
- Containers 6/6: job `114037186859`; failure cause not yet inspected.
- Browser 3/14: job `114037187338`; failure cause not yet inspected.

Other jobs were pending. These are historical inputs, not results for the future implementation head.
The review snapshot had no unresolved threads; refresh it after every push.

## Acceptance

1. The revised implementation passes Task 05's targeted checks on the current base, with all conflicts resolved semantically.
2. Local HEAD, remote branch, and PR head match. Current-head required checks pass, with no pending or failed checks or unresolved actionable reviews.
3. The PR description, new desktop/phone captures, and package results describe silent restoration and its verified limits.

## Procedure and verification

1. Read fresh PR and branch state before committing or pushing.
2. Fetch current `main`; reconcile any base advance and rerun affected Task 05 checks.
3. Reproduce each current leaf failure with retries disabled before changing its owning code or fixture.
4. Commit through normal hooks; use an explicit force-with-lease after rebase.
5. Replace affected old screenshots on a new immutable media ref. Read back the live PR body after editing it.
6. Wait for checks through `scripts/pr-await`, then disposition current review evidence through PR-fixup.
7. Update work-order results without presenting older green runs as current-head evidence.

```bash
scripts/pr-state --compact 4380
scripts/pr-resolve list 4380
git ls-remote origin refs/heads/feature/investigate-interrup-ef7 refs/heads/main
scripts/pr-await 4380
scripts/pr-state --summary 4380
scripts/pr-resolve list 4380
git status --short
git rev-parse HEAD
git rev-parse '@{upstream}'
git diff --check
```

Capture and inspect each helper's exit status. Preserve every long-running command handle.
If the remote branch changed since the captured lease, inspect that change before pushing.
Human approval or merge queue requirements remain separate from green CI.

## Results

Implementation is committed and rebased. Commit `78065a90aa8f25aaaed4486225ee2b689b636d53` is the current pushed source head on base `43f55a6aad38c7f53c4b7eb56682016639f74405`. Normal commit hooks passed, and the explicit force-with-lease push succeeded. Fresh desktop and phone captures are published on immutable media commit `db7888200de8fdb138a92bba0a5e63b864f38b70`; the live PR body was read back after updating its narrative and screenshots.

The complete PR snapshot for head `78065a90aa8f25aaaed4486225ee2b689b636d53` reported run `38066667357`, attempt 1, job `114255591383` (`Publish PR documentation coverage status`) failed with `Linked delivery documentation is incomplete or invalid.` The run was terminal. I reproduced the exact evaluator from trusted base `43f55a6aad38c7f53c4b7eb56682016639f74405`; it found that the plan omitted requirements `REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002` and `-004`, and Task 08's only referenced design did not declare requirement `-002`. The plan now declares requirements 002 and 004 and references `durable-agent-delivery.md`; Task 08 retains its reattachment design and adds the delivery design. The trusted helper's local preflight now reports `covered` with no errors. The catalog and full specification lint pass. A new exact-head GitHub evaluation is still pending after committing and pushing this documentation correction; other current-head checks were also pending in the captured snapshot. No remote-green claim is made.

Pre-delivery diagnosis reproduced the Docker image build and pinned Kind cluster creation successfully in disposable local environments.
The prior CI fixtures discarded child output, so those failures remain unclassified until fresh CI runs.
Fixture diagnostics now retain bounded failure tails without imposing an output-volume limit on successful commands.
Three focused real-child tests and targeted format/lint checks passed, including a RED/GREEN regression for output larger than 1 MiB.
The primary coordinator reviewed that fixture change. Current-head CI and final integration review remain pending.

The second base review found native Resume launching current reattachment asynchronously while acknowledgement trusted an initialized flag retained from inventory. The regression reproduced a startup failure being swallowed by recovery. Native Resume now waits for current startup and authenticated adoption before acknowledgment. The failed-start regression preserves the open block, interrupted submission, and payload with zero acknowledgment calls. Focused successful Resume and context-continuation controls passed with race instrumentation (4.000 seconds); changed-package lint reported zero issues. The coordinator reviewed the two-file correction.

The coordinator reviewed fresh desktop and phone restoration captures. They show the original conversation and normal composer without the rejected restart-resume form. The interrupted turn remains historical. The process native-resume fixture required a logger after integration with pressure-state logging; only test setup changed. The journal terminal-state fixture now distinguishes malformed retired active work (still unresolved) from acknowledged retired interrupted-unknown history. The full process race gate passed after the fixture correction. Normal commit hooks and remote delivery remain pending.

The bounded waiter `24039` completed after 43m54s for pushed head
`78065a90aa8f25aaaed4486225ee2b689b636d53`: 44 checks passed, 11 skipped, 1 neutral, 14 failed,
and none pending. This is a terminal red result, not a green result. The failed checks were:
`PR documentation coverage`; Backend Postgres (`38066670282/114255674435`); Backend Tests (2/2)
(`38066670282/114255674378`); aggregate Run Backend Tests (`38066670282/114260119276`); E2E
Containers Shards 4/6, 5/6, and 6/6 (`38066670301/114256734487`, `/114256734536`, and
`/114256734610`); E2E browser Shards 2/14, 4/14, 6/14, 7/14, and 12/14
(`38066670301/114256734603`, `/114256734656`, `/114256734577`, `/114256734693`, and
`/114256734688`); aggregate E2E Tests Passed (`38066670301/114264505519`); and Publish PR
documentation coverage status (`38066667357/114255591383`). The two aggregate checks are not
additional failing leaves. The local documentation correction is committed as `d36222a8b3d00ee4d6c7d476c11742d25b5b94eb` but remains unpushed; its trusted-base preflight was run against `43f55a6aad38c7f53c4b7eb56682016639f74405`, so it must be refreshed against current main before delivery.

The browser leaves currently classify as follows: Shards 2 and 6 timed out waiting for the read-only
recovery notice; Shards 7 and 12 rejected Quick Chat `session.launch` with
`unknown_prompt_outcome`; Shards 4 and 12 timed out waiting for the rejected Auggie provider load to
settle before retry. Container Shard 5 timed out waiting for `Sibling survived restart` while the
task pod remained Running and its PVC stayed Bound; artifact diagnostics contain no post-restart
WS/API timeline. Container Shard 6 failed its next-test fixture check because prior SSH runtime rows
remained after the dead-agentctl restart case. Container Shard 4 has a reproduced restart-then-delete
cleanup failure with its remote process still live; the SSH owner is tracing the retained cleanup
row. These leaf failures remain under investigation.

The storage cancellation failure was reproduced with a real SQLite writer wait; the minimal fix
returns the canceled context error when `BeginTxx` fails after cancellation, while preserving
unrelated database errors. The focused SQLite serialization/cancellation race tests passed
(package 6.409s). A real PostgreSQL run passed replacement rollback plus serialization for a
committed predecessor and a canceled waiter (package 3.166s; log
`/tmp/kandev-task05-replacement-postgres-actual.log`). These are local focused gates, not a claim
that hosted CI passed.

The executor startup-cleanup regression was reproduced against the pushed head: cleanup made no
exact-owner claim, changed the failed state to cancelled, and duplicated a stop already owned by a
different cancellation. The fix claims cleanup for the exact execution owner before stopping it.
These three focused executor regressions passed with race instrumentation, along with
`TestManagedGoCacheRecoveryFlows` in normal and race runs. The exact executor command was
`GOMAXPROCS=2 GOGC=50 GOCACHE=/tmp/kandev-task05-gocache go test -race -p=1 -trimpath ./internal/orchestrator/executor -run '^TestResumeSessionSynchronousStartFailure' -count=1`;
the backend app command was
`GOMAXPROCS=2 GOGC=50 GOCACHE=/tmp/kandev-task05-gocache go test -race -p=1 -trimpath ./internal/backendapp -run '^TestManagedGoCacheRecoveryFlows$' -count=1`.
Current-head PR review state and required CI remain unresolved. Current main advanced to
`3fb026365239bbc893fa4a5272d926640c7ef1f9`; the pushed branch is conflicting with it and requires
semantic reconciliation after worker fixes are frozen.

### Focused local recovery gates before the next rebase

On the shared local source snapshot, the lifecycle race selection passed in 1.098 seconds and the
orchestrator selection passed in 1.516 seconds. The selection covered
`TestRemoteRecoveryRetriesAfterPartialReplayTimeout`, bounded recovered-availability publication
after attempt cancellation and manager shutdown, retained persisted turn/message identity after a
retryable recovery guard, non-retryable guard visibility, and marked availability owner,
generation, paused-queue, and running-prompt controls. It also exercised the wait for an authenticated
remote updates stream before releasing the recovery guard.

The exact command was:

```bash
GOMAXPROCS=2 GOGC=50 GOCACHE=/tmp/kandev-task05-gocache go test -race -p=1 -trimpath \
  ./internal/agent/runtime/lifecycle ./internal/orchestrator \
  -run '^(TestRemoteRecoveryRetriesAfterPartialReplayTimeout|TestRecoveredAvailabilityPublicationSurvivesAttemptCancellation|TestRecoveredAvailabilityPublicationStopsWithManager|TestQueueUserPrompt_RetryableRecoveryGuardRetainsPersistedTurnAndMessage|TestPromptTask_NonRetryableRecoveryGuardRemainsVisible|TestRecoveredAvailabilityBootReadyDoesNotDowngradeRunningPrompt|TestRecoveredAvailabilityBootReadyRequiresCurrentOwnerAndGeneration|TestRecoveredAvailabilityBootReadyPreservesPausedQueue|TestRecoveredAvailabilityBootReadyRejectsTaggedWrongAttempt)$' \
  -count=1
```

Changed-scope Go lint passed with zero issues using
`golangci-lint run ./internal/agent/runtime/lifecycle ./internal/orchestrator --new-from-rev=HEAD --timeout=5m`,
and `git diff --check` passed. These are local focused results before the pending base reconciliation;
they do not replace the required post-rebase lifecycle/browser checks or hosted current-head CI.
