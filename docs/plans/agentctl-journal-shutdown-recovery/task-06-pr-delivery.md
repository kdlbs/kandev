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

At the pre-push snapshot on 2026-10-10, the candidate was based on `main` at `3fb026365239bbc893fa4a5272d926640c7ef1f9`, with baseline `084901d76b2c0a7e5300230e9b5e11fb15e73e78` and reviewed changes in the worktree. The remote PR then pointed to `78065a90aa8f25aaaed4486225ee2b689b636d53`; those checks are historical and apply only to that head. A fresh pre-push snapshot confirmed that `main` was at `3fb026365239bbc893fa4a5272d926640c7ef1f9`, the PR was open and not queued, and the old head had 44 passing, 11 skipped, 1 neutral, 14 failing, and 0 pending checks with no unresolved review threads. The rebase's only source conflict was `lifecycle/manager.go`; its resolution preserves main's passthrough MCP ownership maps and the PR's remote-recovery and stop-context fields.

Local frontend recovery checks passed: 11 affected recovery/service/store test files passed 179 tests, web TypeScript checking passed, and ESLint passed for changed TypeScript files with zero warnings. Fresh desktop and phone read-only recovery selections passed. The focused frontend Quick Chat and Auggie recovery selections also passed. The trusted documentation evaluator reported `covered` for the actual local PR paths on base `3fb026365239bbc893fa4a5272d926640c7ef1f9`; catalog validation and full specification lint had passed before the latest result synchronization. These local gates do not change the old head's hosted result.

The post-rebase rejected-replay regression exposed a cleanup bug: failed replay deleted `.pi/mcp.json` while the exact remote owner remained live. Cleanup now removes tracking only for the exact still-pending candidate and cancels reconciliation only for that candidate, preserving persisted remote MCP claims and the file for a same-record retry. `TestRemoteRecoveryRetriesAfterPartialReplayTimeout` and all `TestPiProjectMCP*` cleanup controls pass with race instrumentation, and full lifecycle lint reports zero issues. The Kubernetes inventory regression reproduced the missing trusted executor identity, then passed after request construction restored `executor_id` from the row's `ExecutorID` without changing the existing validator. The focused regression and `^TestKubernetesRecovery` race selection passed, as did lifecycle lint. A fresh managed HOST run of `tests/kubernetes/kubernetes-task-pod.spec.ts`, selecting `sessions share one task pod through stop and backend restart` with retries disabled, passed 1 test in 2.8 minutes; the test itself took 14 seconds. Its backend binary SHA-256 was `98f9cf51178d9e94fd329ca63a1e727a1d7b97c0853ea24a90d3c4306d5df0f9`.

The four-package race gate passed on the earlier snapshot before the task-service handoff correction: lifecycle 107.979 seconds, orchestrator 176.826 seconds, executor 11.114 seconds, and backendapp 101.154 seconds. A later review found that task maintenance could read a pending remote owner immediately before it became live, then misclassify the session as abandoned. The task-service adapters and periodic reconcilers now order pending-owner checks before live-session reads and recheck liveness before writing or retrying settlement. Eleven focused handoff regressions cover the active-session sweep, orphan pre-write check, and partial-settlement retry; normal and race runs passed in 0.418 and 1.565 seconds, and scoped lint reported zero issues (`/tmp/pr4380-pending-session-handoff-green.log`, `/tmp/pr4380-pending-session-handoff-race.log`, `/tmp/pr4380-pending-session-handoff-lint.log`). The final full affected-package race gate passed task/service in 86.616 seconds and backendapp in 100.561 seconds; its log is `/tmp/pr4380-final-task-service-backendapp-race.log`. Catalog validation reported 371 decisions and 1,522 specifications, and full specification lint passed. After push, hosted CI and reviews will be monitored by the delegated PR-fixup owner. Task 06 remains in progress until those checks pass; no current-head GitHub result is claimed here.

### Historical terminal CI result for pushed head `78065a90aa8f25aaaed4486225ee2b689b636d53`

The complete snapshot for this earlier head reported run `38066667357`, attempt 1, job `114255591383` (`Publish PR documentation coverage status`) failed with `Linked delivery documentation is incomplete or invalid.` The run was terminal. The evaluator from trusted base `43f55a6aad38c7f53c4b7eb56682016639f74405` identified missing plan links for requirements `REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002` and `-004`, and a missing Task 08 design reference for requirement `-002`. The plan now declares requirements 002 and 004 and references `durable-agent-delivery.md`; Task 08 retains its reattachment design and adds the delivery design. The refreshed trusted evaluator on base `3fb026365239bbc893fa4a5272d926640c7ef1f9` now reports `covered` for the rebased local tree. This historical red run is not a result for the current local HEAD.

Pre-delivery diagnosis reproduced the Docker image build and pinned Kind cluster creation successfully in disposable local environments.
The prior CI fixtures discarded child output, so those failures remain unclassified until fresh CI runs.
Fixture diagnostics now retain bounded failure tails without imposing an output-volume limit on successful commands.
Three focused real-child tests and targeted format/lint checks passed, including a RED/GREEN regression for output larger than 1 MiB.
The primary coordinator reviewed that fixture change. At that historical snapshot, current-head CI and final integration review remained pending.

The second base review found native Resume launching current reattachment asynchronously while acknowledgement trusted an initialized flag retained from inventory. The regression reproduced a startup failure being swallowed by recovery. Native Resume now waits for current startup and authenticated adoption before acknowledgment. The failed-start regression preserves the open block, interrupted submission, and payload with zero acknowledgment calls. Focused successful Resume and context-continuation controls passed with race instrumentation (4.000 seconds); changed-package lint reported zero issues. The coordinator reviewed the two-file correction.

The coordinator reviewed fresh desktop and phone restoration captures. They show the original conversation and normal composer without the rejected restart-resume form. The interrupted turn remains historical. The process native-resume fixture required a logger after integration with pressure-state logging; only test setup changed. The journal terminal-state fixture now distinguishes malformed retired active work (still unresolved) from acknowledged retired interrupted-unknown history. The full process race gate passed after the fixture correction. At that historical point, normal commit hooks and remote delivery were still pending.

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
additional failing leaves. The documentation correction was committed locally as `d36222a8b3d00ee4d6c7d476c11742d25b5b94eb` after this snapshot and is included in the rebased local tree. Its earlier preflight used base `43f55a6aad38c7f53c4b7eb56682016639f74405`; the refreshed preflight on base `3fb026365239bbc893fa4a5272d926640c7ef1f9` is recorded above.

The browser leaves on the historical head failed as follows: Shards 2 and 6 timed out waiting for
the read-only recovery notice; Shards 7 and 12 rejected Quick Chat `session.launch` with
`unknown_prompt_outcome`; Shards 4 and 12 timed out waiting for the rejected Auggie provider load to
settle before retry. Container Shard 5 timed out waiting for `Sibling survived restart` while the
task pod remained Running and its PVC stayed Bound. Container Shard 6 failed its next-test fixture
check because prior SSH runtime rows remained after the dead-agentctl restart case. Container Shard 4
failed restart-then-delete cleanup while its remote process remained live. These are historical
failures, not the current hosted result. Local follow-up passed the corrected desktop/phone read-only
recovery selections, Quick Chat and Auggie recovery selections, the SSH mock-agent restart/delete
scenario, and the exact Kind task-pod restart case. The real OpenCode SSH case did not reach its test
body because the fixture lacked `effective_version`, so that provider-backed path remains unverified
locally. Focused PostgreSQL replacement/cancellation and backend-app recovery tests also pass locally.
These scoped results do not establish hosted green CI for the candidate.

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
That terminal waiter and its leaf classifications apply only to the pushed head `78065a90aa8f25aaaed4486225ee2b689b636d53`. At that snapshot, local work was at `3f35f29d377fd539945b061435c59baf605039ad` on base `3fb026365239bbc893fa4a5272d926640c7ef1f9`; later rebase and local fixes have superseded it. The candidate's remote checks and review state must be refreshed after the reviewed fixes are committed and pushed.

### Focused local recovery gates on the pre-rebase snapshot

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

These results predate the rebase onto `3fb026365239bbc893fa4a5272d926640c7ef1f9` and are not final current-base lifecycle evidence. Changed-scope Go lint passed with zero issues using
`golangci-lint run ./internal/agent/runtime/lifecycle ./internal/orchestrator --new-from-rev=HEAD --timeout=5m`,
and `git diff --check` passed. These are local focused results before the pending base reconciliation;
they do not replace the required post-rebase lifecycle/browser checks or hosted current-head CI.


### Pinned silent restore after workspace admission refusal (2026-10-27)

SDK PR integration CI at `b7f1e656796` retained a failed mid-turn restart trace:
its session showed unresolved recovery while workspace readers and automatic
restore failed concurrently. That trace does not record shared-operation
ownership. An independent existing-boundary regression proves the source gap:
a pinned silent candidate has no manual `RecoveryAction`, so it inherits a
workspace peer's recovery refusal instead of running its own admission.

Extend the coalescing recovery classification to runtime-only pinned candidate
identities, preserving serialization, ordinary-launch rejection, caller
cancellation, unrelated peer errors, and own-launch errors. Do not assign a
manual recovery action or resolve any delivery evidence early. This enforces
AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.20/.21 through the existing admission path.
The new silent-candidate regression fails before the production change. Run
focused lifecycle race/admission checks and rebuilt mid-turn browser coverage
before delivery. The two mobile CI failures separately passed five unchanged
local repetitions each; neither original cause is established.


Validation result: the focused lifecycle recovery/admission selection passed
with race detection on the final code, including cancelled silent candidates.
Fresh host/web/plugin builds and three mid-turn restart repetitions passed with
retries disabled. Independent review found no actionable issue. These results
prove the controlled classification correction; the original CI trace does not
identify shared-operation ownership and therefore cannot prove that causal link.
