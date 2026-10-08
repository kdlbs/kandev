---
id: "01-step-entry-causation-from-ledger"
title: "Resolve step-entry wake causation from the step-transition ledger"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-RUN-CAUSATION-001
  - REQ-OFFICE-LAUNCH-SAFETY-003
acceptance_criteria:
  - AC-OFFICE-RUN-CAUSATION-001.3
  - AC-OFFICE-RUN-CAUSATION-001.9
  - AC-OFFICE-RUN-CAUSATION-001.25
  - AC-OFFICE-LAUNCH-SAFETY-003.2
system_design:
  - ../../specs/office/system-design/unattended-launch-safety-01.md
---

# Task 01: Resolve step-entry wake causation from the step-transition ledger

## Scope

- Carry the step-transition ledger id on step-entry `queue_run` and
  `queue_run_for_each_participant` requests (ledger `DispatchStepEntry`,
  including the marker-bearing path) and on the Office `auto_start_agent`
  enqueue.
- In the workflow-engine run-queue adapter, when the ledger id is present, read
  the `task_step_transitions` row. Office task launches carry their trusted causing
  run ID through lifecycle events into that row. Run-owned completions use their
  owner run ID. An `agent` row with `causing_run_id` resolves that exact run, without
  relying on mutable session binding or claim fields. A legacy row without the ID
  uses the session-and-time lookup. A `human` row yields a `user` actor with the
  human-rooted flag and no creating run. Any other actor falls back to the existing
  task-boundary carrier.
- Preserve existing trigger and actor semantics for all other workflow transitions.

## Acceptance

A review, reject, rework, review loop driven by agent `step_complete` calls
queues each successive step-entry run as a child of the previous run in one
chain, with `causation_depth` increasing by one per hop. The loop is refused at
the configured `office.maxCausationDepth`. A human board move queues a
`user`-actor, human-rooted root run at depth `0`.

## Verification

```bash
cd apps/backend && go test ./internal/steptelemetry ./internal/task/repository/sqlite \
  ./internal/runs/repository/sqlite ./internal/office/repository/sqlite \
  ./internal/office/service ./internal/orchestrator/watcher ./internal/orchestrator
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
```
