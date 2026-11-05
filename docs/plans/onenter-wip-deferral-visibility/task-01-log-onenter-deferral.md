---
id: "01-log-onenter-deferral"
title: "Log the on-enter deferral"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001
acceptance_criteria:
  - AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.3
  - AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.11
system_design:
  - ../../specs/tasks/system-design/wip-limit-pull-system.md
---

# Task 01: Log the On-Enter Deferral

## Change

In `apps/backend/internal/orchestrator/event_handlers_workflow.go`, report the
task, session, source and destination steps, destination WIP limit, and
`wip_limit_reached` after a session-lifecycle transition commits but before
returning with its destination entry deferred. Carry the admission decision and
limit from the commit result into the lifecycle. If a compatibility commit does
not return admission state and the task reread fails, log the error and stop
before destination entry. Keep queue placement and promotion unchanged, and do
not use the session log for guarded decisions.

## Validation

The contributor recorded the initial scenario as a disposable Go overlay:
unmodified upstream failed because the deferral log was missing, and the
candidate passed while preserving queued placement and deferred entry. The
contributor also reported that existing guarded-decision lifecycle tests,
specification validation, and normal commit hooks passed. The temporary overlay
was removed after that initial validation.

The review follow-up adds permanent coverage in
`apps/backend/internal/orchestrator/event_handlers_workflow_wip_deferral_test.go`.
It covers the log message, level, and fields for turn-start and turn-complete
transitions. It also covers queued placement, deferred entry, and an unchanged
decider session without the session log for guarded decisions. An admitted
destination provides a positive control for entry without a deferral log. A
concurrent limit change proves the log reports the limit used by admission. A
failed fallback reread proves the orchestrator logs the error and does not run
destination entry.

The focused regression command passed:

```bash
GOMAXPROCS=2 go test -p 1 ./internal/orchestrator -run 'TestWorkflowTransition_(WIPDeferralLog|AdmittedDestinationDoesNotLogWIPDeferral|WIPDeferralLogUsesAdmissionLimit|PostCommitTaskReloadFailureWarnsAndDefersEntry)$' -count=1
```

The full orchestrator package test command also passed:

```bash
GOMAXPROCS=2 go test -p 1 ./internal/orchestrator -count=1
```

The changed-file Go lint passed with no issues:

```bash
bash scripts/lint-go-changed apps/backend/internal/orchestrator/event_handlers_github_pr_workflow.go apps/backend/internal/orchestrator/event_handlers_workflow.go apps/backend/internal/orchestrator/event_handlers_workflow_wip_deferral_test.go apps/backend/internal/orchestrator/workflow_store.go
```

Documentation and specification validation passed:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```
