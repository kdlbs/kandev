---
id: "01-acp-detached-lifecycle"
title: "ACP detached-work lifecycle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/fine-grained-background-running-status-indicator/spec.md"
---

# Task 01: ACP detached-work lifecycle

- **Acceptance:** Bash `backgroundTaskId` and async-subagent `agentId` launches
  produce protocol-neutral workload-start evidence without changing the launch
  card's terminal status; human-origin usage produces foreground-yield;
  task-notification-origin usage produces exactly one workload-complete event;
  prompt completion does not prematurely complete detached shell work.
- **Verification:** `cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp/... ./internal/agentctl/types/streams/...`
- **Files likely touched:**
  `internal/agentctl/types/streams/agent.go`,
  `internal/agentctl/server/adapter/transport/acp/adapter.go`,
  `adapter_tools.go`, `adapter_updates.go`, `monitor.go`, and focused tests in
  the same ACP package.
- **Dependencies:** None.
- **Inputs:** ADR-0049 detached-lifecycle amendment; captured Claude 0.60 Bash
  and async-subagent frame ordering; existing subagent, usage, and Monitor tests.
- **Output contract:** Report event shapes, correlations, files changed, tests
  run, lifecycle cleanup behavior, residual provider-data limits, and mark this
  task plus its plan checkbox done.

## Result

- Added provider-neutral `foreground_idle` and `background_complete` events
  from Claude ACP usage-origin metadata while preserving the paired context
  window update.
- Launch tool cards remain terminal; detached shell/async-subagent liveness is
  retained by the orchestrator until provider completion evidence.
- Verified with the ACP adapter package and live Claude 0.60 frame probes.
