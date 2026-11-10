---
status: active
system: tasks
created: 2026-09-12
updated: 2026-11-08
owners:
  - kandev
---

# Initial task brief requirements

## Overview

A first message sent to an ordinary task session must preserve the task
brief. The message supplements that brief unless its instructions explicitly
replace the objective. Both texts remain available to the user and agent.
Starting or recovering an agent without sending input does not consume this
first-message behavior.

The task system owns first-message admission and delivery. The existing UI
transcript contract owns history windows and the synthetic description row.

## Terms

- **Brief:** The task description at first-message acceptance.
- **Eligible session:** An ordinary session awaiting its first accepted or
  reserved user prompt, either prepared (`CREATED`) or ready for input
  (`WAITING_FOR_INPUT`). Readiness can result from recovery without prior input.
  Office, ephemeral Quick Chat, and configuration sessions are excluded.
- **Additional instruction:** A direct user message that starts or prompts an eligible session.
- **Explicit launch instruction:** Nonempty user input supplied to
  `session.launch` with `intent: start_created`. This input is distinct from an
  internal task-description fallback or an automatic workflow-entry prompt.

## Requirements

### REQ-TASKS-INITIAL-TASK-BRIEF-001: Initial task brief preservation

**Intent:** Preparatory instructions retain the objective, scope, and constraints
that the task already contains.

#### Acceptance criteria

- **AC-TASKS-INITIAL-TASK-BRIEF-001.1:** When an additional instruction starts or prompts an eligible session, its first agent prompt shall include the nonempty brief and instruction.
- **AC-TASKS-INITIAL-TASK-BRIEF-001.2:** The first stored user prompt shall retain both texts in order: brief, then additional instruction. Reloading shall preserve both texts.
- **AC-TASKS-INITIAL-TASK-BRIEF-001.3:** When a later message arrives, the system shall not automatically repeat the brief. Message deletion or backend restart shall not restore eligibility.
- **AC-TASKS-INITIAL-TASK-BRIEF-001.4:** When the instruction equals the brief after outer whitespace removal, the system shall include that text once. An empty brief adds nothing.
- **AC-TASKS-INITIAL-TASK-BRIEF-001.5:** Concurrent first-message submissions shall not both add the brief. Retrying an accepted message identity shall return its original saved content without another dispatch.
- **AC-TASKS-INITIAL-TASK-BRIEF-001.6:** When first-message persistence fails, the system shall not dispatch that message. A later retry shall remain eligible if no competing prompt won admission.
- **AC-TASKS-INITIAL-TASK-BRIEF-001.7:** Desktop and phone Chat shall show the preserved first prompt once, through existing history navigation, including after reload.
- **AC-TASKS-INITIAL-TASK-BRIEF-001.8:** Structured and passthrough sessions shall preserve the same visible brief and instruction. Existing attachment and saved-prompt delivery rules shall remain applicable.
- **AC-TASKS-INITIAL-TASK-BRIEF-001.9:** Office, Quick Chat, configuration, and previously prompted sessions shall retain their existing context rules. Automatic workflow entry shall retain its fallback rules.
- **AC-TASKS-INITIAL-TASK-BRIEF-001.10:** An explicit replacement instruction shall remain after the brief, preserving its precedence. The system shall not infer replacement from preparatory wording.
- **AC-TASKS-INITIAL-TASK-BRIEF-001.11:** When an eligible session becomes ready for input without accepting or reserving a prompt, including after a backend restart, its first subsequent direct message shall preserve the brief and instruction in both stored history and agent input. Creating or restoring the agent conversation alone shall not consume eligibility.
- **AC-TASKS-INITIAL-TASK-BRIEF-001.12:** Opening, preparing, or recovering an eligible session shall not send the brief solely to preserve first-message context. Existing explicit-start, workflow-entry, and recovery admission rules shall retain their authority.

#### Explicit prepared-session launch clarification

The following criteria extend the same initial-input preservation contract to
the launch surface. They do not add Chat's brief enrichment to every launch.
Implementation is recorded in the
[prompt integrity package](../../../plans/workflow-prompt-handoff-integrity/plan.md).

- **AC-TASKS-INITIAL-TASK-BRIEF-001.13:** When an explicit launch instruction starts a prepared session, its first agent input shall retain that instruction and applicable workflow guidance.
  This includes step templates that omit `{{task_prompt}}` and tasks with a retained plan.
- **AC-TASKS-INITIAL-TASK-BRIEF-001.14:** The corresponding stored user prompt, visible content, and raw content shall retain the explicit instruction.
  Server context shall preserve existing saved-prompt and attachment rules without duplicate workflow, plan-mode, canvas, or saved-prompt blocks.
- **AC-TASKS-INITIAL-TASK-BRIEF-001.15:** When admission defers an explicit launch, replay shall retain the same instruction and its explicit-input semantics.
  Existing cancellation, supersession, and provider-admission rules shall still apply.
- **AC-TASKS-INITIAL-TASK-BRIEF-001.16:** Empty launch input, internal task-description input, and automatic composed prompts shall retain their existing fallback and replacement rules.
  A request for no initial prompt shall create no prompt turn. Preserving explicit input shall not authorize a new task-description dispatch.

## Exclusions

- Rewriting old conversations or recovering previously omitted prompts automatically.
- Changing task descriptions when users send messages.
- New replacement controls or natural-language intent classification.
- Changes to workflow move instructions, context reset, or automatic entry policy.
- Treating display-only launch previews as authorized agent input.

## System design

[Initial task brief](../system-design/initial-task-brief.md)

## Implementation plans

[Initial task brief fix package](../../../plans/initial-task-brief/plan.md)

[Initial task brief after recovery](../../../plans/initial-task-brief-after-recovery/plan.md)

[Workflow prompt and handoff integrity](../../../plans/workflow-prompt-handoff-integrity/plan.md)
