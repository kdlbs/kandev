---
id: "01-label-resume-answers"
title: "Label resume answers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-ANSWER-LABELS-001
acceptance_criteria:
  - AC-TASKS-QUESTION-ANSWER-LABELS-001.5
system_design:
  - ../../specs/tasks/system-design/user-question-answer-labels.md
---

# Task 01 — Label resume answers

## Summary

Use the original options to make accepted clarification selections readable
in resume prompts. The summary carries each selected ID beside its label.

## Scope

- Use `bundleQuestions` inside `resolveClarificationEventContext` for persisted
  questions. Remove the superseded prompt-only parser and update its references.
- Use one selection formatter for single-question and bundle answers. Use
  options only from the question whose nonempty ID matches the answer.
- The resolver builds answer text for `DetachedClarificationResume` and the
  primary-answer watchdog. Both use the orchestrator's existing prompt builder.
  The live tool result uses the MCP server's JSON projection.
- Document resume prompt labels in the agent communication reference.

## Acceptance

1. After an active bundle loses its waiter, an accepted answer reaches the
   orchestrator once. Each selection in the captured request pairs its ID with
   the label from its own question. A bundle follows the original question order.
2. When saved options provide no unique, nonempty label, the summary shows the
   ID alone.

## Verification

First add `TestResolverDetachedResumeIncludesSelectedLabels` and run the first
command to record the expected label failures. Use
`newResolverDeliveryFixture` with no store entry and persisted options.
Assert one accepted `DetachedClarificationResume` and its `AnswerText`.
Supply bundle rows and answers out of order. Share an option ID across two
questions with different labels to exercise question-level matching.
Add a case with an empty label to exercise the ID fallback.

After the change, run the commands below from the repository root.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/clarification -run '^TestResolverDetachedResumeIncludesSelectedLabels$' -count=1 -v)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/clarification -count=1)
bash scripts/lint-go-changed \
  apps/backend/internal/clarification/resolver.go \
  apps/backend/internal/clarification/bundle_question.go \
  apps/backend/internal/clarification/bundle_question_test.go \
  apps/backend/internal/clarification/answer_annotations_test.go \
  apps/backend/internal/clarification/resolver_answer_labels_test.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/clarification/resolver.go`
- `apps/backend/internal/clarification/bundle_question.go` and
  `apps/backend/internal/clarification/bundle_question_test.go` for parser comments
- `apps/backend/internal/clarification/answer_annotations_test.go` for the
  answer line expectation
- `apps/backend/internal/clarification/resolver_answer_labels_test.go` for the
  new regression tests
- `docs/public/agent-communication.md`
- This work order and its plan

## Dependencies

None.

## Risks

Old messages can have incomplete options. Use the stored ID when the formatter
cannot resolve a label.

## Parallelism

`sequential`

## Inputs

- [Answer label requirements](../../specs/tasks/requirements/user-question-answer-labels.md)
- [Answer label system design](../../specs/tasks/system-design/user-question-answer-labels.md)
- [Active clarification lifecycle](../../specs/tasks/system-design/clarification-active-lifecycle.md)
- `apps/backend/internal/clarification/resolver_delivery_test.go` and
  `apps/backend/internal/clarification/answer_annotations_test.go`

## Results

Implemented persisted option labels in detached resume summaries. The
regression test checks single and bundled answers, bundle and answer reordering,
identical option IDs with question-specific labels, empty and ambiguous label
fallbacks, and one accepted resume. The public agent communication reference
now documents the resume format.

The focused test, clarification package race test, package build, changed-file
Go lint, public-doc checks, specification checks, and `git diff --check` passed.
