---
id: "01-queue-peer-message-behind-live-question"
title: "Queue peer messages behind a live question"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CLARIFICATION-LIFECYCLE-001
acceptance_criteria:
  - AC-TASKS-CLARIFICATION-LIFECYCLE-001.10
system_design:
  - ../../specs/tasks/system-design/clarification-active-lifecycle.md
---

# Task 01: Queue Peer Messages Behind a Live Question

## Summary

Before `dispatchTaskMessage` prompts a non-running session for a queued-mode
peer message, ask the orchestrator whether a live clarification owns the turn.
If it does, queue the message. Repeat the check on the selected session after
workflow preparation, releasing any start admission before queueing.

## In scope

- `liveClarificationReader`, the queued branch in `dispatchTaskMessage`, and
  the selected-session check in `dispatchPreparedTaskMessageWithAdmissionAndInterrupt`.
- `Service.SessionHasLiveClarification`, a public accessor for the existing
  queue-drain rule.
- Tests in `message_task_live_clarification_test.go` and
  `clarification_guard_test.go`, plus the answer-to-ready delivery regression in
  `peer_message_clarification_delivery_test.go`.
- Interrupt-mode and legacy-launcher compatibility controls, repository-error
  fail-closed coverage, and the public messaging delivery table.
- AC `.10` and the system-design section "Peer messages during a live question".

## Verification

- `go test ./internal/mcp/handlers/ ./internal/orchestrator/`
- The queued-behind-live-question test fails without the handler change.

## Review remediation

- A workflow-selected session receives the same live-question barrier as the
  originally addressed session. The regression proves that neither prompt nor
  start is attempted and that only the selected session receives a queued entry.
- The answer-to-ready regression claims the durable answer, invokes clarification
  answer handling, drives the ready event, joins actual peer dispatch, and verifies
  one provider prompt plus preservation of its successor after a stale ready event.
- Hosted mutation proofs exercise removal of admission and ready-drain behavior.
  A shared selected-session control accompanies the genuine selected-target RED.
- Verification runs on hosted infrastructure. Full package coverage belongs to the
  canonical backend suite; exact source, raw Go JSON, and review evidence are retained
  with the PR review receipts.
