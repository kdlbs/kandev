---
spec: docs/specs/ui/acp-shell-command-output.md
created: 2026-07-14
updated: 2026-07-16
status: done
---

# Implementation Plan: ACP Shell Command Output

## Overview

Tasks 01-04 shipped provider normalization, durable bounded output, exit semantics, and the first expandable transcript UI. This follow-up keeps that persistence contract but removes output bodies from normal browser message projections, adds a session-scoped snapshot endpoint, and changes the command row to an always-visible wrapped command plus an output-only disclosure that fetches and polls on demand. ADR-0042 records the transport boundary.

## Completed Foundation

- Tasks 01-02 normalize provider output into `metadata.normalized.shell_exec.output`, persist live updates, bound each text field to 256 KiB, and preserve nullable exit status.
- Tasks 03-04 render and verify output and exit semantics on desktop and mobile.
- The new work does not change ACP parsing, the database schema, the 256 KiB bound, or standalone terminal behavior.

---

## Backend

### Shared Client Message Projection

Files:

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/models/message_shell_output.go` (new)
- `apps/backend/internal/task/models/message_shell_output_test.go` (new)
- `apps/backend/internal/task/service/service_events.go`
- `apps/backend/internal/task/service/service_events_test.go`
- `apps/backend/internal/backendapp/helpers_test.go`

Changes:

- Add one structured metadata helper that recognizes both live typed `*streams.NormalizedPayload` values and generic maps decoded from persisted JSON.
- `ProjectMessageMetadata` uses copy-on-write along the normalized shell path, removes shell `stdout`/`stderr`, and adds `has_output`, `stdout_bytes`, and `stderr_bytes` while retaining nullable `exit_code` and `truncated`.
- Route `Message.ToAPI()` and `publishMessageEvent` through that helper. This covers REST lists, WebSocket `message.list`, task-route boot hydration, and `session.message.added` / `session.message.updated` without mutating repository models.
- Keep non-shell metadata byte-for-byte equivalent after JSON serialization and leave internal repository/service consumers on the full persisted form.

### On-Demand Output Snapshot

Files:

- `apps/backend/internal/task/models/message_shell_output.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/task/handlers/message_handlers_test.go`
- `apps/backend/internal/task/dto/dto.go`

Changes:

- Add `GET /api/v1/task-sessions/:session_id/messages/:message_id/shell-output` beside the existing message routes.
- Load through `Service.GetMessage`, require the row to belong to the path session, and extract a normalized shell snapshot with `message_id`, tool `status`, `updated_at`, and the persisted output fields.
- Return `404` for absent, cross-session, or non-shell messages; return `200` with an empty output object for a valid shell command that has not emitted output.
- Return the existing bounded snapshot as a whole value. Do not add range, delta, or streaming behavior.

---

## Frontend

### Output API and Polling Hook

Files:

- `apps/web/lib/api/domains/session-api.ts`
- `apps/web/hooks/domains/session/use-shell-command-output.ts` (new)
- `apps/web/hooks/domains/session/use-shell-command-output.test.ts` (new)
- `apps/web/components/task/chat/types.ts`

Changes:

- Split the browser type into `ShellExecOutputSummary` for message metadata and `ShellCommandOutputSnapshot` for the endpoint response.
- Add `fetchShellCommandOutput(sessionId, messageId, { signal })` using the shared API client.
- The hook fetches immediately only when open. A running snapshot schedules the next request after the previous one settles, using a one-second base interval and failure backoff capped at five seconds.
- Stop and abort on collapse or unmount. A terminal endpoint response stops recurring polling; a terminal status transition from the message projection aborts the active poll, fetches one final snapshot, and then stops. Ignore stale responses and retain the latest successful snapshot across transient errors.

### Command Row and Output Disclosure

Files:

- `apps/web/components/task/chat/messages/tool-execute-message.tsx`
- `apps/web/components/task/chat/messages/shell-output-disclosure.tsx` (new, if extraction is needed to keep component/function limits)
- `apps/web/components/task/chat/messages/tool-execute-message.test.tsx`

Changes:

- Render the full normalized command, falling back to message content, with `whitespace-pre-wrap` and word breaking rather than truncation. Keep the working directory and status visible with it.
- Remove command duplication from expanded content and remove running auto-expansion.
- Add a separate, keyboard-accessible output disclosure collapsed by default for running and terminal commands. Its compact label uses summary byte counts/truncation without requiring the body.
- Mount fetched stdout/stderr only while open. Render loading, empty, unavailable-with-retry, truncation, and exit states without shifting or overflowing desktop/mobile chat.

No Zustand slice is added; the snapshot is ephemeral disclosure state and normal session messages retain only summaries.

---

## Tests

- **What:** typed live metadata and persisted map metadata project to identical summaries, omit both bodies, preserve nullable exit/truncation, compute retained UTF-8 byte counts, and do not mutate input.
  **File:** `apps/backend/internal/task/models/message_shell_output_test.go`.
  **How:** table-driven unit tests for stdout-only, explicit stderr, empty running output, unknown exit, non-shell metadata, and multibyte text.
- **What:** normal WebSocket notifications use the summary projection and task-route boot messages contain no transcript body.
  **File:** `apps/backend/internal/task/service/service_events_test.go` and `apps/backend/internal/backendapp/helpers_test.go`.
  **How:** publish/boot focused tests with a unique large-output sentinel and assert summary fields are present while the sentinel is absent from serialized payloads.
- **What:** the snapshot route returns full valid output and enforces message/session/type scoping.
  **File:** `apps/backend/internal/task/handlers/message_handlers_test.go`.
  **How:** handler tests for `200` populated, `200` empty-running, missing `404`, cross-session `404`, and non-shell `404`.
- **What:** opening triggers one fetch, terminal output does not poll, running output polls without overlap, failure backoff is capped, and collapse/unmount aborts stale work.
  **File:** `apps/web/hooks/domains/session/use-shell-command-output.test.ts`.
  **How:** Vitest fake timers and deferred API promises.
- **What:** full commands wrap, both running and completed output start collapsed, no body is read from summary metadata, and disclosure states render exact status semantics.
  **File:** `apps/web/components/task/chat/messages/tool-execute-message.test.tsx`.
  **How:** React Testing Library with endpoint hook/API mocks and long-command fixtures.

## E2E Tests

- **Scenario:** GIVEN completed shell messages with large persisted transcripts, WHEN desktop chat opens, THEN full commands are visible, output is collapsed, and no shell-output request occurs until a disclosure is expanded; expansion performs the request and shows the transcript/result.
  **File:** `apps/web/e2e/tests/chat/tool-execute-output.spec.ts`.
- **Scenario:** GIVEN an expanded running shell command, WHEN sequential snapshots add output and become terminal, THEN the transcript refreshes and request count stops after terminal.
  **File:** `apps/web/e2e/tests/chat/tool-execute-output.spec.ts` using a route-controlled response sequence.
- **Scenario:** GIVEN a long command and large output on a narrow viewport, WHEN mobile chat is collapsed and then expanded, THEN command wrapping, disclosure controls, output, and exit status stay within the page width without overlap.
  **File:** `apps/web/e2e/tests/chat/mobile-tool-execute-output.spec.ts`.

---

## Implementation Waves

Completed waves:

- [x] [task-01-backend-normalization](task-01-backend-normalization.md) (`done`)
- [x] [task-02-acp-live-updates](task-02-acp-live-updates.md) (`done`)
- [x] [task-03-frontend-command-row](task-03-frontend-command-row.md) (`done`)
- [x] [task-04-e2e-and-verification](task-04-e2e-and-verification.md) (`done`)

Follow-up waves:

Wave 4:

- [x] [task-05-backend-output-projection](task-05-backend-output-projection.md) (`done`)

Wave 5:

- [x] [task-06-frontend-output-disclosure](task-06-frontend-output-disclosure.md) (`done`)

Wave 6:

- [ ] [task-07-lazy-output-e2e](task-07-lazy-output-e2e.md) (`in_progress`)

Dependency graph:

```text
01-backend-normalization --> 02-acp-live-updates ----> 04-e2e-and-verification
                       `--> 03-frontend-command-row --'

04-e2e-and-verification --> 05-backend-output-projection
                         --> 06-frontend-output-disclosure
                         --> 07-lazy-output-e2e
```

Tasks 05-07 are sequential because Task 06 consumes Task 05's exact summary/snapshot contract and Task 07 verifies the integrated transport and UI.

## Verification

```bash
make -C apps/backend fmt
cd apps/backend && go test ./internal/task/models ./internal/task/handlers ./internal/task/service ./internal/backendapp
make -C apps/backend lint

cd apps && pnpm --filter @kandev/web test -- hooks/domains/session/use-shell-command-output.test.ts components/task/chat/messages/tool-execute-message.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint

cd apps/web && pnpm e2e:run --project chromium tests/chat/tool-execute-output.spec.ts
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-tool-execute-output.spec.ts

make fmt
make typecheck
make test
make lint
```

## Risks

- Message metadata exists in typed form before persistence and generic-map form after JSON decoding. The projection and extraction helper must cover both or live notifications and reloads will diverge.
- `Message.ToAPI()` and `publishMessageEvent` are separate delivery paths today. Tests must prevent a future path from leaking full bodies even though boot and list routes already reuse `ToAPI()`.
- Polling whole snapshots is intentionally simple, but a user can expand several running commands. One in-flight request per open disclosure and cleanup on collapse/unmount are required to keep concurrency bounded.
- Full output still lives inside the message metadata column. This iteration reduces network/browser cost, not database storage or metadata-row decode cost.
- Output snapshots may race a normal terminal message update. Either signal must stop polling, and stale responses must not replace a newer snapshot.

## Open Questions

None. The endpoint shape, summary fields, polling bounds, storage choice, and failure behavior are fixed by the spec and ADR.
