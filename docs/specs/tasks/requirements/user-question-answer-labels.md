---
status: active
system: tasks
created: 2026-10-24
owners:
  - kandev
---

# User question answer labels

## Overview

An agent must identify the option a user selected without guessing the meaning
of a generated option ID. The Tasks system owns clarification answers and their
association with the original questions. Agent transports consume this contract.

## Requirements

### REQ-TASKS-QUESTION-ANSWER-LABELS-001: Readable selected answers

**Intent:** Return the selected option's displayed label with its existing ID.

#### Acceptance criteria

- **AC-TASKS-QUESTION-ANSWER-LABELS-001.1:** When an answer selects one uniquely
  identifiable offered option, `ask_user_question_kandev` shall return its exact
  label as `selected_label`. The answer shall retain `selected_option` and the
  original question key. This applies to every question in a bundle, including
  custom question IDs and options with identical labels.
- **AC-TASKS-QUESTION-ANSWER-LABELS-001.2:** The tool shall preserve custom text,
  unanswered markers, rejection markers, and rejection reasons. Free-text-only
  and unanswered entries shall not acquire a selected label. A partial answer
  in a rejected bundle shall retain the label of a uniquely identified selection.
- **AC-TASKS-QUESTION-ANSWER-LABELS-001.3:** If a selected ID cannot identify
  exactly one offered option with a nonempty label in that question, the result
  shall omit `selected_label`. It shall preserve existing answer fields and
  shall not infer a label from positions or another question.
- **AC-TASKS-QUESTION-ANSWER-LABELS-001.4:** The discovered output schema shall
  describe `selected_option` as an option ID and `selected_label` as its label.
  Structured results and the JSON text result shall contain equivalent answers.

## Out of scope

- Changing question input fields, generated IDs, choice cardinality, or validation.
- Changing question rendering, answer submission, persistence, or delivery authority.
- Changing parent-question, external answer-submission, or native provider protocols.

## Related contracts

- [Active clarification lifecycle](clarification-active-lifecycle.md).
- [User question turn boundary](user-question-turn-boundary.md).
- [System design](../system-design/user-question-answer-labels.md).
