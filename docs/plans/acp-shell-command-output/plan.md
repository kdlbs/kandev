---
spec: docs/specs/ui/acp-shell-command-output.md
created: 2026-07-14
status: complete
---

# Implementation Plan: ACP Shell Command Output

## Overview

Extend the existing normalized shell payload rather than adding a transport or storage layer. First define bounded, nullable provider normalization, then wire ACP capability negotiation and live tool updates into it. Finally update the existing command row and verify its persisted desktop/mobile rendering.

---

## Backend

### Normalized Shell Output Contract

Files:

- `apps/backend/internal/agentctl/types/streams/tool_payload.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/normalize.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/shell_output.go` (new, if extraction keeps `normalize.go` within limits)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/normalize_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/shell_output_test.go` (new, if paired with the helper)

Changes:

- Change `ShellExecOutput.ExitCode` to a nullable pointer and add `Truncated`.
- Replace the current plain-string-implies-zero behavior with provider-aware extraction for Codex `formatted_output`/`exit_code`, OpenCode `metadata.exit`, Auggie XML, and plain fallback output.
- Add one bounded text helper with a 256 KiB per-field limit that retains the newest valid UTF-8 content.
- Encode output update mode explicitly: Codex delta appends; Claude terminal output, OpenCode cumulative content, and final aggregates replace. Final missing output preserves accumulated text.
- Apply exit precedence from the spec and keep unknown nullable.

### ACP Capability and Live Updates

Files:

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/tool_call_update_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/conversion_test.go` or a focused initialize test file

Changes:

- Set `ClientCapabilities.Meta["terminal_output"] = true` on the initialize request without advertising ACP terminal RPC support.
- Feed recognized `_meta.terminal_output_delta`, `_meta.terminal_output`, `_meta.terminal_exit`, text content, and raw output into the shell normalizer while the adapter lock owns the active payload.
- Synthesize `in_progress` only for recognized statusless terminal-output updates so the orchestrator persists them.
- Normalize before deleting terminal tool state; keep the existing prompt-end cleanup behavior.

No orchestrator, database, HTTP, or WebSocket changes are planned. `tool_update` already replaces and publishes the persisted normalized message metadata.

---

## Frontend

### Expandable Command Row

Files:

- `apps/web/components/task/chat/types.ts`
- `apps/web/components/task/chat/messages/tool-execute-message.tsx`
- `apps/web/components/task/chat/messages/tool-execute-message.test.tsx` (new)

Changes:

- Model exit status as `number | null | undefined` and add `truncated` to the shared shell output type.
- Remove the current absent-exit-means-success rule.
- Render live/final combined output in the current expanded content, plus a completion footer with `Exit code N` or `Exit code unavailable` and a truncation label when needed.
- Keep known zero success, known nonzero/error failure, and unknown neutral. Preserve stable sizing, wrapping, horizontal scrolling, and the existing expand-state behavior.

No new state, hook, or API client is needed.

---

## Tests

- **What:** provider-specific output and exit extraction, precedence, unknown exit, append/replace behavior, deduplication, bounds, and UTF-8 safety.
  **File:** `apps/backend/internal/agentctl/server/adapter/transport/acp/normalize_test.go` and/or `shell_output_test.go`.
  **How:** table-driven tests built from the captured Codex, Claude, OpenCode, and Auggie frame shapes.
- **What:** statusless terminal metadata becomes persisted `in_progress`, final updates retain output and exit, and terminal state is cleaned up.
  **File:** `apps/backend/internal/agentctl/server/adapter/transport/acp/tool_call_update_test.go`.
  **How:** drive initial tool call plus incremental/final `SessionToolCallUpdate` values through the adapter conversion methods.
- **What:** ACP initialization advertises only the terminal-output meta extension required by Claude.
  **File:** focused adapter initialize test or `conversion_test.go`.
  **How:** capture the fake agent's `InitializeRequest` and assert `_meta.terminal_output == true`.
- **What:** exit `0`, nonzero, and unknown render distinct status; live output and truncation are visible.
  **File:** `apps/web/components/task/chat/messages/tool-execute-message.test.tsx`.
  **How:** React Testing Library cases over persisted `Message` fixtures.

---

## E2E Tests

- **Scenario:** GIVEN persisted successful, failed, and unknown-exit command messages, WHEN desktop chat expands each row, THEN combined output and the exact exit label are visible and unknown is not presented as success.
  **File:** `apps/web/e2e/tests/chat/tool-execute-output.spec.ts`.
- **Scenario:** GIVEN a persisted command with long wrapped output on a narrow viewport, WHEN mobile chat expands the row, THEN output, truncation state, and exit label remain readable without overlap or horizontal page overflow.
  **File:** `apps/web/e2e/tests/chat/mobile-tool-execute-output.spec.ts`.

The provider wire shapes are covered at the backend adapter level; E2E seeds the normalized persisted contract so it remains deterministic and does not require four external authenticated CLIs.

---

## Implementation Waves

Wave 1:

- [x] [task-01-backend-normalization](task-01-backend-normalization.md) (`done`)

Wave 2 (parallel after Task 01):

- [x] [task-02-acp-live-updates](task-02-acp-live-updates.md) (`done`)
- [x] [task-03-frontend-command-row](task-03-frontend-command-row.md) (`done`)

Wave 3:

- [x] [task-04-e2e-and-verification](task-04-e2e-and-verification.md) (`done`)

Dependency graph:

```text
01-backend-normalization
  |-- 02-acp-live-updates --|
  `-- 03-frontend-command-row --+--> 04-e2e-and-verification
```

## Verification

```bash
make -C apps/backend fmt
(cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp)
(cd apps && pnpm --filter @kandev/web test -- components/task/chat/messages/tool-execute-message.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/tool-execute-output.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-tool-execute-output.spec.ts)
make -C apps/backend test
make -C apps/backend lint
```

## Risks

- Provider extensions are not part of the stable ACP schema. Parsing remains defensive and scoped to shell payloads so unrelated `_meta` cannot mutate other tool kinds.
- Codex may omit output generated immediately after process start upstream. Kandev can only preserve bytes present in ACP frames; provider-side loss is explicitly out of scope.
- OpenCode reports nonzero commands with ACP status `completed`; UI success must therefore use a known exit code before the generic completed status.

## Open Questions

None. The 256 KiB per-field bound and tail-retention behavior are explicit product constraints for this implementation.
