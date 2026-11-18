---
id: "01-bundle-gallery-capacity"
title: "Raise the question bundle image limit to 16"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-006
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGE-REVIEW-006.1
  - AC-TASKS-QUESTION-IMAGE-REVIEW-006.2
  - AC-TASKS-QUESTION-IMAGE-REVIEW-006.3
  - AC-TASKS-QUESTION-IMAGE-REVIEW-006.4
system_design:
  - ../../specs/tasks/system-design/question-image-review.md
---

# Task 01: Raise the question bundle image limit to 16

## Summary

Let four questions each carry their own baseline and candidates. Keep the
per-question and per-choice limits. This is backend and documentation work only.

## In scope

- Add `MaxBundleImages = 16` and `MaxQuestionImageReferences = 4` in
  `apps/backend/internal/clarification/protocol/images.go`. Use them in
  `ValidateQuestionImages` and in its error messages.
- Build the `buildQuestionImageSchema` description in
  `apps/backend/internal/mcp/server/question_images.go` from the same constants.
- Update AC-TASKS-QUESTION-IMAGES-001.1 and AC-005.1 in
  `docs/specs/tasks/requirements/question-image-previews.md` to point at
  AC-TASKS-QUESTION-IMAGE-REVIEW-006.1, and the matching text in the previews
  system design. Remove "larger question bundles" from that document's out-of-scope list.
- Update the limits in `docs/public/agent-communication.md`. Use `/docs-maintainer`.

## Out of scope

Web changes, the review view, and any change to per-question or per-choice limits.

## Acceptance

1. A bundle with 16 distinct image IDs across four questions is accepted through
   MCP, REST, and WS admission. A bundle with 17 is rejected before a wait, a
   durable message, or a retry identity exists.
2. Per-question (4) and per-choice (1) limits still reject excess references.
3. The tool schema description and public docs state 16, and a test proves the
   schema text uses the constants.

## Verification

Write the failing 16-image acceptance test first and confirm it fails on the
current limit of four.

```bash
(cd apps/backend && go test ./internal/clarification/... -run 'Image' -count=1)
(cd apps/backend && go test ./internal/mcp/server -run 'Image|Question' -count=1)
(cd apps/backend && go test ./internal/mcp/handlers -run 'TestQuestionImagesAdmission|TestChoiceImages' -count=1 -timeout=3m)
bash scripts/lint-go-changed apps/backend/internal/clarification/protocol/images.go apps/backend/internal/mcp/server/question_images.go
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
```

## Likely files

- `apps/backend/internal/clarification/protocol/images.go`
- `apps/backend/internal/clarification/question_images_test.go`
- `apps/backend/internal/mcp/server/question_images.go` and its tests
- `docs/specs/tasks/requirements/question-image-previews.md`
- `docs/specs/tasks/system-design/question-image-previews.md`
- `docs/public/agent-communication.md`

## Dependencies and risks

No dependencies. Risk: some tests may assert the exact old error text; update
them to the new constant-based text.

## Results

Implemented on 2026-11-07. `protocol.MaxBundleImages` (16),
`MaxQuestionImageReferences` (4), and `MaxChoiceImages` (1) drive validation,
error text, and the operator tool schema description. Tests:
`TestQuestionImagesBundleAcceptsSixteenDistinctImages`,
`TestQuestionImagesBundleRejectsSeventeenDistinctImages` (protocol-level, since
the four-question limit makes 17 placements impossible through admission), and
`TestQuestionImagesSchemaStatesBundleLimit`. The previews requirement, its design,
and `docs/public/agent-communication.md` now state 16.

- `go test ./internal/clarification/...`: pass.
- `go test ./internal/mcp/server`: pass.
- `go test ./internal/mcp/handlers -run 'TestQuestionImagesAdmission|TestChoiceImages'`: pass.
- `scripts/lint-go-changed` on the changed files: 0 issues.
