---
id: "01-keep-startup-reports-out-of-turns"
title: "Keep startup reports out of work turns"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SESSION-TURN-SETTLEMENT-002
  - REQ-TASKS-RESUME-PROMPT-QUEUE-001
acceptance_criteria:
  - AC-TASKS-SESSION-TURN-SETTLEMENT-002.7
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.3
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.5
system_design:
  - ../../specs/tasks/system-design/session-turn-settlement.md
  - ../../specs/tasks/system-design/resume-prompt-queue.md
---

# Task 01. Keep startup reports out of work turns

## Summary

Convert the Codex ACP startup report into a status warning so the resumed
session can accept queued work.

## Scope

1. Write `TestCodexMCPStartupWarningDoesNotArmAsyncTurn` using the pinned ACP
   frame and `handleACPUpdate`. The current code arms a finalizer for the report.
2. Recognize the Codex ACP report before tool handling. Check its failed
   status, `other` kind, reserved `mcp_startup.` ID, and startup title. Put the
   predicate in `internal/common/acpcompat` so the lifecycle callback and
   standalone agentctl binary share the same classification. Ordinary failed
   calls use the existing tool handler.
3. Carry the report as `EventTypeMCPStartupWarning` with the existing text and
   report ID fields. Sanitize the provider detail or title with
   `streams.SanitizeProviderMessage`. Add the event to the metadata retention
   list. Skip prompt submission attribution and publish it before `recordActivity`.
4. Route the warning after stream ownership checks and before turn handling.
   Use `CreateSessionLifecycleMessageIdempotent` to write a `status` message
   with `variant: warning`. Derive the message ID from the owning session,
   execution, and report IDs. Let boot-ready and queue admission drive dispatch.

## Acceptance

- A matching report leaves the current session state and prompt ownership
  intact. The status warning survives reload and replay as one message.
- With Auto-run ON, the normal handlers dispatch the eligible queue head once.
  Auto-run OFF keeps it pending. Genuine tool calls still drive work turns.

## Regression cases

- `TestCodexMCPStartupWarningDoesNotArmAsyncTurn` checks the report, a genuine
  failed tool with a similar title, and a report during an active prompt; the
  active-prompt assertion checks that capacity evidence remains clean.
- `TestCodexMCPStartupWarningPassesDuringOwnedLoad` checks that the warning
  passes its restore owner while ordinary and stale replay calls stay suppressed.
- `TestCodexMCPStartupWarningBuffersUntilNewSessionBinding` checks reports
  emitted before `session/new` returns; only the returned session's warning is
  released after its binding is installed.
- `TestCodexMCPStartupWarningBypassesTurnActivity` checks a Ready execution
  with the warning and with the older normalized tool-call form.
- `TestCodexMCPStartupWarningDoesNotOpenTurnOrStrandQueue` uses SQLite and the
  real stream handler. Check a late warning, zero open turns before dispatch,
  one provider dispatch, and one stored warning after replay.
- `TestCodexMCPStartupWarningDoesNotClaimSubmission` and
  `TestCodexMCPStartupWarningUsesMetadataRetention` check the two delivery
  entries in the existing test files. Reuse the desktop and phone resume queue
  specs for the queue controls.
- `TestCodexMCPStartupReportDoesNotAdoptLegacySubmission` and
  `TestCodexMCPStartupReportKeepsDurableReceiptWithoutAdoptingSubmission`
  check legacy and durable stream admission, including preservation of the
  durable report's original submission receipt.
- `TestRejectedMCPStartupWarningDoesNotCancelClarificationWatchdog` checks
  cancellation identity rejection without removing recovery state.
- Desktop and mobile browser cases in the resume queue specs check a visible
  startup warning, queued prompt admission during restore, and one delivery
  after readiness.

## Verification

```bash
(cd apps/backend && go test -tags fts5 -trimpath -count=1 \
  -run '^(Test(CodexMCPStartup(Warning|Report)|RejectedMCPStartupWarning)|TestWakeup_|TestHandleAgentBootReady_DrainsOrphanedQueuedMessage|TestCheckQueueAdmissionReadiness)' \
  ./internal/agentctl/server/adapter/transport/acp \
  ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle \
  ./internal/orchestrator ./internal/task/repository/sqlite)
(scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium tests/session/session-resume-prompt-queue.spec.ts)
(scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/session/mobile-session-resume-prompt-queue.spec.ts)
(cd apps/backend && go build -trimpath ./...)
(cd apps/backend && go test -trimpath -count=1 \
  ./internal/common/acpcompat ./internal/agentctl/acpcompat ./cmd/mock-agent)
git diff --check
```

## Files likely touched

- `apps/backend/internal/common/acpcompat/codex_mcp_startup.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go`, `adapter_session.go`, and `adapter_session_startup_warning_test.go`
- `apps/backend/cmd/mock-agent/main.go`
- `apps/backend/internal/agent/runtime/lifecycle/streams_events.go` and `streams_events_codex_mcp_startup_test.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go` and `event_handlers_mcp_startup_warning_test.go`
- `apps/web/e2e/helpers/session-resume-prompt-queue.ts`
- `apps/web/e2e/tests/session/session-resume-prompt-queue.spec.ts` and `mobile-session-resume-prompt-queue.spec.ts`

## Dependencies

None. Implement classification first, then the warning projection and queue
regression.

## Risks

A genuine tool can have a similar title. Check the reserved report identity
before taking the warning path.

## Parallelism

`sequential`

## Inputs

- [MCP startup reports](../../specs/tasks/system-design/session-turn-settlement.md#mcp-startup-reports)
- [Resume queue design](../../specs/tasks/system-design/resume-prompt-queue.md)
- [ADR 0035](../../decisions/0035-version-agent-ready-events-by-prompt-generation.md)
- `createTestManagerWithTracking`, `newResumeTodoFixture`, and `CheckQueueAdmissionReadiness`

## Results

Implemented and verified:

- The focused Go regression command passed for all five packages.
- ACP session creation and load reports, clean capacity evidence during an
  active prompt, legacy and durable stream admission, and rejected-warning
  watchdog regressions passed.
- The common classifier, retained agentctl compatibility package, and mock
  agent package tests passed.
- The complete backend build passed with `go build -trimpath ./...`.
- Desktop Chromium resume and prompt queue checks passed: 4 tests.
- Mobile Chrome resume and prompt queue checks passed: 2 tests.
- `git diff --check` passed.
- Changed-package Go lint reported zero issues but exceeded the five-minute
  runner timeout.
- `gofmt -l` reported no unformatted Go files.
