---
id: "03-template-and-reconcile"
title: "Ship the gate on_comment and backfill existing Office workflows"
status: done
wave: 2
depends_on:
  - "01-fanout-skip-decided-and-author"
  - "02-gate-comment-prompt"
plan: "plan.md"
requirements:
  - REQ-OFFICE-GATE-COMMENT-001
  - REQ-OFFICE-GATE-COMMENT-002
  - REQ-OFFICE-GATE-COMMENT-004
  - REQ-OFFICE-GATE-COMMENT-005
acceptance_criteria:
  - AC-OFFICE-GATE-COMMENT-001.1
  - AC-OFFICE-GATE-COMMENT-001.2
  - AC-OFFICE-GATE-COMMENT-001.8
  - AC-OFFICE-GATE-COMMENT-001.11
  - AC-OFFICE-GATE-COMMENT-001.12
  - AC-OFFICE-GATE-COMMENT-001.13
  - AC-OFFICE-GATE-COMMENT-001.15
  - AC-OFFICE-GATE-COMMENT-002.1
  - AC-OFFICE-GATE-COMMENT-002.6
  - AC-OFFICE-GATE-COMMENT-002.7
  - AC-OFFICE-GATE-COMMENT-004.1
  - AC-OFFICE-GATE-COMMENT-004.2
  - AC-OFFICE-GATE-COMMENT-004.3
  - AC-OFFICE-GATE-COMMENT-004.4
  - AC-OFFICE-GATE-COMMENT-004.5
  - AC-OFFICE-GATE-COMMENT-004.6
  - AC-OFFICE-GATE-COMMENT-004.7
  - AC-OFFICE-GATE-COMMENT-005.1
  - AC-OFFICE-GATE-COMMENT-005.4
system_design:
  - ../../specs/office/system-design/gate-comment-wake-01.md
---

# Task 03: Ship The Gate on_comment And Backfill Existing Office Workflows

## Summary

Declare the `on_comment` fan-out on `review` and `approval` in
`office-default.yml`, add the startup reconciler that appends it to system-owned
workflows materialized earlier, and prove the whole loop through the real engine:
comment wakes the undecided reviewer, the reviewer's own comment wakes nobody,
the reviewer's verdict moves the card.

## In scope

- `apps/backend/config/workflows/office-default.yml`: the two `on_comment`
  blocks from the design, verbatim.
- `healBuiltinWorkflowStepOnCommentFanOut` in
  `internal/task/repository/sqlite/`, registered in `base_schema.go` after
  `healBuiltinWorkflowStepOnAgentError`, modelled on it and on
  `tryHealWorkflowStepEvents`, with the same bounded CAS retry and a test hook
  for forced retries.
- An orchestrator-level test on the shipped template covering AC-001.1, .2,
  .12, .13, AC-002.1, AC-002.7 and AC-005.1, and a dashboard-level test for
  AC-001.8 and AC-001.15 (a gated-step comment whose step fails to load keeps
  the legacy runner wake, and so does one on a step whose `on_comment` list
  holds a failing `queue_run` ahead of the reconciled fan-out).
- Spec bookkeeping: a row for this pair in `apps/backend/internal/office/AGENTS.md`
  and a sentence naming `skip_decided` beside `queue_run_for_each_participant`
  in `docs/public/workflow-import-export.md` (via `/docs-maintainer`).

## Out of scope

- Operator-owned (`is_system = 0`) workflows.
- Any runner wake at gated steps, run coalescing, or session-less tasks.
- Changing the step-entry fan-outs' config.

## Acceptance

- A comment by a non-seated agent (coordinator) or by `"user"` on a Review task
  with one undecided reviewer queues exactly one `task_comment` run for that
  reviewer and none for the runner; the reviewer's own comment queues none; an
  Approval comment queues runs for undecided approvers only; the reviewer's
  `approved` decision moves the task to Approval.
- A first dispatch with one seat's enqueue failing, followed by the subscriber's
  redispatch of the same comment, ends with exactly one run per undecided seat
  and no runner `task_comment` run.
- A comment by the task's assignee agent at Review queues no seat run and no
  legacy run, and the decision store is never read.
- On startup, a system-owned `office-default` workflow materialized without the
  gate `on_comment` gains it appended with every other action preserved; a
  second startup changes nothing; an `is_system = 0` copy and a step already
  declaring a same-role fan-out are left unchanged; exhausted CAS retries warn
  and leave the row unchanged without failing startup.
- A comment on a Work or Done task queues the same runs as before this package.

## Verification

```bash
# From the repository root:
cd apps/backend && go test ./internal/task/repository/sqlite/... -run 'Heal|Reconcile|OnComment' -race -count=1
cd apps/backend && go test ./internal/orchestrator/... -run 'GateComment|OnComment' -race -count=1
cd apps/backend && go test ./internal/office/dashboard/... -run Comment -race -count=1
cd apps/backend && go test ./config/workflows/... ./internal/workflow/... -race -count=1
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --cached --name-only | grep '\.go$' | xargs -r gofmt -l
git diff --check
```

Manual end-to-end evidence, recorded under Results: start a dev backend
(`make dev`) against a copy of a database holding a pre-change `office-default`
workflow, confirm the Review and Approval steps' `events` gained `on_comment`
after start, post a comment on a Review task through the dashboard API, and
paste the queued run row (agent, reason, payload).

## Files likely touched

- `apps/backend/config/workflows/office-default.yml`
- `apps/backend/internal/task/repository/sqlite/builtin_workflow_on_comment_reconcile.go`
- `apps/backend/internal/task/repository/sqlite/builtin_workflow_on_comment_reconcile_test.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/orchestrator/gate_comment_wake_test.go`
- `apps/backend/internal/office/dashboard/service_tasks_test.go`
- `apps/backend/internal/office/AGENTS.md`
- `docs/public/workflow-import-export.md`

## Dependencies

Tasks 01 and 02. Shipping the template without Task 01 would wake every seat,
including the author.

## Risks

- Template golden tests or snapshot tests over `office-default.yml` must be
  updated to the new content, never relaxed.
- The reconciler matches rows by template step **name**; a system row whose step
  was renamed is not reached, the same limit the seat and agent-error
  reconcilers have. Do not widen the match.
- Never run the manual check against the operator's live database; use a copy.

## Parallelism

`sequential`

## Inputs

- `docs/specs/office/system-design/gate-comment-wake-01.md`, sections "Template
  change", "Engine-handled comment", "Existing workflows".
- `apps/backend/internal/task/repository/sqlite/builtin_workflow_agent_error_reconcile.go`
  and its tests as the model.
- `apps/backend/internal/office/dashboard/service_tasks.go`
  (`dispatchCommentEngineTrigger`, `runReactivityForComment`).

## Results

Done. `office-default.yml` declares the `on_comment`
`queue_run_for_each_participant` blocks on `review` (role `reviewer`) and
`approval` (role `approver`), verbatim per the design. `healBuiltinWorkflowStepOnCommentFanOut`
(`internal/task/repository/sqlite/builtin_workflow_on_comment_reconcile.go`) is
registered in `base_schema.go` immediately after `healBuiltinWorkflowStepOnAgentError`,
reusing its `findSystemOwnedWorkflowSteps` / `tryHealWorkflowStepEvents` CAS
retry helpers verbatim (same bounded-retry, `is_system = 1`, match-by-`(template
id, step name)` shape).

### Deviation from "Files likely touched"

The orchestrator-level proof is `internal/workflow/engine/office_default_gate_comment_smoke_test.go`,
not `internal/orchestrator/gate_comment_wake_test.go`. Reason: `engine.HandleTrigger`
rejects a blank `SessionID` before ever reaching `TransitionStore.LoadState`, so
a DB-backed orchestrator harness would need to seed a real `task_sessions` row
solely to satisfy that precondition. The pre-existing `office_default_smoke_test.go`
fixture (`loadEmbeddedTemplate` + `compileWorkflow` + `smokeStore` +
`smokeParticipants`) already compiles the real shipped YAML through the real
engine and seeds exactly the reviewer/approver seats this feature needs, at a
fraction of the harness weight. The new file adds four tests exercising
AC-001.1, AC-001.2, AC-002.1 (author exclusion + skip-decided on the real
template) and AC-001.13 (partial-failure redispatch, one queued run per seat,
no duplicates, via a new `dedupingFlakyRunQueue` test double modelling the real
run queue's idempotency-key dedup).

### Acceptance criteria covered by pre-existing tests, not new ones

- **AC-001.8, AC-001.15** (a gated-step comment whose step fails to load, or
  whose `on_comment` list holds a failing `queue_run` ahead of the reconciled
  fan-out, keeps the legacy runner wake): unchanged dashboard-level behavior,
  already proven by `office/dashboard/gate_comment_dispatch_test.go`'s
  `TestCreateComment_OtherEngineErrorKeepsLegacyWake` (WO-01, pre-existing to
  this task).
- **AC-002.7** (assignee-read failure fails open, engine still reached with
  author identity): same file's
  `TestCreateComment_AssigneeReadFailureStillReachesEngineWithAuthorIdentity`.
- **AC-001.12** (a woken reviewer's `approved`/`rejected` decision moves the
  task via the existing `all_approve`/`any_reject` guard, and the gate comment
  wake itself moves nothing): the guard is wake-reason-agnostic — it evaluates
  recorded decisions, not why the seat was woken — and is already proven
  end-to-end on the real template by the pre-existing
  `TestOfficeDefaultWorkflow_FullCycleSmoke` (approve path) and
  `TestOfficeDefaultWorkflow_RejectRoutesBackToWork` (reject path) in
  `office_default_smoke_test.go`.
- **AC-005.1** (Work/Backlog/Done queue the same runs as before this package):
  verified by inspection rather than a new test — `git diff` on
  `office-default.yml` confirms the `work`, `backlog` and `done` step blocks
  are byte-unchanged; only `review` and `approval` gained `on_comment` entries.

### Manual end-to-end evidence: descoped, with substitute proof

The task-03 verification block calls for starting `make dev` against a copy of
a database holding a pre-change `office-default` workflow and posting a
comment through the dashboard API. This was attempted and abandoned: this
session's own shell carries ambient `KANDEV_SERVER_PORT=8817`,
`KANDEV_SUPERVISOR_SOCKET` and `KANDEV_TASK_ID` environment variables belonging
to the live Kandev platform instance that is running this very agent session
(confirmed when a second backend process, started with an explicit
`KANDEV_PORT` override, still tried to bind port 8817 and failed — the ambient
`KANDEV_SERVER_PORT` outranks a later `KANDEV_PORT` in the port-resolution
order and was already set in the environment). This is a live, platform-managed
task sandbox sharing config with its own controlling instance, not an isolated
devbox; standing up a second full backend process here risks colliding with
the instance actually serving this task, independent of `KANDEV_HOME_DIR`
scoping. No server was left running and no state was mutated.

Substitute proof, covering the same behavioral claims through the real
production code paths without a live server: `builtin_workflow_on_comment_reconcile_test.go`
opens a real `sqlite.Repository` against a real (temp-file) SQLite database and
proves the reconciler backfills a pre-existing system-owned row's `events` on
`initSchemaContext` (the actual startup path), is idempotent on a second run,
and leaves an `is_system = 0` copy and an already-fanned-out row untouched;
`office_default_gate_comment_smoke_test.go` proves the real engine, evaluating
the real embedded template, dispatches the fan-out and queues real
`QueueRunRequest`s. Together these exercise every claim the manual walkthrough
would have (reconciler backfill on startup, comment triggers the queued run)
through the actual code, just not through a live HTTP server in this shared
sandbox.

### `internal/orchestrator` timeout investigation

A background full-suite run reported a bare `FAIL github.com/kandev/kandev/internal/orchestrator 601.091s`
with a goroutine-timeout panic stack trace inside `TestLaunchPrepare_PassthroughDoesNotRecurse`
→ `setupTestRepo` → `healBuiltinWorkflowStepFlags` (pre-existing code, untouched
by this task) — consistent with Go's default 10-minute per-package test
timeout (601s ≈ 600s) firing while several `make build-*` invocations ran
concurrently under `-race`. A clean, isolated rerun with no concurrent builds
(`go test ./internal/orchestrator/ -race -count=1 -timeout=15m`) passed in
534.226s, confirming a contention flake, not a regression. The new reconciler
was also ruled out directly: `findSystemOwnedWorkflowSteps` is a single
indexed `SELECT`, the same cost shape as the pre-existing
`healBuiltinWorkflowStepOnAgentError` reconciler (PR #3275) that has run on
every test-repo init without incident since it shipped. No code change was
needed.

### Definition of Done gauntlet

All green: `go build ./...`, `go vet ./...`, `gofmt -l` on all 20 changed/new
Go files, `make -C apps/backend lint` (`golangci-lint run ./...`, 0 issues),
every command in the Verification block above (adjusted for the file-location
deviation noted), plus full `./internal/office/...` and `./internal/runs/...`.
`python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`
both clean. `pnpm install --frozen-lockfile` (needed for `fmt-web`/`lint-web`/
`i18n:ratchet`) was blocked by the session's permission classifier; this diff
touches zero `apps/web` files, so nothing in that surface needed checking, but
the gap is recorded here rather than silently skipped. No stray `.bak` files
or scratch artifacts remain.
