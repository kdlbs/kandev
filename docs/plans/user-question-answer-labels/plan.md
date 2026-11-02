---
created: 2026-10-24
status: implemented
requirements:
  - REQ-TASKS-QUESTION-ANSWER-LABELS-001
system_design:
  - ../../specs/tasks/system-design/user-question-answer-labels.md
legacy_specs: []
---

# Implementation plan: readable user question answers

## Overview

Add `selected_label` to the MCP question result while preserving option IDs.
One sequential work order implements the response, tests, and public reference.
This package addresses [issue #4667](https://github.com/kdlbs/kandev/issues/4667).
The user authorized implementation on 2026-10-24. Work order 01 is complete.

## Investigation evidence

- Issue read on 2026-10-24: open, no comments, no image or file attachments.
  Its timeline links feature issue #4748, which proposes images beside questions.
  There is no linked fix PR. Searches for the issue and answer-label terms found
  no overlapping fix. PR #4723 was inspected and concerns automation parents.
- Workspace HEAD: `595df36ed5c1cf02fd90cb972b623ad6edc53df8`.
  Remote main: `0b81ae48d4e2c358f80ac489e39388dfd8d7e6ab`.
  The question formatter and output schema are unchanged between those commits.
  No reviewed answer-label package was found on either revision.
- Fetch reported an unrelated broken shared Git ref. The remote commit object
  was available locally afterward. Its exact source diff and history were read.
  No branch was moved and no shared ref was repaired.
- A temporary registered-tool test submitted question `flow` with A/B/C and a
  backend selection of `q1_opt1`. It verified the outgoing question retained
  label `A` and ID `q1_opt1`. The returned result was
  `{"flow":{"selected_option":"q1_opt1"}}`. Expecting `selected_label: "A"`
  failed. The temporary test was removed after diagnosis.

Confirmed root cause: `extractQuestionAnswers` calls `simplifyAnswer` without
using the original question's options. The simplified entry contains only the
selection ID and custom text. The output schema also forbids a label property.

The initial reproduction isolated the real MCP handler and mocked the backend
answer source. PR validation later exercised browser submission with the mock agent.

## Scope

### In scope

- Exact label lookup within the matching normalized question.
- Optional output property, discovery description, and equivalent result forms.
- Focused regression and transport coverage, plus public MCP documentation.

### Out of scope

- UI, native provider questions, parent questions, and external answer protocols.
- Input-schema changes, persistence, clarification lifecycle, and new feature flags.
- Commits, push, merge, issue comments, and issue closure without a delivery request.

## Technical approach

Follow the [system design](../../specs/tasks/system-design/user-question-answer-labels.md).
Modify `extractQuestionAnswers` in `handlers.go` to enrich each simplified entry
from its matching question. Match exact option IDs, not positional strings.
Add `selected_label` to `askUserQuestionOutputSchema` and explain it in the
registered tool description in `server.go`.

Place focused tests in `ask_user_question_answer_labels_test.go`. Reuse the
package's `testBackend`, `newTaskModeServer`, and transport fixture patterns.
Do not grow the already large `ask_user_question_test.go`. Update the exact-result
expectation in `tool_metadata_test.go` for the additive field.

During implementation, add an answer-result example to the operator-question
section of `docs/public/agent-communication.md`. That reference should distinguish
the returned label from the stable ID. The public reference update is part of the implementation.

## Tests

All new tests belong to `apps/backend/internal/mcp/server/ask_user_question_answer_labels_test.go`.

- `TestAskUserQuestionAnswerLabels`: AC 001.1 and 001.4. Cover A/B/C with
  custom question ID `flow`, generated IDs, explicit IDs, reordered backend
  answers, several questions, duplicate labels, and equal labels across questions.
- `TestExtractQuestionAnswerLabels`: AC 001.2 and 001.3. Cover selected option
  plus custom text, free text only, missing answers, rejected and partially
  answered bundles, unknown IDs, duplicate option IDs, and absent labels.
- `TestAskUserQuestionAnswerLabelsSchema`: AC 001.4. Assert the optional field
  and its description in the registered output schema.

## Protocol journey

`TestAskUserQuestionAnswerLabelsTransport` covers AC 001.1 and 001.4 through
Streamable HTTP: initialize, discover tools, call with A/B/C, receive the
backend's simulated first-option answer, and inspect both final result forms.
Use the existing `postJSONRPC` and HTTP server patterns. Bound requests and
register cleanup immediately. No production instance or real user prompt is needed.

This checks the agent-visible protocol journey. Existing desktop and phone
Playwright flows also verify the submitted IDs and labels. Live provider
interpretation remains outside this validation.

## Work orders

- [x] [Task 01: return selected labels](task-01-return-selected-labels.md)

## Verification results

Diagnosis: `go test -trimpath -tags fts5 ./internal/mcp/server -run
'^TestRepro4667SelectedLabel$' -count=1 -v` failed as expected, with actual label
`nil` versus expected `A`. The test ran from `apps/backend` and took 0.034s.
The initial sandbox run could not write the shared Go cache. The authorized
rerun used the existing cache and reached the expected assertion failure.

Design validation passed:

- `python3 scripts/list-docs.py validate`: 384 decisions and 1618 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check -- docs/specs docs/plans/user-question-answer-labels`: passed.
- The repository's `.github/scripts/pr-docs.cjs` `validateCoverage` API accepted
  the four package files against the planned `handlers.go` change. Result:
  `covered`, with no errors. The runtime path was supplied for preflight only.
- Catalog lookup found both new specifications. The design-handoff status showed only this
  four-file documentation package, with no permanent code or test edits.

Implementation validation passed on 2026-10-24:

- Focused RED failed on missing labels and schema metadata. GREEN passed all four
  test functions and ten projection subcases, including HTTP discovery and answers.
- The complete MCP server package passed with `-race` (52.392s).
- Changed-file lint passed with zero issues using the existing CI-pinned 2.9.0 binary.
- Public documentation checks passed (one validator test, 48 published pages).
  Specification checks and documentation coverage passed. The work order records
  every exact command and result.

The requirement is active, the design is current, and work order 01 is done.
The implementation is published in PR #4752. Browser submission passed with
the mock agent. Live provider interpretation remains unverified. PR fixup
validation and CI findings are recorded in work order 01.

## Risks

- The output schema rejects undeclared fields. Update schema and behavior together.
- Labels can repeat or contain arbitrary text. Preserve exact text and stable IDs.
- The workspace trails main. Recheck current source before implementation and
  reconcile relevant changes without repairing unrelated shared Git refs.
- The mock agent proves browser submission and result projection. Live provider
  interpretation remains unverified.
