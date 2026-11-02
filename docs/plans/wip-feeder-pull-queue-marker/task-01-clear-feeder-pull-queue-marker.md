---
id: "01-clear-feeder-pull-queue-marker"
title: "Clear the queue marker on engine feeder pulls"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001
acceptance_criteria:
  - AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.2
system_design:
  - ../../specs/tasks/system-design/wip-limit-pull-system.md
---

# Clear the queue marker on engine feeder pulls

## Summary and scope

`workflowStore.pullOneFeederTask` must promote a feeder-queued task as admitted
with no queue marker, so the destination's entry behavior runs after admission.

## Owned files

- `apps/backend/internal/orchestrator/workflow_store.go`
- `apps/backend/internal/orchestrator/workflow_store_test.go`

## Implementation acceptance

1. After an engine transition vacates a limited step with a feeder, the pulled
   task is in the destination with `wip_admitted = true`, empty
   `queued_for_step_id`, and nil `queued_at`.
2. The atomic promotion predicate is unchanged; it still matches the stored
   queue marker, so concurrent pulls cannot overfill the step.

## Verification

```bash
(cd apps/backend && go test ./internal/orchestrator -run 'TestWorkflowStore_' -count=1)
git diff --check
```

## Results

- RED: `TestWorkflowStore_ApplyTransitionFeederPullClearsQueueMarker` failed with
  `pulled task kept queue marker: queued_for_step_id="step-limited"`.
- GREEN: the same test passes after the fix. The focused orchestrator run
  (`-run 'TestWorkflowStore_|Feeder|Pull|Queue|WIP'`) passes except
  `TestHandleAgentReady_PassthroughQueuedAttachmentUsesAttachmentAwarePrompt`,
  which fails on Windows because the attachment path uses `\`; it does not touch
  WIP code.
- Changed-package `golangci-lint --new-from-rev=HEAD` for `internal/orchestrator`.
