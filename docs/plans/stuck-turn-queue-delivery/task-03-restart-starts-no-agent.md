---
id: "03-restart-starts-no-agent"
title: "Pin that a restart starts no agent"
status: done
wave: 1
depends_on: ["02-drain-restored-send-now"]
plan: "plan.md"
requirements:
  - REQ-TASKS-RESUME-PROMPT-QUEUE-003
acceptance_criteria:
  - AC-TASKS-RESUME-PROMPT-QUEUE-003.1
  - AC-TASKS-RESUME-PROMPT-QUEUE-003.2
  - AC-TASKS-RESUME-PROMPT-QUEUE-003.3
system_design:
  - ../../specs/tasks/system-design/resume-prompt-queue.md
---

# Task 03: Pin that a restart starts no agent

## Summary

Add regression coverage for the existing rule: backend startup starts no agent
for queued work, and opening the task delivers it. This keeps Task 02's new drain
and any later change from turning a restart into automatic agent starts.

## In scope

- Test startup with pending queued entries, Auto-run ON, and a restored Send Now claim.
- Test that a later session resume and readiness delivers each entry once.

## Out of scope

- Any startup drain, sweep, or resume.
- Production changes, unless the test finds a startup path that starts an agent.
  In that case, stop and report it.

## Acceptance

- After startup, no launch or resume happens and the entries, order, and Auto-run are unchanged.
- After resume and readiness, each entry is delivered once.

## Verification

```bash
(cd apps/backend && env -u KANDEV_DEBUG_AGENT_MESSAGES go test -trimpath ./internal/orchestrator -run 'QueueRestart' -count=1)
```

## Files likely touched

- `apps/backend/internal/orchestrator/queue_restart_no_autostart_test.go`

## Dependencies

Task 02, so the coverage includes the restored-claim case.

## Risks

None

## Parallelism

`sequential`

## Inputs

- [Backend restart](../../specs/tasks/system-design/resume-prompt-queue.md#backend-restart)
- `background_work_settlement_test.go` for startup fixtures

## Results

Done. `queue_restart_no_autostart_test.go` pins that startup restores a claimed
entry without prompting, launching, or marking a drain, and that Auto-run, the
entries, and their order survive the restart. A second test delivers each entry
once across successive readiness boundaries. No production startup path needed
changing.
