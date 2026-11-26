---
id: "02-image-group-contract"
title: "Image group contract for operator questions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-002
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGE-ITEMS-002.1
  - AC-TASKS-QUESTION-IMAGE-ITEMS-002.2
  - AC-TASKS-QUESTION-IMAGE-ITEMS-002.4
  - AC-TASKS-QUESTION-IMAGE-ITEMS-002.5
  - AC-TASKS-QUESTION-IMAGE-ITEMS-002.6
  - AC-TASKS-QUESTION-IMAGE-ITEMS-002.7
system_design:
  - ../../specs/tasks/system-design/question-image-items.md
---

# Task 02: Image group contract

## Summary

Issue #5495, contract half. Agents can declare `image_groups` (pairs and singles)
on an operator question. The server validates, persists, and includes them in the
request identity. The tool schema and public docs explain them. Design: "Image
group contract".

## Scope

- `protocol.QuestionImageGroup`, `QuestionImageGroupKind`, and
  `Question.ImageGroups`.
- `validateImageGroups`, called from `ValidateQuestionImages`, with errors that
  name the question and the failed rule.
- `clarificationQuestionData` writes `image_groups`; the bundle question decoder
  reads it; `questionsEqual` compares it.
- `askParentQuestionHandler` rejects `image_groups`.
- `buildQuestionImageGroupSchema` in `mcp/server/question_images.go`, and the
  updated image description.
- Web type `QuestionImageGroup` and `ClarificationQuestion.image_groups`.
- `docs/public/agent-communication.md`: a short section with a pair-plus-single
  example and the rules. Run `/docs-maintainer`.

## Exclusions

All review-view behavior (work order 03). No answer change.

## Acceptance

1. Valid pairs and singles are accepted. Each invalid case in AC-ITEMS-002.2 fails
   before a wait or retry identity, with a message naming the question and rule.
2. Groups round-trip through message metadata and the bundle decoder, and a
   changed group list is a different request identity.
3. Questions without groups produce byte-identical metadata and unchanged schema
   behavior; parent questions reject groups.

## Verification

```bash
make -C apps/backend fmt
(cd apps/backend && go test -trimpath ./internal/clarification/... ./internal/mcp/... ./internal/backendapp/ -run 'Image|Question|Parent|Schema')
(cd apps/backend && go vet ./internal/clarification/... ./internal/mcp/...)
make -C apps/backend lint
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
```

## Likely files

- `apps/backend/internal/clarification/protocol/images.go`, `types.go`
- `apps/backend/internal/clarification/protocol/` tests, or a new
  `apps/backend/internal/clarification/question_image_groups_test.go` beside
  `question_image_labels_test.go`
- `apps/backend/internal/clarification/bundle_question.go`, `store.go` and tests
- `apps/backend/internal/backendapp/adapters.go`, `adapters_question_images_test.go`
- `apps/backend/internal/mcp/server/question_images.go`, `handlers.go`,
  `ask_user_question_images_test.go`
- `apps/web/lib/types/http-agents.ts`
- `docs/public/agent-communication.md`

## Risks

- The image read route also runs `ValidateQuestionImages`; persisted groups must
  decode exactly as created, or reads fail. The round-trip test covers it.
- Schema `maxItems` alone cannot enforce pair versus single counts; server
  validation is the authority and its messages must be clear to agents.

## Results

Implemented on 2026-11-24 in `protocol/image_groups.go` (`QuestionImageGroups`,
`validateImageGroups`), `types.go`, `images.go`, `store.go`, `bundle_question.go`,
`backendapp/adapters.go`, both parent-question paths, and
`mcp/server/question_images.go` (`buildQuestionImageGroupSchema`). Errors read
`question N ("id"): image_groups: <rule>`. Tests: `question_image_groups_test.go`,
`bundle_question_image_groups_test.go`, `adapters_question_image_groups_test.go`,
`ask_user_question_image_groups_test.go`, `parent_question_image_groups_test.go`.
`go test` for `./internal/clarification/... ./internal/mcp/... ./internal/backendapp/`
and `scripts/lint-go-changed` pass. Public docs updated in
`docs/public/agent-communication.md`.

Review follow-up (2026-11-25): image reads validate only the flat image references
(`ValidateQuestionImages`); creation paths use `ValidateQuestionBundleImages`, so
a malformed saved group never hides valid images. MCP decode errors name their
question, and errors omit an empty question ID.
