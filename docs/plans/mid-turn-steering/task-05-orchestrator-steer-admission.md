---
id: "05-orchestrator-steer-admission"
title: "Admit and order steers in the orchestrator"
status: done
wave: 3
depends_on: ["01-negotiate-steer-capability", "02-runtime-toggle", "04-adapter-steer-admission"]
plan: "plan.md"
spec: "../../specs/platform/mid-turn-steering.md"
---

# Task 05: Admit and order steers in the orchestrator

- **Acceptance:** A `RUNNING`, generating session accepts a direct prompt as a
  steer when the agent is steer-capable and the toggle is enabled; every other
  coarse state follows the existing admission table unchanged. With the toggle
  off, or an agent that does not advertise the capability, admission is exactly
  today's queue behavior.
- **Acceptance:** Order is preserved — while the session has any queued message,
  new input queues rather than steering. At most one unacknowledged steer exists
  per session; a second attempt queues.
- **Acceptance:** Admission stays check-and-claim under one lock, and a steer
  does not take the ordinary foreground claim that the predecessor still holds.
  Two concurrent steer attempts result in exactly one steer.
- **Verification:** `cd apps/backend && go test -race ./internal/orchestrator/...`
- **Files likely touched:**
  `apps/backend/internal/orchestrator/turn_activity.go`,
  `task_operations.go` (prompt admission and the queue/drain path),
  `service.go` (config field), plus focused tests including a concurrency test.
- **Dependencies:** Tasks 01, 02, 04.
- **Inputs:** Spec "What" (order rule, single in-flight steer), "State machine",
  and the admission table in
  `../../specs/platform/background-work-liveness.md`. ADR-0049's
  check-and-claim requirement and its `ErrAgentPromptInProgress` loser behavior
  are the precedent for the concurrency rule.
- **Risks:** The existing `QueueAndInterruptForPeerMessage` path
  (`task_operations.go`) is a *different* mechanism — parent→child MCP interrupt
  delivery — and must not be conflated with steering or altered here.
- **Output contract:** Report the admission predicate, the order and
  single-steer rules, the concurrency test evidence, `-race` results, and update
  only this task's status.

## Validation Results

Re-run on 2026-08-04 against the branch merged with `main`.

- `cd apps/backend && go test -race ./internal/orchestrator/...`: passed.
- Admission predicate, order rule (never jumps a non-empty queue) and the
  single-in-flight-steer rule are covered by `orchestrator/steer_test.go`,
  including its concurrent-submission case, under `-race`.
