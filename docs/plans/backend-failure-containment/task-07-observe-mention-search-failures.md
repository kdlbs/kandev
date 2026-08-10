---
id: "07-observe-mention-search-failures"
title: "Make mention-search failures observable"
status: done
wave: 2
depends_on: ["02-publish-agent-runtime-availability"]
plan: "plan.md"
spec: "../../specs/ui/entity-reference-composer.md"
---

# Task 07: Make mention-search failures observable

## Intent

Keep partial provider search resilient, treat superseded client requests as
normal cancellation, and ensure every unexpected mention-search 500 has one
safe structured backend error record.

## Acceptance

- Invalid requests remain 400 and missing workspaces remain 404.
- `context.Canceled` from a client request returns 499 and stops provider work.
- Workspace repository failures and unexpected aggregate search failures return
  500 only after exactly one structured Error record.
- Error records include workspace ID and a stable failure stage/class where
  available.
- Provider failures remain safe per-provider groups in an HTTP 200 response and
  can emit source/provider/kind/status diagnostics.
- Logs do not contain the query, prompts, credentials, tokens, raw provider
  payloads, or raw provider error bodies.
- Request cancellation is not recorded as an unexpected 500.

## TDD sequence

1. Update/add handler and backend composition tests for invalid input, missing
   workspace, generic workspace lookup failure, cancellation, provider failure,
   and an unexpected searcher error.
2. Attach a `zaptest/observer` and assert status, level, message, safe fields,
   exactly one record for each 500, and absence of query/secret fixtures.
3. Confirm RED for cancellation (currently 500) and unlogged aggregate failures.
4. Inject the logger, classify `context.Canceled` as 499, and log unexpected
   aggregate failures before writing 500.
5. Add safe provider status diagnostics without changing the service's partial
   success response contract.
6. Run both mention and backend composition packages under `-race`, then refactor
   only after GREEN.

## Files likely touched

- `apps/backend/internal/mentions/handler.go`
- `apps/backend/internal/mentions/handler_test.go`
- `apps/backend/internal/mentions/service.go`
- `apps/backend/internal/mentions/service_test.go`
- `apps/backend/internal/backendapp/mentions.go`
- `apps/backend/internal/backendapp/mentions_test.go`
- `apps/backend/internal/backendapp/helpers.go`

## Dependencies

Task 02 only for implementation sequencing: both tasks can edit backend route
composition helpers. There is no behavioral dependency on runtime availability.

## Parallelism

`sequential-after-task-02` to avoid overlapping edits in
`internal/backendapp/helpers.go`. It is otherwise independent from Tasks 03,
05, and 06.

## Verification

- `cd apps/backend && go test -race ./internal/mentions ./internal/backendapp`
- `cd apps/backend && golangci-lint run ./internal/mentions ./internal/backendapp --timeout=5m`

## Inputs

- Unexplained historical 500 for
  `GET /api/v1/workspaces/:id/mentions/search`.
- Current `writeSearchError` generic fallback and workspace-validation wrapper.
- Existing frontend debounce/`AbortController` behavior and backend cancellation
  composition test that currently expects 500.
- Existing middleware treatment of HTTP 499 as routine.

## Output contract

Record the error-path matrix, observed RED statuses/log absence, final safe log
schema, sensitive-field assertions, and focused race/lint results. State that
the historical request's exact cause remains unknown unless new evidence proves
it.

## Results

Implemented stable search failure classification and safe structured logging.
Invalid input remains 400, missing workspaces remain 404, client cancellation
maps to 499 without an unexpected-error record, and aggregate failures emit one
Error record with workspace ID plus failure stage/class before returning 500.
Provider failures retain their HTTP 200 grouped response and emit only safe
source/provider/kind/status diagnostics; query text, raw provider bodies, and
other sensitive values are excluded.

Focused validation passed:

- `gofmt` on the changed Go files.
- `go test -count=1 -run 'TestHandlerSearch|TestServiceSearchLogsProviderFailure|TestMentionHTTPComposition' ./internal/mentions ./internal/backendapp` — 12 tests passed.
- `go test -race ./internal/mentions ./internal/backendapp` — 430 tests passed.
- `golangci-lint run ./internal/mentions ./internal/backendapp --timeout=5m` — no issues.
