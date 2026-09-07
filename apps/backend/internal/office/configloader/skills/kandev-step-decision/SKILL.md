---
name: kandev-step-decision
description: Record the current workflow step decision through the task-bound Office CLI.
kandev:
  system: true
  version: "0.42.0"
---

# Workflow step decisions

Use the task-bound CLI command when you hold the current review or approval
seat and the run prompt advertises the decision action.

## Record the decision

The only accepted verdicts are `approved` and `rejected`. Use a non-empty reason
that explains the evidence for the verdict.

```bash
$KANDEV_CLI kandev task decision --decision approved|rejected --reason "..."
```

Replace the verdict and reason with the decision for the current workflow
step. Make this command the final action for the turn, then stop. A repeated
decision supersedes the previous decision for that participant and step.

The command returns structured JSON with these fields:

```json
{
  "decision": "approved",
  "role": "reviewer",
  "step_id": "step-1",
  "decision_id": "decision-1",
  "decided_at": "2026-01-01T00:00:00Z",
  "transition_applied": false,
  "guards": []
}
```

Posting comments do not count as a workflow step decision. Approval-inbox commands do not count
either. Do not use either operation instead of the task decision command.
