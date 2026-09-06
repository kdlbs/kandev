---
spec: docs/specs/agents/requirements/agent-stall-recovery.md
requirements:
  - REQ-AGENTS-AGENT-STALL-RECOVERY-001
  - REQ-TASKS-TASK-STOP-REACHABILITY-001
system_designs:
  - docs/specs/agents/system-design/agent-stall-recovery.md
  - docs/specs/tasks/system-design/task-stop-reachability.md
decision: docs/decisions/2026-09-02-terminal-stall-owns-process-teardown.md
created: 2026-09-02
status: implemented
---

# Implementation Plan: Never-Started Stall Teardown

## Overview

Three linked backend corrections that restore one invariant: a record of
terminal work must correspond to no running process.

1. The inactivity clock stops accepting metadata frames as progress, so a
   never-started prompt trips the watchdog on time.
2. The never-started classification tears down its own execution instead of
   only recording `FAILED`.
3. The task-scoped stop resolves what to halt from the execution registry as
   well as the active-session query, so a terminal session can no longer hide a
   live process.

Backend only. No schema migration, no HTTP or WebSocket contract change, no
frontend change.

## Confirmed root cause

**Defect 1 — the clock can be reset by metadata alone.**
`waitForPromptDone` (`apps/backend/internal/agent/runtime/lifecycle/session.go`)
fires when `time.Since(lastActivityAt) >= 5m`. `recordActivity`
(`apps/backend/internal/agent/runtime/lifecycle/manager_events.go:551-559`)
assigns `lastActivityAt = time.Now()` for **every** inbound event, while it sets
`agentEventSincePrompt` only for `turnContentEventTypes`. A session emitting
only `usage_update`, `context_window`, `available_commands_update`, or
`session_info_update` therefore restarts the clock forever while
`NeverStarted` stays true, and the watchdog never fires. Observed: execution
`12fed303` on task `54dd18cf` produced nothing for 5m23s with no stall log.

**Defect 2 — the watchdog marks `FAILED` but never stops the process.**
`handleAgentStalled`
(`apps/backend/internal/orchestrator/event_handlers_stall.go:37-122`) posts the
notice and calls `recordSessionLaunchFailure`, which moves the session and task
to `FAILED`. It never calls a stop. The agent process survives its own
tombstone.

**Defect 3 — `FAILED` makes the task-scoped stop structurally blind.**
`Executor.StopByTaskID`
(`apps/backend/internal/orchestrator/executor/executor_interaction.go:245-277`)
resolves sessions with `ListActiveTaskSessionsByTaskID`, whose SQL matches only
`state IN ('CREATED','STARTING','RUNNING','WAITING_FOR_INPUT')`
(`apps/backend/internal/task/repository/sqlite/session.go:2495-2500`). `FAILED`
is outside that set, so the stop returns `ErrExecutionNotFound` while the
execution is still registered in `ExecutionStore` and the process is still
running.

Reproduced live on task `bca4780c` (2026-09-02):

```text
12:11:29  agent stall detected ... execution_id=04c6fac6 ... "never_started": true
          -> session 038c8173 state = FAILED   (process NOT stopped)
12:12:38  stopping task execution ... reason "diagnosing MCP startup hang"
12:12:38  ERROR failed to stop task ... error: "execution not found"
12:12:57  stopping execution ... agent_execution_id=04c6fac6   <- session-scoped stop worked
```

The session-scoped stop succeeding 19 seconds later proves the execution was
alive and registered throughout.

## Requirement and design references

| Work order | Acceptance criteria | System design |
| --- | --- | --- |
| Task 01 | `AC-AGENTS-AGENT-STALL-RECOVERY-001.9`, `.2` | [Agent stall recovery](../../specs/agents/system-design/agent-stall-recovery.md) |
| Task 02 | `AC-AGENTS-AGENT-STALL-RECOVERY-001.10`, `.11`, `.7` | [Agent stall recovery](../../specs/agents/system-design/agent-stall-recovery.md) |
| Task 03 | `AC-TASKS-TASK-STOP-REACHABILITY-001.1` – `.7` | [Task stop reachability](../../specs/tasks/system-design/task-stop-reachability.md) |

## Work orders

Sequential. No task is parallel-safe: Task 02 depends on Task 01's clock being
honest before teardown can be exercised on schedule, and Task 03's regression
constructs the orphan that Task 02 stops producing.

- [x] [Task 01: Measure prompt progress, not traffic](task-01-honest-inactivity-clock.md)
- [x] [Task 02: Tear down a never-started execution](task-02-terminal-stall-teardown.md)
- [x] [Task 03: Reach a registered execution from a task stop](task-03-stop-reaches-registered-execution.md)

## Verification strategy

Each work order owns a regression that fails first for the stated reason. The
lifecycle timing regressions use `testing/synctest` with the real one-minute
ticker and the real five-minute threshold; production durations are not
shortened to make a test pass.

No end-to-end browser test is added. All three corrections are backend
lifecycle and stop-path behavior with no new user-facing surface: the notice,
its copy, and its rendering are unchanged and already covered by
`apps/web/e2e/tests/session/pause-resume-recovery.spec.ts`.

After all three work orders pass their own checks:

1. `cd apps/backend && gofmt -l $(git diff --name-only origin/main -- '*.go')`
2. `make -C apps/backend test`
3. `make -C apps/backend lint`

### Final verification results

This branch's HEAD (`c51ec0a21`) is far behind `origin/main`, so step 1's
literal `git diff --name-only origin/main` pulls in thousands of unrelated
files that don't exist in this old checkout's layout. Ran the equivalent,
correctly scoped checks instead:

1. `gofmt -l` on every file this plan touched — clean.
2. `go test -tags fts5 ./...` (`make -C apps/backend test`) — ran to
   completion after two host disk-space exhaustions during this session were
   cleared with `go clean -cache` (the repo's Go build/link artifacts, fully
   regenerable; unrelated to this change). Full-suite run: every package not
   already known-flaky on this host passes. The only failures are the same
   set already documented and independently confirmed pre-existing (via
   scoped `git stash` round-trips back to unmodified HEAD) in the three work
   orders' Results sections:
   - 7 tests in `internal/agent/runtime/lifecycle` (6 `TestWorktreePreparer_*`
     git/worktree-state tests plus
     `TestBuildAuthMethodsIdentityAgentOverridesEnvironment`, a Unix-socket
     path-length limit).
   - A larger, separately-confirmed pre-existing cluster keyed on the same
     `unsafe worktree path: not a directory` fixture assumption, spanning
     `internal/worktree`, `internal/task/service`,
     `internal/system/storage/workspaces`, `internal/launcher`,
     `internal/agentctl/server/*`, `internal/common/config`, and
     `internal/agent/managedruntime` — none of these packages, or anything
     they depend on, is touched by this plan's changes (`lifecycle`,
     `orchestrator`, `orchestrator/executor`, `backendapp/adapters.go`,
     `integration` test doubles only). Reproduced identically against
     unmodified HEAD via scoped stash to confirm.
   - `internal/orchestrator`, `internal/orchestrator/executor`, and every
     other package this plan touches: all green.
3. `golangci-lint run ./... --new-from-rev=<merge-base-with-origin/main>` (the
   correct base, found via `git merge-base HEAD origin/main`, since comparing
   against `origin/main` directly has the same stale-branch problem as step
   1) — 0 issues.
4. `go build ./...` — clean.

All three work orders are code-complete, tested, and documented.

### Build-step self-review addendum

Walked the recovery path added by Task 03 for the "guard reproduces the
failure it exists to prevent" and "value re-read after the decision" patterns
(see `task-03-...md` for the full account). Found and fixed one real bug:
`recoverRegistryOnlySessions` silently dropped a registered session on a
transient `GetTaskSession` load failure, which could make `StopByTaskID`
report the same `ErrExecutionNotFound` as "nothing registered" — a false
all-clear over a real orphan. Fixed with a new regression,
`TestStopByTaskID_RegistryRecoveryLoadFailureIsNotReportedAsNotFound`
(red-then-green).

Also investigated, on request, whether defect 1 pre-fix leaves a stalled
session at RUNNING (not FAILED) and whether `StopByTaskID` is blind to that
case too. Conclusion, detailed in Task 03's Results: yes it leaves it at
RUNNING, but `StopByTaskID` was never blind there — RUNNING has always been
in `ListActiveTaskSessionsByTaskID`'s active-state set, and Task 01 + Task 02
together remove the failure mode at its source. No fourth defect shape found;
nothing added beyond the hardening fix above.

### Definition of Done (Build step gauntlet)

- `make fmt` — clean, nothing reformatted.
- `make typecheck` — clean, after regenerating
  `apps/web/lib/generated/{changelog,release-notes}.json` (a pre-existing
  worktree gap: `make typecheck` invokes `tsc` via `pnpm exec`, which does not
  run the `pretypecheck` npm hook that normally generates these files — same
  class of issue as the documented `apps/node_modules` worktree gotcha, not
  caused by this change).
- `make lint` (backend + web + harness + specs + architecture) — 0 issues,
  including an unscoped `golangci-lint run ./...` across the whole backend.
- `make lint-format` — clean.
- `cd apps/web && pnpm run i18n:ratchet` — clean, no-op (no `apps/web` files
  touched by this change).
- `make test-cli test-scripts` — `test-cli` green. `test-scripts` has one
  failure, `Make variable home` in `scripts/dev-prod-db-path.test.sh`,
  unrelated to anything this plan touches (Makefile `KANDEV_HOME_DIR` path
  resolution). Proven pre-existing per the workflow's own rule: reproduced
  identically in a separate scratch worktree checked out at the merge base
  (`git merge-base HEAD origin/main` = `c51ec0a21`), not via stash.
- `make -C apps/backend test` (`test-backend`) — full suite green except the
  documented pre-existing/environment failures (disk pressure on this shared
  host); see the Final verification results section above and each work
  order's Results.
- `make test-web` — not run to completion. This host was running vitest for
  several other concurrent sessions' worktrees at the same time (`ps aux`
  showed 4+ other `vitest run` processes from unrelated task directories),
  disk had dropped to ~10Gi free, and the run produced no output after 37+
  minutes of CPU time. `apps/web` is untouched by this diff and both
  `make typecheck` and web `make lint` (eslint) are clean, which is the
  relevant signal for a change that touches zero frontend files. Stopped the
  run rather than let it continue starving a shared, already disk-constrained
  host for a suite this diff cannot affect.

## Reviewed but excluded: the unbounded `conn.Prompt()` call

`apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
starts `conn.Prompt(traceCtx, ...)` in a goroutine with no deadline. That is
why a hung adapter is unbounded, and the task asks whether the fix belongs here.

It does not, for three reasons.

- **It is a different decision, with a different owner.** A prompt deadline is a
  policy about how long legitimate work may run. That is exactly the ambiguity
  [ADR-2026-07-29](../../decisions/2026-07-29-agent-stall-user-controlled-recovery.md)
  weighed when it rejected automatic timeouts, and reversing it needs its own
  requirement covering the value, what the user sees when it fires, and how it
  composes with cancel escalation. Deciding that inside a defect fix would
  smuggle a product change into a repair.
- **It would not fix any of the three defects.** A deadline on the ACP call
  cannot make a metadata-only stream trip the watchdog, cannot stop the process
  that a terminal classification left running, and cannot make the stop path see
  a registered execution. The defects are in the observation, the disposal, and
  the resolution; the deadline is upstream of all three.
- **Its urgency is what this work removes.** The reason an unbounded call is
  currently dangerous is that nothing downstream reports or disposes of it.
  After these three work orders, a hung adapter is detected on schedule, its
  process is torn down, and any remaining orphan is stoppable. A deadline
  becomes a defence in depth rather than the only backstop, and can be evaluated
  on its own merits.

Recommended as a separate task, not carried here.

## Risks and non-goals

- **The advisory notice becomes more frequent.** A turn that emits only metadata
  frames after real activity will now reach the advisory threshold. This is
  intended and specified by `AC-AGENTS-AGENT-STALL-RECOVERY-001.9`; the notice
  is non-destructive and offers only a cancel action.
- **The completion-signal watchdog shares the corrected clock.** It reads the
  same activity snapshot, so it also stops being fooled by metadata frames. This
  is a deliberate consequence, covered by a Task 01 regression, not a silent
  side effect.
- **Stopping an orphan reclaims the card.** A task whose only live work was an
  orphan now stops successfully, so the caller's existing post-stop task
  handling runs. Task 03's regression pins this so it is a reviewed outcome.
- **A failed teardown still leaves an orphan.** It is logged and the execution
  stays registered for a later stop; Task 03 is what makes that retry reachable.
- **MCP startup timeouts are out of scope.** They are what starts the stall;
  this work is about the stall being unreported and unrecoverable.
- No test is weakened or deleted. The four existing stall regressions in
  `session_test.go` and the six in `event_handlers_stall_test.go` must keep
  passing unchanged.
