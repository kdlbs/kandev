---
id: "04-annotation-contract"
title: "Carry region annotations in clarification answers"
status: done
wave: 3
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-007
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-008
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGE-REVIEW-007.2
  - AC-TASKS-QUESTION-IMAGE-REVIEW-007.3
  - AC-TASKS-QUESTION-IMAGE-REVIEW-007.4
  - AC-TASKS-QUESTION-IMAGE-REVIEW-007.5
  - AC-TASKS-QUESTION-IMAGE-REVIEW-007.6
  - AC-TASKS-QUESTION-IMAGE-REVIEW-008.2
  - AC-TASKS-QUESTION-IMAGE-REVIEW-008.3
system_design:
  - ../../specs/tasks/system-design/question-image-review.md
---

# Task 04: Carry region annotations in clarification answers

## Summary

Add an optional, validated `annotations` list to each answer. Persist it with the
response and deliver it to the agent live and in detached prompts. Backend only.
The user scheduled this after work order 03.

## In scope

- `protocol.AnswerAnnotation` and `Answer.Annotations`.
- `validateAnswerAnnotations` in `outcome_validation.go`, and trimming in `normalizeAnswer`.
- Serialization in `response_serialization.go` with `omitempty`.
- `simplifyAnswer` and the MCP output schema in `apps/backend/internal/mcp/server`.
- `withAnswerEvidence` in `resolver.go`, with one JSON-encoded line per region.
- Web type `ClarificationAnswerAnnotation` in `apps/web/lib/types/http-agents.ts`.
- Agent-facing docs in `docs/public/agent-communication.md`.

## Out of scope

The drawing interface, history rendering, and native or parent question changes.

## Acceptance

1. Valid regions are accepted, persisted in `metadata.response.annotations`, and
   returned in the live keyed result. Invalid regions (wrong image, out of bounds,
   oversized, empty or long comment, more than eight) fail before the claim.
2. Detached prompt text lists each region as one JSON line. A comment containing
   quotes, newlines, or `Region 2:` cannot create another line.
3. Answers without regions serialize to the same bytes and produce the same MCP
   result and prompt text as before.

## Verification

Write failing tests first, including a golden test for byte-identical
serialization without annotations and an injection test for comments.

```bash
(cd apps/backend && go test ./internal/clarification/... -count=1)
(cd apps/backend && go test ./internal/mcp/server -run 'Answer|Question' -count=1)
(cd apps/backend && go test ./internal/orchestrator -run 'Clarification' -count=1 -timeout=5m)
bash scripts/lint-go-changed apps/backend/internal/clarification/protocol/types.go apps/backend/internal/clarification/outcome_validation.go apps/backend/internal/clarification/response_serialization.go apps/backend/internal/clarification/resolver.go apps/backend/internal/mcp/server/handlers.go
(cd apps/web && pnpm run typecheck)
```

## Likely files

- `apps/backend/internal/clarification/protocol/types.go`
- `apps/backend/internal/clarification/outcome_validation.go` and tests
- `apps/backend/internal/clarification/response_serialization.go` and tests
- `apps/backend/internal/clarification/resolver.go` and tests
- `apps/backend/internal/mcp/server/handlers.go` and the output schema tests
- `apps/web/lib/types/http-agents.ts`
- `docs/public/agent-communication.md`

## Dependencies and risks

No code dependencies. Risks: the MCP output schema must allow the new key or
strict clients reject results; `normalizeAnswer` must not drop annotations when
attachments are absent.

## Results

Implemented on 2026-11-07. `protocol.AnswerAnnotation` and `Answer.Annotations`;
`internal/clarification/answer_annotations.go` holds validation, normalization,
metadata decoding, and the one-JSON-line-per-region formatter used by
`withAnswerEvidence` (renamed from `withAnswerFiles`). Persistence needed no new
code beyond serialization: the claim stores the `Answer` JSON in
`metadata.response`. The ask handler already returns the full
`clarification.Response`, so the live MCP result only needed `simplifyAnswer` and
the output schema.

Verification:

- `go test ./internal/clarification/...`: pass, including byte-identical legacy
  serialization and a comment that tries to forge `Region 2:`.
- `go test ./internal/mcp/server`: pass.
- `go test ./internal/mcp/handlers` and `go test ./internal/orchestrator -run Clarification`: pass.
- `scripts/lint-go-changed` on the changed files: 0 issues.
- `pnpm run typecheck`: pass.
