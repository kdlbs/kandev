---
created: 2026-11-26
status: done
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
system_design:
  - ../../specs/platform/system-design/provider-error-recovery-cursor.md
  - ../../specs/platform/system-design/provider-interruption-continuation.md
legacy_specs: []
---

# Implementation Plan: Recover after Cursor prompt-stream loss

## Overview

Resolve [issue #5541](https://github.com/kdlbs/kandev/issues/5541). Cursor can
answer `session/prompt` with `-32603 Internal error` after it streams output.
This happens when its model stream closes while a `postToolUse` hook is still
running. Recognize the verified envelope in the Cursor dialect, then route it
through the existing continuation or pre-result replay policy. This package has
one work order and runs in the main session.

## Root cause

Cursor's terminal reply carries the only cause in `error.data`:

```json
{"error":{"code":-32603,"data":{"details":"WritableIterable is closed"},"message":"Internal error"},"id":5,"jsonrpc":"2.0"}
```

`Adapter.Prompt` (`adapter_prompt.go`) sends this error to
`retainedACPApplicationError`. `ProviderErrorFromError` classifies on `Message`
only, as criterion `.24` requires. `Internal error` is unclassified, so the
session retains the runtime and settles to `WAITING_FOR_INPUT` with
**Dismiss and continue** and `attempts_started=0`. That matches the incident.
The Cursor dialect has no `continuationError` hook. No `routingerr` rule maps
the condition to `agent_transport_lost`. The continuation snapshot is therefore
never attached to the failure.

## Requirement change

New criterion `AC-PLATFORM-INTERRUPTION-CONTINUATION-001.10` defines the exact
envelope, its classification, and its negative shapes. It also states that a
`stopReason` result settles normally. The criterion is a narrow exception to
`.24`, recorded in
[ADR-2026-11-26](../../decisions/2026-11-26-cursor-prompt-stream-closed-recovery.md).
Continuation admission, budget, hidden `continue`, and feedback reuse
`AC-PLATFORM-INTERRUPTION-CONTINUATION-001.1` through `001.9` and `002.1`
through `002.4` unchanged.

## Scope

### In scope

- The exact Cursor envelope predicate and a constant sanitized diagnostic.
- The provider-scoped catalogue rule `cursor.prompt_stream_closed.v1` mapping
  to `agent_transport_lost`.
- The retained-runtime failure event with the prompt's `ContinuationSafety`
  snapshot, emitted through a helper shared with the `RetriableError` branch.
- A sanitized regression fixture covering positive and negative shapes.

### Out of scope

- Generic `RequestError.Data` classification, and other `details` values.
- Reading Cursor-internal logs such as `turnEnded`.
- Changes to admission rules, the retry budget, UI copy, mock-agent E2E
  scenarios, and public docs. The existing continuation notice already
  describes this recovery.

## Technical approach

Follow the "Cursor terminal prompt-stream loss" section of the Cursor design.
Add `cursorPromptStreamClosed(err error) bool` to `dialect_cursor.go`. It
requires `remotePromptRequestError`, the exact code and message, and a
single-key `data` object whose `details` string matches exactly. Do not
reuse the dialect `continuationError` hook. That hook emits the fixed
`peer disconnected before response` text, which would misreport Cursor's
condition (`.30`).

In the `err != nil` branch of `Adapter.Prompt`, check the predicate after
`syncNotifQueue` and before `retainedACPApplicationError`. The check applies
only when `a.agentID == cursorAgentID` and the disposition is `retain_runtime`.
Move the event construction from the terminal `RetriableError` branch into a
small `sendCursorTransportFailure` helper, and call that helper from both
branches. Place the new code in a new file, `adapter_cursor_prompt_error.go`,
so `adapter_prompt.go` stays within the function limits.

Add the routing rule beside the other Cursor rules in `runtime_rules.go`. Share
the diagnostic constant through `acpcompat` or `routingerr` so both layers
compare the same text.

| Terminal reply | Expected disposition |
| --- | --- |
| Exact envelope, completed or pending foreground tools, retained runtime | `agent_transport_lost`, automatic hidden `continue` |
| Exact envelope, no assistant output or tool activity, current attempt proven pre-result safe | `agent_transport_lost`, at most one original-prompt replay per admitted attempt under the existing budget; never send `continue` |
| Exact envelope with background shell work or unresolved permission | `agent_transport_lost`, manual refusal |
| Bare `-32603`, `data` omitted or `null`, extra key, other `details`, wrong case | Unclassified manual recovery (unchanged) |
| Same envelope from a non-Cursor adapter | Unchanged generic behavior |
| `stopReason: end_turn` | Normal completion; no continuation |

## Tests

Write RED tests first. Each test must fail on the current base for the expected
reason:

- `TestCursorPromptStreamClosedContinuationEvidence`, in the new
  `adapter_cursor_prompt_error_test.go`. It replays the sanitized frames: one
  `agent_message_chunk`, one completed tool, and the exact error reply. It then
  asserts one `EventTypeError` with `ProviderError.Source == cursor_acp`, the
  constant message, `retain_runtime`, a `ContinuationSafety` snapshot that is
  `SafeFor` the generation, and no raw `data` in any event field. Today this
  test fails because the event carries an unclassified `Internal error` and no
  snapshot.
- `TestCursorPromptStreamClosedRejectsOtherShapes` is table-driven over the
  negative shapes above, including a non-Cursor adapter. It asserts that
  today's generic retained failure is unchanged, with no `ContinuationSafety`.
- `TestCursorPromptStreamClosedNoOutputKeepsReplayEvidence` uses the exact
  envelope without assistant output or tool activity and asserts safe empty
  evidence. `TestCursorPromptStreamClosedWithoutOutputReplaysOriginalPrompt`
  exercises the concrete-profile transient owner, which dispatches the cached
  original prompt once for the admitted retry and does not send `continue`.
- `TestCursorPromptStreamClosedRule`, in
  `routingerr/cursor_prompt_stream_closed_test.go`. The constant classifies as
  high-confidence `agent_transport_lost` for `cursor-acp` only. Prose that
  contains the phrase, and other providers, stay unclassified.

Orchestrator admission uses the semantic code, prompt-attempt evidence, and
snapshot. Existing V3 tests cover pending-tool continuation; the two no-output
regressions above cover the distinct pre-result replay disposition.

## E2E tests

No new E2E test was added. The existing
[`provider-interruption-continuation.spec.ts`](../../apps/web/e2e/tests/session/provider-interruption-continuation.spec.ts)
runs in the `chromium` project. Its mock-agent `read`, `write`, and `shell`
scenarios cover completed foreground tools and same-conversation continuation
(criteria `001.2` and `001.3`); `pending-v3` covers pending-tool continuation
(criteria `001.8` and `001.9`). Other desktop cases cover same-conversation
reload, hidden `continue`, Cancel, and refusal behavior (criteria `001.2`,
`001.3`, `001.6`, and `002.2`).

[`mobile-provider-interruption-continuation.spec.ts`](../../apps/web/e2e/tests/session/mobile-provider-interruption-continuation.spec.ts)
runs in the `mobile-chrome` project with the Pixel 5 profile. It covers mixed
completed and pending tools, same-conversation behavior, hidden `continue`, and
Cancel behavior (criteria `001.3`, `001.6`, `001.8`, `001.9`, and `002.2`). Both
specs use the mock-agent fixture;
neither sends Cursor ACP's exact `-32603` envelope or exercises its predicate.
The exact-envelope behavior in criterion `001.10`, including no-output original
prompt replay, is covered by the adapter and orchestrator Go regressions above.

## Work orders

- [x] [Task 01: Project Cursor prompt-stream loss](task-01-project-cursor-prompt-stream-loss.md)

## Verification results

- PASS `(cd apps/backend && go test -trimpath ./internal/agentctl/server/adapter/transport/acp -run 'Test.*(CursorPromptStreamClosed|Retriable|PromptError|Continuation)' -count=1)`
- PASS `(cd apps/backend && go test -trimpath ./internal/agent/runtime/routingerr -count=1)`
- PASS `(cd apps/backend && go test -trimpath ./internal/orchestrator -run 'Test.*(Interruption|Continuation|CursorPromptStreamClosed)' -count=1)`
- PASS `make -C apps/backend build-agentctl`
- PASS `bash scripts/lint-go-changed apps/backend/internal/agent/runtime/routingerr/runtime_rules.go apps/backend/internal/agent/runtime/routingerr/cursor_prompt_stream_closed_test.go apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor.go apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_cursor_prompt_error.go apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_cursor_prompt_error_test.go apps/backend/internal/orchestrator/event_handlers_transient_retained_runtime_test.go` (0 issues)
- PASS `python3 scripts/list-docs.py validate` (468 decisions and 2,127 specifications)
- PASS `python3 scripts/lint-spec-files.py --all`

## Risks

- Cursor may change the `details` text or add fields. The exact match then
  fails closed to today's manual recovery.
- If Cursor had already finished its reply, continuation costs one extra model
  request. ACP gives no signal to tell finished and unfinished work apart.
  This trade-off is recorded in the ADR.
- The reproduction launched backgrounded shell work. Such turns stay manual
  under `001.2`. Automatic continuation applies only when the foreground
  evidence is eligible.
