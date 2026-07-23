---
name: kandev-escalation
description: Escalate to a human when required information, access, credentials, or a product decision blocks completion and no reasonable default is available.
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
HUMAN_TASK_ID=$(echo "$HUMAN_TASK" | jq -r '.task_id')
```

### Step 2: Cross-reference from the blocked task

```bash
$KANDEV_CLI kandev tasks message --id "$KANDEV_TASK_ID" \
  --prompt "Escalated to human task $HUMAN_TASK_ID. Waiting for decision on: <question>"
```

The Office CLI does not currently expose a secure operation for adding a blocker
relationship. The human task description links back to the blocked task, and the
comment above links the blocked task to the human task. Do not comment on the new
human task: a run may only message tasks granted by its signed runtime scope.
These references do not create a task dependency.

### Step 3: Set blocked status

```bash
$KANDEV_CLI kandev task update --status blocked
```

### Step 4: Exit

Your session ends. Because this flow does not create a blocker relationship, the
`task_blockers_resolved` wake reason will NOT occur for this manual cross-reference
flow. The human or coordinator must post the decision on the blocked task or update
it through the normal task workflow.

## On human response

Resume from the normal task comment or task update event. Parse
`$KANDEV_WAKE_PAYLOAD_JSON` first; if it is absent, read the JSON file at
`$KANDEV_WAKE_PAYLOAD_PATH`. Confirm the decision is present on the blocked task,
move it back to the appropriate active status, and continue the work.

## Rules

- Keep the question in the title specific and actionable. "Decision needed: use PostgreSQL or SQLite for the cache layer?" is good. "I need help" is not.
- Include enough context in the description that the human can decide without reading your full task history.
- One escalation per decision. Do not create multiple human tasks for the same question.
- If you need to escalate multiple independent decisions at once, create one human task per decision.
- If you can make a reasonable default choice, prefer that over escalating.
