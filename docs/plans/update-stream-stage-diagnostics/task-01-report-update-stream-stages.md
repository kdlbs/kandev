---
id: "01-report-update-stream-stages"
title: "Report bounded update stream stages"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-STALL-RECOVERY-001
acceptance_criteria:
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.9
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.13
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.15
system_design:
  - ../../specs/agents/system-design/agent-stall-recovery.md
---

# Task 01: Report bounded update stream stages

## Outcome

The first-stall diagnostic record includes bounded stage counters and ages for
adapter updates, agentctl writes, backend reads, and durable projection, so a
stalled turn shows which stage stopped making progress.

## In scope

- agentctl counts adapter updates (including drained ones) and stream writes,
  and sends an internal stage heartbeat only when the backend requests that
  capability.
- The lifecycle manager records backend reads and completed projection per
  stream owner and adds the stage snapshot to the stall diagnostic.

## Exclusions

- No change to durable replay order, MCP messages, the stall threshold, or the
  one-record diagnostic rule.
- Active-stream reattachment, accepted-queue successor recovery, and stop-wait
  snapshots are separate behaviors.

## Acceptance conditions

1. Stage heartbeats and counters are metadata: they are not turn events and do
   not restart the inactivity clock (`AC-AGENTS-AGENT-STALL-RECOVERY-001.9`) or
   prove acceptance (`AC-AGENTS-AGENT-STALL-RECOVERY-001.13`).
2. The diagnostic stays one bounded record per stall; unavailable or stale
   measurements remain unknown, and a later connection's projection counters
   cannot be replaced by an older connection
   (`AC-AGENTS-AGENT-STALL-RECOVERY-001.15`).
3. A retained older agentctl peer that omits the heartbeat keeps working.

## Verification

Focused existing durable replay, final-wakeup, and projection-batch tests in
`internal/agentctl/server/api`, `internal/agentctl/server/process`, and
`internal/agent/runtime/lifecycle`, plus the existing stall diagnostic tests.

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/update_stage.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/api/update_stage.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/update_stage.go`
- `apps/backend/internal/agent/runtime/agentctl/agent.go`
- `apps/backend/internal/agent/runtime/agentctl/update_stage.go`
- `apps/backend/internal/agent/runtime/lifecycle/update_stream_stage.go`
- `apps/backend/internal/agent/runtime/lifecycle/stall_diagnostic.go`
- `apps/backend/internal/agent/runtime/lifecycle/stream_event_processor.go`

## Dependencies

None.
