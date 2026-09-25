---
id: "01-observe-concurrent-worktree-sessions"
title: "Observe concurrent worktree sessions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004.1
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004.2
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004.3
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004.4
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
---

# Task 01: Observe Concurrent Worktree Sessions

## Summary

Record, at the two seams that start an agent process against an already
attached task workspace, that another session of the same task is already
working. Emit a structured warning and an expvar counter. Change no admission
outcome.

## In scope

- Generalize `hasOtherWorkingSessions` into a helper returning sibling session
  IDs plus a read-failure signal, and keep its existing boolean caller on it.
- Call the helper from `Executor.LaunchPreparedSession` when it starts the
  agent, and from `Executor.resumeSession`, before the process starts.
- Add `session_coresidency_admitted_total` (labelled by `site`) and
  `session_coresidency_observation_skipped_total` (labelled by `reason`).
- Add the park interaction warning to `docs/public/tasks-and-workflows.md`
  where it currently says a parked session is not an active process.
- Add Go unit coverage for the co-resident, solitary, and read-failure cases.

## Out of scope

- Refusing, deferring, or serializing any launch or resume.
- Changing the default `profile_session_end_policy`.
- Frontend, board indicators, or session-tab surfaces.
- Changing `is_primary` semantics.
- Instance-wide session-ceiling behavior.

## Acceptance

- A launch or resume with a working sibling increments the counter once for
  that start and logs the sibling session IDs.
- A launch or resume with no working sibling records nothing.
- A failing sibling read records a skip with its reason, and the launch or
  resume still proceeds.
- Counter label keys are exactly `site` and `reason`; no identifier appears as
  a label value.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/executor
cd apps/backend && make lint
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
```

Each new unit test must fail before the production change and pass after it.

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/session_coresidency_metrics.go`
- `apps/backend/internal/orchestrator/executor/session_coresidency_test.go`
- `docs/public/tasks-and-workflows.md`
- `docs/plans/concurrent-session-worktree-visibility/plan.md`
- `docs/plans/concurrent-session-worktree-visibility/task-01-observe-concurrent-worktree-sessions.md`

## Dependencies

None.

## Risks

- Placing the observation after the process starts would report the condition
  too late to be useful in a log read backwards from a corruption.
- Adding identifiers as counter labels would make cardinality unbounded.
- Logging at error level would imply Kandev failed to prevent something it
  deliberately permits; warning with explicit wording is required.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004` and its criteria.
- `docs/specs/tasks/system-design/additional-session-workspace-reuse.md`,
  section "Concurrent session visibility".
- `docs/decisions/2026-08-31-workflow-profile-session-switch-policy.md`.
- `hasOtherWorkingSessions`, `isRuntimeWorkingSessionState`,
  `LaunchPreparedSession`, `resumeSession`, `validateAndLockResume`.
- `apps/backend/internal/orchestrator/office_stall_metrics.go` as the expvar
  label-model precedent.

## Results

Implemented on `feature/two-live-sessions-ca-nal`, commit `06f16e718`.

- `executor_execute.go`: extracted `workingSessionSiblings` (returns sibling
  IDs + error, no early stop — a task's live-session count is small and the
  call runs once per launch/resume, not per turn, so the plan's "stop at
  first working sibling" mitigation was unneeded; kept for simplicity).
  `hasOtherWorkingSessions` now calls it (behavior-preserving refactor, same
  log lines). Added `observeSessionCoresidency(ctx, site, taskID, sessionID)`:
  warns + increments `session_coresidency_admitted_total{site}` when siblings
  exist; on a read failure, increments
  `session_coresidency_observation_skipped_total{reason=sibling_read_failed}`
  and warns, never treating the failure as zero siblings.
- Wired into `LaunchPreparedSession` (guarded on `startAgent`, immediately
  before the fast-path/full-launch branch) and `resumeSession` (guarded on
  `startAgent`, immediately after the existing `admitWorktreeRecovery` call).
  Both are before the agent process starts on every branch.
- `session_coresidency_metrics.go` (new): the two expvar maps, modelled on
  `orchestrator/office_stall_metrics.go`. Labels are `site` (`launch`,
  `resume`) and `reason` (`sibling_read_failed`) only — no identifiers.
- `session_coresidency_test.go` (new): solitary session (nothing recorded),
  working sibling (warning + counter, wording asserted to contain "permit"
  and not "fail"), sibling-read failure (skip counter + warning, not the
  admitted counter), plus two wiring tests driving real
  `LaunchPreparedSession`/`ResumeSession` calls to confirm the call sites are
  reached, not just the extracted helper.
- `executor_mocks_test.go`: added `listTaskSessionsFunc` override hook so the
  read-failure case could be simulated (mirrors the existing
  `listActiveTaskSessionsByTaskIDFunc` pattern).
- `docs/public/tasks-and-workflows.md`: added a paragraph after the "parked
  session is not an active process" sentence stating that answering a parked
  session starts an agent in the same shared workspace a still-running
  destination session may be using, that Kandev permits and records but does
  not prevent this, and that using **Complete the session** avoids it.

### TDD receipt

Watched the new tests fail before the production code existed: stashed
`executor_execute.go`, `executor_resume.go`, and the new
`session_coresidency_metrics.go` (keeping the new test file and the mock
hook), then ran `go vet ./internal/orchestrator/executor/...`:

```
vet: internal/orchestrator/executor/session_coresidency_test.go:62:25: undefined: sessionCoresidencyAdmittedTotalVar
```

Restored the stash, re-ran green:

```
$ go test ./internal/orchestrator/executor/... -run 'ObservesWorkingSibling' -v
--- PASS: TestLaunchPreparedSession_ObservesWorkingSiblingBeforeStartingAgent (0.00s)
--- PASS: TestResumeSession_ObservesWorkingSiblingBeforeStartingAgent (0.00s)
PASS
```

### Definition-of-Done receipts

```
$ make -C apps/backend fmt        # clean; unrelated pre-existing drift in
                                   # session_open_recovery_test.go /
                                   # workflow_start_prompt.go reverted, not ours
$ make -C apps/backend lint       # golangci-lint run ./...  ->  0 issues.
$ make -C apps/backend test       # CGO_ENABLED=1 go test -tags fts5 ./...
                                   # FAIL internal/worktree only (see below);
                                   # every other package ok
$ make lint-format                # prettier --check web cli packages -> ok
$ (cd apps/web && pnpm run i18n:ratchet)
                                   # no UI source added or modified
$ python3 scripts/lint-spec-files.py --all   # All specification files passed.
$ python3 scripts/list-docs.py validate      # Validated 292 decisions and 1054 specifications.
```

`apps/node_modules` was missing in this worktree (the documented worktree
gotcha); ran `cd apps && pnpm install --frozen-lockfile` once before the
web-side commands above.

### Pre-existing failure proof (`internal/worktree`)

`make -C apps/backend test` failed 4 tests, all in `internal/worktree`, a
package this task never touches:
`TestRestoreManagedBranchFromRecoveryHead_BoundsUpdateRefAdmissionAndReleasesRepoLock`,
`TestManager_AdmitTaskRecoveryCoalescesFollowerAfterSuccessfulRecovery`,
`TestManager_Create_RefusesInvalidNonEmptyCheckoutWithoutRemovingIt`,
`TestManager_Create_RefusesMissingAdminWhenRecordedBranchIsUnreachable` — all
four are on the repo's known-failures list ("Worktree and workspace lifecycle
tests"). Reproduced on `git merge-base HEAD origin/main` (`9e171c83774368a8d39dea3721467ea6a0b5fb85`,
which is also this branch's own base — no commits diverge yet) in a scratch
worktree at `/tmp/scratch-worktree-coresidency`:

```
$ cd /tmp/scratch-worktree-coresidency/apps/backend && CGO_ENABLED=1 go test -tags fts5 \
  -run 'TestManager_Create_RefusesInvalidNonEmptyCheckoutWithoutRemovingIt|TestManager_AdmitTaskRecoveryCoalescesFollowerAfterSuccessfulRecovery|TestManager_Create_RefusesMissingAdminWhenRecordedBranchIsUnreachable|TestRestoreManagedBranchFromRecoveryHead_BoundsUpdateRefAdmission' \
  -v ./internal/worktree/...
--- FAIL: TestRestoreManagedBranchFromRecoveryHead_BoundsUpdateRefAdmissionAndReleasesRepoLock (1.01s)
--- FAIL: TestManager_AdmitTaskRecoveryCoalescesFollowerAfterSuccessfulRecovery (0.51s)
--- FAIL: TestManager_Create_RefusesInvalidNonEmptyCheckoutWithoutRemovingIt (0.27s)
--- FAIL: TestManager_Create_RefusesMissingAdminWhenRecordedBranchIsUnreachable (0.41s)
```

Same tests, same error messages, on unmodified `origin/main`. Pre-existing;
not this task's regression. Scratch worktree removed after the check
(`git worktree remove /tmp/scratch-worktree-coresidency --force`).
