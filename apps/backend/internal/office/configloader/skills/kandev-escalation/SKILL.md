---
name: kandev-escalation
description: How to create a human-actionable decision task when you are blocked
kandev:
  system: true
  version: "0.42.0"
  default_for_roles: [worker, specialist, assistant, reviewer]
---

# Escalating to a Human

Use this pattern when you cannot proceed without human input: a design decision,
missing credentials, ambiguous requirements, or access you do not have.

## When to escalate

Escalate when ALL of the following are true:
1. You cannot make the decision yourself based on available context.
2. The decision is required to complete your task.
3. You have already checked comments and memory for prior guidance.

Do NOT escalate for routine technical choices you can make independently.

## Escalation procedure

### Step 1: Create the human task

Create a new task with a clear question as the title. Leave it unassigned
(or assign to `$KANDEV_HUMAN_USER_ID` if that variable is set in your environment).

```bash
HUMAN_TASK=$(
  $KANDEV_CLI kandev task create \
    --title "Decision needed: <your specific question here>" \
    --description "Context: <1-2 sentences of background>

Question: <the specific decision the human needs to make>

Options considered:
- Option A: <brief description>
- Option B: <brief description>

Blocked task: $KANDEV_TASK_ID" \
    2>/dev/null
)
HUMAN_TASK_ID=$(echo "$HUMAN_TASK" | jq -r '.id')
```

### Step 2: Block your task on the human task

```bash
# Link: your task is blocked by the human task
$KANDEV_CLI kandev task update --id "$KANDEV_TASK_ID" \
  --add-blocker "$HUMAN_TASK_ID"
```

### Step 3: Post a comment and set blocked status

```bash
$KANDEV_CLI kandev comment add --body "Escalated to human: created task $HUMAN_TASK_ID. Waiting for decision on: <question>"

$KANDEV_CLI kandev task update --status blocked
```

### Step 4: Exit

Your session ends. The orchestrator will wake you with reason
`task_blockers_resolved` when the human closes the decision task.

## On wakeup after resolution

When you wake with `KANDEV_WAKE_REASON=task_blockers_resolved`, parse
`$KANDEV_WAKE_PAYLOAD_JSON` for the resolved blocker titles, then continue
your work incorporating the human's decision (visible in the resolved task's
comments or description).

## Rules

- Keep the question in the title specific and actionable. "Decision needed: use PostgreSQL or SQLite for the cache layer?" is good. "I need help" is not.
- Include enough context in the description that the human can decide without reading your full task history.
- One escalation per decision. Do not create multiple human tasks for the same question.
- If you need to escalate multiple independent decisions at once, create one human task per decision.
- If you can make a reasonable default choice, prefer that over escalating.
