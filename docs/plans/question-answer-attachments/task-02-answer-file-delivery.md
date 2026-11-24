---
id: "02-answer-file-delivery"
title: "Deliver files through question response transports"
status: in_progress
wave: 2
depends_on:
  - "01-answer-file-admission"
plan: "plan.md"
requirements:
  - REQ-TASKS-ANSWER-ATTACHMENTS-001
  - REQ-TASKS-ANSWER-ATTACHMENTS-002
acceptance_criteria:
  - AC-TASKS-ANSWER-ATTACHMENTS-001.1
  - AC-TASKS-ANSWER-ATTACHMENTS-001.2
  - AC-TASKS-ANSWER-ATTACHMENTS-001.3
  - AC-TASKS-ANSWER-ATTACHMENTS-001.6
  - AC-TASKS-ANSWER-ATTACHMENTS-002.2
  - AC-TASKS-ANSWER-ATTACHMENTS-002.3
  - AC-TASKS-ANSWER-ATTACHMENTS-002.4
  - AC-TASKS-ANSWER-ATTACHMENTS-002.6
system_design:
  - ../../specs/tasks/system-design/question-answer-attachments.md
---

# Task 02: Deliver files through question response transports

## Summary

Make the winning answer's files readable in the source execution before the agent receives the response.
Return question-associated file references through MCP and the existing native Codex bridge, with compatible detached recovery.

## In scope

- Expose a narrow existing-execution materializer around `SessionManager.materializeAttachments` without prompt dispatch or generation changes.
- Wire a trusted materialization interface into clarification delivery after the durable claim.
- Fence streaming completion by task, source session, execution, and current conversational turn.
- Construct a delivery-only path projection. Keep generated paths out of browser responses and durable metadata.
- Add optional MCP answer attachments and update the closed output schema, structured output, and equivalent JSON text.
- Preserve exact selected labels and custom-text policy through native Codex. Append separate file-reference entries to native answer arrays.
- Forward canonical descriptors through detached resume and watchdog/restart reconstruction.
- Restore the owned current-turn claim on pre-delivery failure. Preserve uncertain-delivery handling and established winner replay.
- Keep external answer submission and parent-question input schemas unchanged.

## Out of scope

New native provider protocols, inline image result blocks, new prompts during a live question, and frontend controls are excluded.

## Acceptance

1. MCP and native Codex receive the selected/custom answer plus files with matching bytes, MIME metadata, question identity, and readable execution paths.
2. Failures, unavailable runtimes, execution replacement, competing responses, and retries cannot dispatch twice or release a response with unreadable files.
3. Detached resume and recovery retain exact file associations, while browser/durable representations exclude transient paths and all file bytes.

## TDD and verification

Actual delivery evidence is `TestMaterializationUsesPostClaimDeliveryBudget`,
`TestMaterializationFailureRestoresClaim`, `TestMaterializationRejectsSupersededConversationalTurn`,
`TestMaterializeAnswerAttachmentsRejectsExecutionReplacement`,
`TestToolUserInputAppendsAnswerAttachmentPaths`, `TestResumeDetachedClarificationCarriesAnswerAttachments`,
and the mock-agent byte/hash proof in `question-answer-attachments.spec.ts`.
Use the existing materialization client seam and real private attachment storage.
Block the streaming result to prove that the waiter cannot return early.
Include different filenames with identical question labels to expose positional matching errors.

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/clarification ./internal/mcp/server ./internal/mcp/handlers -run 'AnswerAttachment|AskUserQuestion|ExtractQuestionAnswers|Clarification' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'AnswerAttachment|SessionAttachment|Clarification' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/api ./internal/agentctl/server/adapter/transport/codexappserver -run 'AnswerAttachment|UserInput|Clarification' -count=1)
```

Run `scripts/lint-go-changed` against the final owned Go file list.
Tests through the materialization seam establish transport behavior. Record any absence of live SSH or Kubernetes evidence explicitly.

## Files likely touched

- `apps/backend/internal/clarification/resolver.go`
- `apps/backend/internal/clarification/types.go`
- `apps/backend/internal/clarification/protocol/types.go`
- `apps/backend/internal/clarification/handlers.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go` and attachment tests
- `apps/backend/internal/agent/runtime/runtime.go`
- `apps/backend/internal/agent/runtime/facade.go`
- `apps/backend/internal/backendapp/adapters.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/mcp/handlers/handlers_ask.go`
- `apps/backend/internal/mcp/handlers/handlers_ask_retry.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/ask_user_question_answer_attachments_test.go` (new)
- `apps/backend/internal/clarification/resolver_attachment_delivery_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/manager_answer_attachments_test.go` (new)
- `apps/backend/internal/agentctl/server/api/codex_user_input.go` and tests
- `apps/backend/internal/agentctl/types/user_input.go`
- `apps/backend/internal/agentctl/server/adapter/transport/codexappserver/user_input_requests.go` and tests
- `apps/backend/internal/orchestrator/event_handlers_clarification.go` and tests

## Dependencies

Task 01 supplies the canonical manifest and atomic response/file claim.

## Risks

Materialization runs while the provider waits inside a tool call.
The implementation must not acquire a new prompt lifecycle or cancel that waiting call.
An execution replacement requires fresh file paths and current-turn checks.

## Parallelism

`sequential`

## Inputs

- [Requirements 001 and 002](../../specs/tasks/requirements/question-answer-attachments.md).
- [Agent delivery, Recovery and retention, and Authority and security](../../specs/tasks/system-design/question-answer-attachments.md).
- Existing live delivery confirmation, detached resume, `materializeAttachments`, and native user-input tests.
- Scoped backend, agentctl, and agentctl API `AGENTS.md` files.

## Results

Implemented the trusted existing-execution materializer, agentctl path response, per-question MCP
result attachments, native Codex file-reference entries, detached resume descriptors, watchdog/queue
descriptor forwarding, and current-turn recheck after streaming.

Verification passed: agentctl API, Codex app-server transport, clarification, MCP server/handlers,
lifecycle, orchestrator, and backendapp tests; changed-file Go lint with 0 issues. Desktop E2E proved
the mock agent read materialized bytes and reported their SHA-256 digest. Review remediation added a
post-claim delivery budget shared by transfer and response delivery, canonical current-turn authority
for the post-transfer check, and execution-identity revalidation after streaming. Focused regressions
cover a transfer outliving the pre-claim budget, transfer failure restoring the retry manifest, and
execution replacement during transfer. Detached descriptor mapping is unit-tested; a full detached
prompt resume with files was not exercised in Playwright. Remote SSH, Kubernetes, and live
native-provider runs remain unexercised.
