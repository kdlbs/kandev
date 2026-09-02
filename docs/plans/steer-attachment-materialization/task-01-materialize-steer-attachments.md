---
id: "01-materialize-steer-attachments"
title: "Materialize steer attachments"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
acceptance_criteria:
  - AC-TASKS-PROMPT-ATTACHMENTS-001.6
  - AC-TASKS-PROMPT-ATTACHMENTS-001.7
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
---

# Task 01: Materialize Steer Attachments

## Summary

Resolve claimed attachment descriptors before a steer dispatches into a live
turn. Keep the resolution outside the prompt lifecycle lock and make a
resolution failure terminal for that message.

## In scope

- Materialize in `SendPromptSteerWithDispatchCallback` before `tryDispatchSteer`.
- Add an explicit `attachmentsMaterialized` parameter to `sendPrompt`.
- Pass resolved descriptors through the steer fallback without re-uploading.
- Return the materialization error instead of dispatching or falling back.
- Record materialize uploads in `newMockAgentServer` for assertions.
- Add the three lifecycle regression tests named in the plan.

## Out of scope

- Claim admission, upload limits, retention, and delivery-mode selection.
- The Playwright scenario, which is Task 02.

## Acceptance

- A steer into a generating turn reaches agentctl with a resolved filename and
  no bare attachment ID.
- The bytes are uploaded exactly once when the steer falls back to an ordinary
  prompt.
- A materialization failure returns an error and dispatches no prompt frame.
- Existing steer generation and ordering behavior is unchanged.

## Verification

```bash
gofmt -l apps/backend/internal/agent/runtime/lifecycle
cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'TestSendPromptSteer' -count=1 -race
cd apps/backend && go test ./internal/agent/runtime/lifecycle -count=1
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_steer_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_test.go`
