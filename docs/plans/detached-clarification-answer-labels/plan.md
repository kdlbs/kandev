---
created: 2026-11-23
status: done
requirements:
  - REQ-TASKS-QUESTION-ANSWER-LABELS-001
system_design:
  - ../../specs/tasks/system-design/user-question-answer-labels.md
legacy_specs: []
---

# Implementation plan for detached clarification answer labels

## Overview

Restore the saved option labels when an agent receives a clarification answer
as a resume prompt. The Tasks system owns the association between saved
questions and answers. The live tool result already returns the selected label
with the ID. [Issue #5447](https://github.com/kdlbs/kandev/issues/5447),
[live tool result fix #4752](https://github.com/kdlbs/kandev/pull/4752).

## Reproduction

On `main` at `a2ef38bbfcf`, a temporary Go overlay ran
`TestResolverDetachedResumeIncludesSelectedLabels` against the real resolver.
The fixture supplied current-turn messages with stored options and an empty
waiter store. The test captured one accepted resume request per case.

The single-question case returned `User selected: [q1_opt2]`. The bundle
returned `A1: [q1_opt2]` and `A2: [q2_opt2]` in question order. Both failed
the label assertion. The fixture modeled the state after a lost waiter.

## Scope

The work order covers the resolver text used for direct detached resumes and
for watchdog recovery. The active lifecycle governs the current-turn claim
and resume acknowledgement.
[Active lifecycle design](../../specs/tasks/system-design/clarification-active-lifecycle.md).

Answers to superseded questions follow ordinary message admission.
[Late clarification messages ADR](../../decisions/2026-09-18-late-clarification-messages.md).

## Technical approach

Before implementation, `resolveClarificationEventContext` used
`questionsFromMessages`, which copied prompts and IDs but skipped the stored
options. `buildAnswerSummary` then printed `SelectedOptions` as IDs. The
implementation now reuses `bundleQuestions` and formats labels beside IDs.

1. Reuse `bundleQuestions` for persisted question recovery. Validation already
   uses this reader for option membership and bundle order.
2. Match the answer's question ID before looking up each selected option.
   Put the label and ID in the existing single-question or bundle summary.
   Use the ID alone for missing or ambiguous labels.
3. Add regression coverage before the formatter change. Document the prompt
   during implementation. [Agent communication reference](../../public/agent-communication.md).

The proposed single-question output is `User selected: ["Run checks" (q1_opt2)]`.

## Tests

The requirement is
`REQ-TASKS-QUESTION-ANSWER-LABELS-001`. [Acceptance criteria](../../specs/tasks/requirements/user-question-answer-labels.md).

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-TASKS-QUESTION-ANSWER-LABELS-001.5` | `TestResolverDetachedResumeIncludesSelectedLabels` checks one question, a bundle, and a selection with no usable label. |

The tests belong in `apps/backend/internal/clarification/resolver_answer_labels_test.go`.
Existing resolver, orchestrator, and MCP tests cover delivery and live results.

## Work orders

- [x] [Task 01 — Label resume answers](task-01-label-resume-answers.md) (`done`)

## Verification results

- The temporary overlay reproduced both failures and was removed after the run.
- Existing resolver tests for live delivery, detached resume, and annotation
  formatting passed.
- The specification catalog, 36 specification-linter tests, and full
  specification lint passed.
- The PR documentation coverage validator returned `covered` for the
  planned resolver changes and this package.
- `TestResolverDetachedResumeIncludesSelectedLabels` failed before the resolver
  change with ID-only output, then passed for single and bundled questions. It
  checks question-specific labels, bundle order, and ID fallback for empty and
  ambiguous labels.
- The clarification package race test and package build passed.
- Public documentation tests and validation passed. Specification catalog
  validation, specification linter tests, and full specification lint passed.
- Changed-file Go lint, `git diff --check`, and the package build passed.

## Risks

An option ID can appear in more than one question. Match the question ID first
so each label comes from the correct question.
