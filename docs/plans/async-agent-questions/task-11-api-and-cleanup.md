---
id: "11-api-and-cleanup"
title: "Close remaining API and cleanup gaps"
status: done
wave: 9
depends_on:
  - "10-transcript-and-status"
plan: "plan.md"
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-002
  - REQ-TASKS-ASYNC-QUESTIONS-007
acceptance_criteria:
  - AC-TASKS-ASYNC-QUESTIONS-002.2
  - AC-TASKS-ASYNC-QUESTIONS-002.3
  - AC-TASKS-ASYNC-QUESTIONS-007.4
  - AC-TASKS-ASYNC-QUESTIONS-007.5
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
---

# Task 11: Close remaining API and cleanup gaps

## Summary

Finish the smaller review items: make retries report the right result, stop revealing foreign records, and remove leftovers.
Bring the public docs and plan records in line with what shipped.

## In scope

- Storing `submission_id` on the winning outcome, so an exact retry reports `won=true` and creates no delivery.
- Non-disclosing 404 responses for inaccessible records on `respond`.
- Clear operator feedback when an answer hits a stale request, closing the row instead of showing a generic failure.
- Removing async rows in workspace cascade deletion.
- Removing the stub `POST /:id/withdraw` route.
- Writing or removing the wait's unused `workspace_id` column value.
- Public docs for the chat surface, the task-scoped route, and the event payload.
- Correcting stale plan and work-order records.

## Out of scope

- New features, or changes to the question lifecycle.

## Acceptance

1. An answer retried with its original submission ID reports `won=true` and creates one delivery, and a different submission reports `won=false`.
2. A caller without workspace access gets the same 404 for a real question and a missing one.
3. Deleting a workspace leaves no async question, forward, delivery, wait, or dependency rows.

## Verification

Run from the repository root. In a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -trimpath ./internal/task/questions -count=1)
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite -run 'TestAsyncQuestions|WorkspaceCascade' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/web && pnpm exec vitest run components/needs-you-inbox)
bash scripts/lint-go-changed <changed go files>
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

Required new tests: `TestAsyncQuestionsRespondRetryKeepsWin`, `TestAsyncQuestionsRespondHidesForeignRecord`, and `TestAsyncQuestionsWorkspaceCascadeRemovesRows`.

## Files likely touched

- `apps/backend/internal/task/questions/http_handlers.go`, `http_handlers_test.go`, `service.go`
- `apps/backend/internal/task/models/async_question.go`
- `apps/backend/internal/task/repository/sqlite/workspace.go`, `async_question_waits.go`, `async_questions_test.go`
- `apps/web/components/needs-you-inbox/async-question-row.tsx`
- `docs/public/agent-communication.md`, `automation-and-mcp.md`, `websocket-api.md`, `coverage.json`
- `docs/plans/async-agent-questions/plan.md`

## Dependencies

Task 10. It runs late because it touches files that Tasks 09 and 10 also change.

## Risks

- `workspace.go` cascade deletion has several variants. Confirm they share `deleteWorkspaceCascade`.
- A 403-to-404 change must still respect the existing scope contract for callers who can read but not write. Decide this from the existing clarification handlers, and test both cases.

## Parallelism

`sequential`

## Inputs

- [HTTP and event contracts](../../specs/tasks/system-design/async-agent-questions.md#http-and-event-contracts).
- [Withdrawal and terminal cleanup](../../specs/tasks/system-design/async-agent-questions.md#withdrawal-and-terminal-cleanup).
- Backend guidance on workspace deletion side tables.

## Results

Implemented forward-scoped operator HTTP reads for parent requests, author-scoped transcript detail access for the transcript card, payload-aware answer submission IDs, owner/MCP response identity redaction, and delivery admission despite an authority-sweep query error.

Passed:

- Backend question, MCP server, and backendapp suites passed.
- `TestAsyncQuestionsHTTPOperatorScopeRequiresForwardForParentRequests`, `TestAsyncQuestionMCPViewOmitsSubmissionIdentity`, `TestAsyncQuestionOwnerDeliveryOmitsSubmissionIdentity`, and `TestAsyncQuestionDeliveryAdmissionContinuesAfterAuthoritySweepError` passed.
- Focused Vitest regressions: 4 files, 25 tests passed, including payload-change submission retries.
- Changed-file Go lint: `0 issues`.
