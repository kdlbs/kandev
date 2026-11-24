---
status: current
system: tasks
requirements:
  - REQ-TASKS-QUESTION-ANSWER-LABELS-001
---

# User question answer labels

## Purpose and boundaries

The MCP server projects a confirmed clarification response into a tool result.
The clarification resolver builds answer text for the agent's resume prompt.
Both projections use the options from the original question.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-QUESTION-ANSWER-LABELS-001` | Answer projection, Resume prompt, Output contract, Failure behavior |

## Components and responsibilities

- `apps/backend/internal/mcp/server/handlers.go`: `parseQuestions` validates and
  normalizes each question and its options. `askUserQuestionHandler` retains
  those questions while the backend waits for the response.
- `extractQuestionAnswers` associates answers with question IDs and produces
  the structured result and equivalent JSON text.
- `simplifyAnswer` preserves the existing selection, custom text, and stub rules.
- `apps/backend/internal/mcp/server/server.go`: `askUserQuestionOutputSchema`
  declares the allowed answer properties. `registerInteractionTools` publishes
  the tool description and schema.
- `apps/backend/internal/clarification/resolver.go` builds the answer text in
  `buildAnswerSummary`. The text goes to detached resume and to the primary
  answer notification that supplies watchdog recovery.

## Answer projection

For each normalized question, associate its answer by question ID as today.
After simplification, resolve `selected_option` against that question's
normalized `options` by exact `option_id` equality. If exactly one option
matches and its label is a nonempty string, add that exact label.

Do not parse `q1_opt1`, use the answer array index, or search across questions.
This rule also handles explicit option IDs that the existing parser accepts.
Do not expose a new input field as part of this repair.

## Resume prompt

`resolveClarificationEventContext` uses the original questions while the
request is in memory. For persisted questions, `bundleQuestions` supplies the
IDs, prompts, and options in bundle order.

`buildAnswerSummary` associates each answer with its question by ID. The
formatter shows each resolved choice as a quoted label followed by the ID in
parentheses. Quoting keeps line breaks and delimiters inside the label.

The single-question summary can read `User selected: ["Run checks" (q1_opt2)]`.
Bundle answers use the existing `A1:`, `A2:` prefixes in question order.
When an answer has custom text, the summary uses that text. The file and
annotation summaries follow the answer.

The active clarification lifecycle governs resume admission.
[Active clarification lifecycle](clarification-active-lifecycle.md).

## Output contract

An example response is:

```json
{
  "flow": {
    "selected_option": "q1_opt1",
    "selected_label": "A"
  }
}
```

Add optional string property `selected_label` to `askUserQuestionOutputSchema`.
Its nested `additionalProperties: false` makes the schema update mandatory.
Describe both fields in the schema and the registered tool description.
Generate text and structured content from the same enriched map.

Existing clients can continue reading `selected_option`. Labels are descriptive
data, not stable identifiers. Duplicate labels are valid and do not replace IDs.

The issue proposes labels, caller-supplied IDs, or generated-ID documentation.
Adding a label meets the stated need without making callers encode or decode
option identity. This is a local additive response change, so the design and
work order preserve its rationale without a separate architectural decision.

## Failure behavior

For an unknown ID, a duplicate ID in the same question, or an empty label,
the tool result omits the label. The prompt shows the ID alone. When the
summary falls back to positional output, it shows the IDs alone.
Free-text-only entries and unanswered stubs use their existing form.
Rejected tool results retain their envelope and partial answers.

## Persistence, security, and observability

The normalized request already contains the labels shown to the user. Reuse
that data without adding persistent state or logging answer contents.
Existing permission checks, request retry identity, wait barriers, and delivery
confirmation remain authoritative. No new metrics are required.

## Verification boundary

Table-driven projection tests cover question-ID association, generated and
explicit option IDs, duplicate labels, unknown IDs, text, and rejection.
A registered-tool test reproduces the issue with question `flow` and A/B/C.
A Streamable HTTP test checks discovery and the final tool result through the
real MCP transport with a deterministic backend answer. Browser rendering is
outside this change. The transport test does not prove live provider behavior.

Resolver tests exercise one and several persisted questions with no live waiter.
They capture the text passed to the orchestrator and check that labels come from
the correct question. They also check that an unresolved label leaves the
selected ID in the summary.

## Related contracts

- [Requirements](../requirements/user-question-answer-labels.md).
- [Active clarification lifecycle](clarification-active-lifecycle.md).
- [Current-turn clarification ownership](../../../decisions/2026-08-14-current-turn-clarification-ownership.md).
- [Live tool result implementation plan](../../../plans/user-question-answer-labels/plan.md).
- [Resume prompt fix plan](../../../plans/detached-clarification-answer-labels/plan.md).
