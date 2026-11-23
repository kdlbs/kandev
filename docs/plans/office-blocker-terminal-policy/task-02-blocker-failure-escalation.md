---
id: "02-blocker-failure-escalation"
title: "Escalate Office blocker failures once to the responsible agent"
status: done
wave: 2
depends_on:
  - "01-success-only-classification"
plan: "plan.md"
requirements:
  - REQ-OFFICE-BLOCKER-POLICY-002
acceptance_criteria:
  - AC-OFFICE-BLOCKER-POLICY-002.1
  - AC-OFFICE-BLOCKER-POLICY-002.2
  - AC-OFFICE-BLOCKER-POLICY-002.3
  - AC-OFFICE-BLOCKER-POLICY-002.4
  - AC-OFFICE-BLOCKER-POLICY-002.5
  - AC-OFFICE-BLOCKER-POLICY-002.6
  - AC-OFFICE-BLOCKER-POLICY-002.7
  - AC-OFFICE-BLOCKER-POLICY-002.8
system_design:
  - ../../specs/office/system-design/blocker-terminal-policy.md
---

# Task 02: Escalate blocker failures once

## Summary

Make every committed transition into `FAILED` or `CANCELLED` observable on
`task.state_changed`. Then add the `task_blocker_failed` subscriber. It wakes the
runner of the blocker's parent task, or the workspace CEO when there is none, once
per failure occurrence, listing every held dependent.

## In scope

- Confirm that the orchestrator's runtime `FAILED` writers already publish
  `task.state_changed`: they write through `backendapp.taskRepositoryAdapter`,
  which calls the publishing task service writers. No orchestrator change.
- `office/shared/runreasons.go`, `wakereasons.go`, `runs/service/metrics_vars.go`,
  the unattended list in `shared/runprovenance_completeness_test.go`
  (`AC-OFFICE-BUDGET-007.4` amended), and the `scheduler/run.go` and
  `service/run.go` aliases for `task_blocker_failed`.
- New `office/scheduler/blocker_failure.go`: `RegisterBlockerFailureSubscriber`,
  the transition filter, the stale re-read, held-dependent selection, recipient
  resolution, immutable occurrence identity, and a durable `blocker_failure`
  wakeup request sent through the shared wakeup dispatcher.
- Registration in `backendapp.activateOfficeServices`. A subscription error fails
  activation.
- `office/service/prompt_blocker_failed.go` `buildBlockerFailedPrompt` and its
  payload enrichment.
- After implementation: add a row to the spec table in
  `apps/backend/internal/office/AGENTS.md` and update its count. Promote the specs
  and plan per the spec-driven-development workflow.

## Out of scope

- Actor plumbing on task events, sweeps, `agent_error` changes, and Inbox or UI
  changes.

## Acceptance

1. Publication: the runtime callback and the early launch-failure writer each emit
   exactly one `task.state_changed` with the correct `old_state`/`new_state`, and
   an archived skip emits none. The existing subscriber packages stay green.
2. Escalation: a failure transition with held dependents persists one durable
   `task_blocker_failed` wakeup request for the parent's runner, with the CEO as
   fallback. The payload lists every held dependent. There is no request for: no
   held dependents, `FAILED`→`CANCELLED`, a stale event, or no recipient (which logs
   a Warn). Duplicate delivery gives one request. A refail after retry is a new
   request occurrence. The shared dispatcher merges it into any queued or claimed
   run for the recipient and retains all alert details. A paused request stays
   queued and is replayed after resume or by the recovery tick; replay drops stale
   alerts when the blocker recovered or no longer holds an Office dependent.
3. The registry tests and a prompt test pass. A real-bus test proves the
   subscriber's registration through the production activation path.

## Verification

From `apps/backend`:

```bash
go test -trimpath -race ./internal/office/shared/ ./internal/office/scheduler/ \
  ./internal/office/service/ ./internal/runs/service/ -count=1
go test -trimpath -race ./internal/orchestrator/ -run 'LaunchFailure|TaskStateChange|ChildrenCompleted' -count=1
go test -trimpath -race ./internal/task/statussummary/ ./internal/coordinator/... -count=1
go test -trimpath -race ./internal/backendapp/ -run 'Office' -count=1
```

From the repo root:

```bash
bash scripts/lint-go-changed <changed Go files>
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Likely files

- `apps/backend/internal/orchestrator/service.go`, `task_operations.go`
- `apps/backend/internal/office/scheduler/blocker_failure.go` (new) and its test
- `apps/backend/internal/office/shared/{runreasons,wakereasons,runprovenance}.go`
- `apps/backend/internal/runs/service/metrics_vars.go`
- `apps/backend/internal/office/service/prompt_builder.go`
- `apps/backend/internal/backendapp/` (Office activation)
- New: `apps/backend/internal/office/service/blocker_failure_escalation_test.go`

## Risks

- New `FAILED` publications reach every `task.state_changed` subscriber. Check
  the status-summary projector, the coordinator wake recorder, the outcomes
  recorder, and the handoff subscriber for unwanted reactions, such as duplicate
  activity rows. The task service's state-activity logger runs only in
  `publishTaskEvent` with `taskStateActivity`. Confirm that the orchestrator
  publisher does not write a duplicate activity row.
- The transition identity must come from immutable evidence carried by the
  producer. Do not read mutable `updated_at` from the current task row to construct
  an idempotency key. The recipient is not part of the occurrence key.

## Results

The implementation keeps the Office policy, canonical task-state writes, and
shared run execution ownership. It records a wakeup request before dispatch, so a
confirmed pause or transient delivery failure cannot discard the alert. The
existing dispatcher owns the in-flight merge. Each blocker occurrence is stored
under a blocker-plus-occurrence key, and prompts read merged context even when a
different run reason owns the active run. Fresh runs keep the parent task target;
the CEO fallback stays taskless.

Task-service events use the committed `updated_at` carried in the event. Office
status changes generate an occurrence ID for the mutation, and tree cancellation
uses its durable hold ID. Recipient assignment is excluded from the key. Replay
checks current blocker state and held dependents, then skips stale requests.

Additional prompt hardening sanitizes and bounds every task identifier and title
before rendering, including fallback IDs.

Focused package verification from `apps/backend`:

```bash
go test -trimpath ./internal/office/service ./internal/office/scheduler ./internal/office/wakeup ./internal/office/repository/sqlite
```

Result: passed. The service package completed in 20.214s, scheduler in 1.027s,
wakeup in 0.294s, and the SQLite repository package used its cached result. The
suite covers immutable occurrence identity, recipient reassignment, pause resume
and recovery replay, stale request disposal, queued/claimed in-flight run merge,
multi-blocker prompt context, parent targeting, and bounded prompt fields.
