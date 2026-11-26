---
created: 2026-11-22
status: completed
requirements:
  - REQ-TASKS-SESSION-TURN-SETTLEMENT-002
  - REQ-TASKS-RESUME-PROMPT-QUEUE-001
system_design:
  - ../../specs/tasks/system-design/session-turn-settlement.md
  - ../../specs/tasks/system-design/resume-prompt-queue.md
legacy_specs: []
---

# Fix plan for Codex ACP MCP startup warnings

## Outcome

A Codex ACP session can finish resuming and deliver eligible queued prompts
when an optional MCP server fails to start. The conversation shows the failure
through its existing status warning on desktop and phone.
[#5419](https://github.com/kdlbs/kandev/issues/5419).

## Root cause and reproduction

The Codex ACP reporter emits a standalone `tool_call` in state `failed`.
Its `mcp_startup.<encoded-server>.<UUID>` ID identifies a startup report.
The provider sends the report outside `session/prompt`.
[Codex ACP 2.8.0 reporter](https://github.com/agentclientprotocol/codex-acp/blob/ad09e0ead18511a65a0761cac832aa3087219b10/src/tool-calls/reporters/McpStartupReporter.ts#L29-L38).

In `adapter_tools.go`, the initial tool call preserves the ACP status `failed`.
`isAsyncToolTerminal` in `adapter_async_complete.go` omits `failed` from its
terminal statuses. The finalizer records an active tool and waits for terminal evidence.
`recordActivity` in `manager_events.go` promotes a Ready execution to Running.
The orchestrator's `handleToolCallEvent` opens a turn and marks the session
Running. The queue requires a promptable session.

The smallest reproduction injects one matching report into a resumed session
with a completed work turn. With a pending prompt and Auto-run ON, run the
queue admission check after the report. The SQLite reproduction recorded a
`RUNNING` session, one open turn, and one pending prompt.

## Technical approach

Recognize the Codex report once, at the ACP adapter. Carry it as a session
warning through the existing event stream. The orchestrator uses its existing
idempotent writer for status messages in a `lifecycle_only` turn. The regular
boot and queue handlers handle readiness and dispatch.
[Session turn settlement design](../../specs/tasks/system-design/session-turn-settlement.md#mcp-startup-reports).

The check uses the reserved startup ID, kind, status, and title together. An
older agentctl stream can carry the same report as a tool call. The lifecycle
callback uses the same check for that case.

## Tests

The focused Go regressions check warning classification, lifecycle readiness,
and actual queue delivery with SQLite. Use a genuine failed tool call as a
control. Those tests cover the startup warning and Auto-run admission criteria.

The desktop and phone resume queue specs register the mock ACP peer under the
`codex-acp` identity and emit a matching startup report during `session/load`.
Each spec checks that the warning is visible, a prompt is accepted into the
queue while resume is pending, and the prompt is delivered once after the
session becomes ready. The SQLite regression checks that Kandev stores the
warning in the message format rendered by both layouts.

The browser coverage lives in
`apps/web/e2e/tests/session/session-resume-prompt-queue.spec.ts` and
`apps/web/e2e/tests/session/mobile-session-resume-prompt-queue.spec.ts`.

## Work orders

- [x] [Task 01. Keep startup reports out of work turns](task-01-keep-startup-reports-out-of-turns.md)

## Verification results

Three temporary Go probes reproduced the failure on `c68b3340d72`. The ACP
probe armed a finalizer with one active tool. The lifecycle probe published
`agent.running`. The SQLite probe left the session Running, with one open turn
and one queued prompt. Each probe failed its expected-state assertion.

The existing ACP finalizer, lifecycle wake-up, and orchestrator queue tests
passed. The spec catalog, spec lint, and PR documentation reference checks
passed.

Implementation classifies the pinned Codex report before tool tracking, carries
it as a sanitized session warning, and stores it as an idempotent lifecycle-only
status message. Startup warnings do not acquire prompt submission ownership or
change Ready state. The existing boot and queue paths dispatch eligible work.

Verification passed:

- Focused Go regressions for ACP session creation and load reports, active-prompt
  capacity evidence, lifecycle readiness, legacy and durable submission
  ownership, watchdog cancellation identity, and SQLite queue admission.
- `go test -trimpath -count=1 ./internal/common/acpcompat
  ./internal/agentctl/acpcompat ./cmd/mock-agent`.
- `go build -trimpath ./...` in `apps/backend`.
- Desktop Chromium resume and prompt queue spec: 4 passed, including a report
  emitted during restore and one queued prompt delivered after readiness.
- Mobile Chrome resume and prompt queue spec: 2 passed, with the same warning
  visibility and queued delivery assertions.
- `git diff --check`.
- Changed-package Go lint produced zero issues but did not finish before the
  repository runner's five-minute timeout.

## Risks

Codex can deliver the warning while a prompt is active. The regression checks
that the warning and the prompt each retain their own identity. The probes
exercise the shared backend path with synthetic ACP data and SQLite.
