---
id: "01-question-image-contract"
title: "Persist question image references"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGES-001
  - REQ-TASKS-QUESTION-IMAGES-004
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGES-001.1
  - AC-TASKS-QUESTION-IMAGES-001.3
  - AC-TASKS-QUESTION-IMAGES-001.4
  - AC-TASKS-QUESTION-IMAGES-001.5
  - AC-TASKS-QUESTION-IMAGES-004.1
  - AC-TASKS-QUESTION-IMAGES-004.3
  - AC-TASKS-QUESTION-IMAGES-004.4
system_design:
  - ../../specs/tasks/system-design/question-image-previews.md
---

# Task 01: Persist question image references

## Summary

Add optional image references to operator questions. Preserve them through
admission, durable messages, reconstruction, and external pending-question listing.
This work order does not read files or render previews.

## In scope

- Implement the exact `QuestionImage` shape and limits from the design.
  Validate MCP, REST, and WS inputs before accepting a bundle. Ensure malformed
  inputs cannot bypass validation through native free-text branches.
- Add the operator-only schema. Keep the shared parent schema image-free and
  reject unsupported parent image payloads. Preserve native provider producers.
- Store and parse optional images, including external listings and historical
  message metadata. Keep empty images omitted and legacy JSON unchanged.
- Extend content equality and retry coverage. Compare image-bearing question IDs.
  Preserve image-free hash fixtures. Keep keyed answer extraction unchanged.
- For image-bearing admission, derive task identity from the source session.
  Reject mismatched supplied task IDs. Preserve existing caller authorization.

## Out of scope

File reads, UI markup, uploads, option-label response changes, native provider
image protocols, larger bundles, and unrelated clarification refactors.

## Acceptance

1. A repeated image reference survives a database-shaped JSON round trip and
   authorized listing, with independent stable question IDs.
2. Invalid references fail before a pending request or message is created.
   Parent and legacy producer schemas retain their existing contracts.
3. Exact retries reconcile once. Changed images cannot reuse a different pending
   bundle. Existing answer, rejection, timeout, and concurrent-resolution tests pass.

## TDD cases

Add `ask_user_question_images_test.go` and `question_images_test.go`. Cover omitted
and empty images, limits, repeated consistent IDs, conflicting IDs, duplicate IDs,
Unicode descriptions, bad types, unknown fields, unsafe paths, and supported suffixes.
Use the same fixtures against admission paths where possible.

Extend `TestClarificationQuestionDataPreservesCustomTextPolicy` or add
`TestQuestionImagesMetadataRoundTrip` in the backend adapter suite. Serialize and
unmarshal through JSON before reconstructing questions. Verify image-free storage
has no added field. Verify response metadata updates preserve images.

Add image-bearing cases to `store_test.go`, `retry_test.go`, and
`handlers_ask_retry_test.go`. Include changed path, repo, alt, order, and question ID.
Run existing first-writer-wins and late-delivery tests. Do not redefine claim rules.

## Verification

Run from the repository root. New test names use the `QuestionImage` prefix.

```bash
(cd apps/backend && go test ./internal/mcp/server ./internal/mcp/handlers ./internal/clarification)
(cd apps/backend && go test ./internal/backendapp -run 'Test(ClarificationQuestionData|QuestionImage)')
(cd apps/backend && go test -race ./internal/clarification ./internal/mcp/handlers -run 'QuestionImage|Retry|CrossEntry')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run `bash scripts/lint-go-changed` with the concrete changed Go files before
recording completion. Record that exact command in Results.

## Files likely touched

- `apps/backend/internal/clarification/protocol/types.go`
- `apps/backend/internal/clarification/handlers.go`, `bundle_question.go`, `store.go`, `retry_test.go`
- `apps/backend/internal/mcp/server/server.go`, `handlers.go`, `ask_user_question_images_test.go`
- `apps/backend/internal/mcp/handlers/handlers_ask.go`, `parent_question.go`, relevant tests
- `apps/backend/internal/backendapp/adapters.go`, `adapters_test.go`

New helpers should prevent these existing large files from growing further.

## Dependencies

None. Recheck issue #4667 and upstream question changes before starting.

## Risks

The schema builder and parser serve both parent and operator tools. The metadata
parser is manual. Empty optional fields can change retry hashes unless omitted.

## Parallelism

Sequential. This work changes shared contracts.

## Inputs

Read the requirements, design sections Authoring contract and Persistence and
projections, `apps/backend/AGENTS.md`, and the listed existing tests.

## Results

Implemented and verified on 2026-10-24. RED tests showed discarded image metadata
and the missing operator schema. The additive contract now reaches admission,
durable messages, history reconstruction, and pending-question listings. Parent
questions reject image metadata. Empty image lists retain legacy serialization.

Passed from `apps/backend`:

```bash
go test ./internal/clarification ./internal/sysprompt
go test ./internal/mcp/server
go test ./internal/mcp/server ./internal/backendapp ./internal/sysprompt -run 'QuestionImage|ClarificationQuestionData|Question|Barrier'
go test -race ./internal/clarification ./internal/mcp/handlers -run 'QuestionImage|Retry|CrossEntry'
go test -race ./internal/mcp/handlers -run 'TestQuestionImagesCrossEntry|TestAnswerQuestion_REST'
go test -race ./internal/clarification -run 'QuestionImages'
```

The explicit REST/MCP race command covers both legacy and image-bearing bundles.
Detached answer failure, retry, rejection, and recorded-winner replay preserve image
metadata. The broad handler-suite result belongs to Task 04; it does not replace
these focused results. Changed-package Go lint passed with zero issues.
