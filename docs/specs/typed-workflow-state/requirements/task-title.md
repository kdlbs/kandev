---
status: draft
system: typed-workflow-state
created: 2026-10-08
owners:
  - kandev
---

# Task title placeholder requirements

A server-substituted task title in workflow prompt templates, so a step or
workflow prompt can name the task it is working on. System-wide terminology,
non-functional constraints and exclusions are in [../README.md](../README.md).

## Why

Step prompts inject the task description through `{{task_prompt}}`, but the
task's title never reaches the prompt pipeline. When a description is short or
terse, the agent loses the context a human gets from the task name.
`{{task_prompt}}` cannot carry the title instead: the same base prompt is reused
for ordinary chat messages during a step, so a title prepended there would leak
into unrelated turns.

## Requirements

### REQ-TWS-006: Task title in workflow prompt templates

Workflow prompt templates can request the task's title, and the server
substitutes it.

- **AC-TWS-006.1:** When a workflow prompt template contains the token
  `{task_title}`, the built prompt shall contain the task's title in its place,
  with no braces. Every occurrence of the exact literal shall be replaced.
- **AC-TWS-006.2:** Substitution shall apply at both production call sites (the
  step prompt and the workflow-level prompt), so a template works wherever it is
  authored.
- **AC-TWS-006.3:** When the template does not contain `{task_title}`, no task
  lookup shall be issued and the built prompt shall be byte-for-byte identical to
  its output before this change (NFR-3).
- **AC-TWS-006.4:** When the task lookup fails, the token `{task_title}` shall be
  left in the prompt verbatim, the failure shall be logged at warn level with the
  task identifier, and the rest of the prompt shall be built and delivered
  unchanged.
- **AC-TWS-006.5:** When the task identifier is empty, no lookup shall be issued
  and the token shall be left verbatim.
- **AC-TWS-006.6:** The token is resolved against the templates only. A literal
  `{task_title}` inside the base prompt (task description or direct message)
  shall never be substituted.
- **AC-TWS-006.7:** The title is inserted as plain text. A title that itself
  contains `{task_id}`, `{step_entry_number}`, `{{task_prompt}}` or
  `{task_title}` shall appear verbatim and shall not be expanded, and
  `{{task_prompt}}` shall still be replaced at the template's own first
  occurrence.
- **AC-TWS-006.8:** `{task_id}`, `{step_entry_number}` and `{{task_prompt}}`
  substitution shall be byte-for-byte unchanged, including the replacement of
  only the first `{{task_prompt}}` occurrence.
- **AC-TWS-006.9:** Saved-prompt references (`@name`) in the title shall be
  treated exactly as they are in the base prompt: the substituted title is part
  of the prompt handed to saved-prompt expansion, which runs once over the
  assembled prompt.

## Verification

One line per case; the cited AC is authoritative for the expected value.

1. A template with the token renders the task's title at every occurrence (006.1).
2. Both the step template and the workflow-level template substitute (006.2).
3. A template without the token issues no lookup, asserted against a counting
   fake (006.3).
4. A lookup error leaves the token literal, logs one warning with the task
   identifier, and still returns the rest of the prompt (006.4).
5. An empty task identifier leaves the token literal with no lookup (006.5).
6. A base prompt containing `{task_title}` keeps it literal (006.6).
7. A title containing `{{task_prompt}}`, `{task_id}` and `{step_entry_number}`
   renders verbatim on both call sites, with the description at the template's
   own `{{task_prompt}}` (006.7).
8. All four placeholders in one template resolve, and a second `{{task_prompt}}`
   stays literal (006.8).
9. The existing `{step_entry_number}` and workflow prompt tests pass unchanged
   (006.8).
10. A direct message appended to a step prompt receives no substitution, and
    `skip_step_prompt` still substitutes the workflow-level block (006.2, 006.6).
11. Workflow entry renders the title (006.1, 006.2).
12. A title containing the workflow-instructions end marker cannot close the
    workflow block early (006.7).
13. An empty title substitutes as an empty string without a warning (006.1).
14. `@` references in the title reach saved-prompt expansion together with the
    base prompt (006.9).
