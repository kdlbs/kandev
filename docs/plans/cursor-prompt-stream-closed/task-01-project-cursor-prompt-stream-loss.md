---
id: "01-project-cursor-prompt-stream-loss"
title: "Project Cursor prompt-stream loss"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.22
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.24
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.30
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.8
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.10
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.1
system_design:
  - ../../specs/platform/system-design/provider-error-recovery-cursor.md
  - ../../specs/platform/system-design/provider-interruption-continuation.md
---

# Task 01: Project Cursor prompt-stream loss

## Summary

Recognize Cursor's exact `-32603` / `WritableIterable is closed` prompt reply
as `agent_transport_lost`. Emit it as a retained failure that carries the
prompt's continuation evidence, so existing recovery can either continue after
eligible tool activity or replay the original prompt at a proven pre-result
safe point.

## In scope

- `cursorPromptStreamClosed(err)` exact predicate in the Cursor dialect.
- A new prompt-error branch for Cursor before `retainedACPApplicationError`.
- A shared `sendCursorTransportFailure` helper, reused by the terminal
  `RetriableError` branch.
- The `cursor.prompt_stream_closed.v1` catalogue rule and its constant message.

## Out of scope

Generic `Data` classification, other `details` values, admission policy, UI,
E2E, and public docs.

## Acceptance

1. The exact envelope from a peer response yields one `EventTypeError` with
   these fields: `ProviderError` with source `cursor_acp` and the constant
   message, `retain_runtime`, and the turn's `ContinuationSafety` snapshot.
   Raw `data` appears in no event, log, or persisted field.
2. The constant classifies only for `cursor-acp`, as high-confidence
   `agent_transport_lost`.
3. These shapes all keep today's generic retained failure with no snapshot:
   a bare `-32603`, `data` omitted or `null`, extra keys, another or
   differently cased `details`, a local error, and a non-Cursor adapter.
4. The terminal `RetriableError` path keeps its existing output.
5. With the exact envelope and no assistant output or tool activity, the
   adapter emits safe empty evidence. The concrete-profile transient path uses
   replay mode and dispatches the cached original prompt at most once per
   admitted attempt under the existing retry budget; it does not send
   `continue`.

## Verification

Run from the repository root. Write the plan's named RED tests first.

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/server/adapter/transport/acp -run 'Test.*(CursorPromptStreamClosed|Retriable|PromptError|Continuation)' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/routingerr -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -run 'Test.*(Interruption|Continuation|CursorPromptStreamClosed)' -count=1)
bash scripts/lint-go-changed \
  apps/backend/internal/agent/runtime/routingerr/runtime_rules.go \
  apps/backend/internal/agent/runtime/routingerr/cursor_prompt_stream_closed_test.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_cursor_prompt_error.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_cursor_prompt_error_test.go \
  apps/backend/internal/orchestrator/event_handlers_transient_retained_runtime_test.go
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_cursor_prompt_error.go` (new)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_cursor_prompt_error_test.go` (new)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
- `apps/backend/internal/orchestrator/event_handlers_transient_retained_runtime_test.go`
- `apps/backend/internal/agent/runtime/routingerr/runtime_rules.go`
- `apps/backend/internal/agent/runtime/routingerr/rule_ids.go`, if rule IDs are enumerated there
- `apps/backend/internal/agent/runtime/routingerr/cursor_prompt_stream_closed_test.go` (new)

## Dependencies

None. Rebase onto current `main` before starting.

## Risks

The SDK decodes `RequestError.Data` as `map[string]any`; non-object values stay
outside the exact match. Do not loosen the match.

## Parallelism

`sequential`

## Inputs

- Plan root cause and disposition table.
- The Cursor design section "Cursor terminal prompt-stream loss".
- The [issue capture](https://github.com/kdlbs/kandev/issues/5541#issuecomment-7197605216).

## Results

Implemented the exact Cursor prompt-stream failure projection and provider-scoped
classification. Added adapter evidence coverage and a concrete-profile
orchestrator regression proving a no-output/no-tools failure uses replay mode,
dispatches the cached original prompt once for the admitted attempt, and never
sends `continue`. Removed the speculative `json.RawMessage` branch and redundant
cancel guard. Post-review verification on 2026-11-26 passed: the ACP, routing,
and orchestrator Go suites, agentctl build, changed-file Go lint (0 issues),
documentation catalog validation, and specification lint. The plan records the
exact commands and their results.
