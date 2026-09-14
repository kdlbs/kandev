---
id: "05-idle-workspace-access"
title: "Reduce idle workspace access"
status: completed
wave: 5
depends_on: ["04-filesystem-access-diagnostics"]
plan: "plan.md"
requirements:
  - "REQ-WORKSPACES-LOCAL-REPOSITORIES-004"
acceptance_criteria:
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-004.4"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-004.5"
system_design:
  - "../../specs/workspaces/system-design/local-repositories.md"
---

# Task 05: Reduce Idle Workspace Access

## Summary

Complete one final file and Git scan at turn end. Then deliver paused mode to
agentctl and stop automatic retries for a denied tracker.

## In scope

- Use execution activity release as the turn-completion signal.
- Request one final workspace refresh before runtime interest ends.
- Deliver `paused` from the lifecycle mode aggregator.
- Change the 60-second no-push fallback to final scan and pause.
- Resume on focus, a new operation, or explicit retry.

## Out of scope

- Change fast or slow polling intervals.
- Attribute normal `~/.kandev/tasks` polling to macOS TCC prompts.
- Change repository discovery refresh behavior.

## Acceptance

- Lifecycle tests prove final refresh occurs before runtime interest is released.
- Gateway and process tests prove paused delivery and the no-push transition.
- Denial tests prove that only visible user activity can retry a denied tracker.

## Verification

```bash
cd apps/backend && rtk go test ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agentctl/server/process
cd apps/backend && rtk go test -race ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process
rtk make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/activity.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_subscription.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_subscription_test.go`
- `apps/backend/internal/agent/runtime/agentctl/workspace_state.go`
- `apps/backend/internal/agentctl/server/api/workspace_state.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/workspace_poll_mode.go`
- `apps/backend/internal/agentctl/server/process/workspace_monitor.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_poll.go`

## Dependencies

- Task 04 supplies the operation and denial diagnostics.

## Risks

- A pause before the final scan can leave a stale sidebar diff.
- A suppressed paused push can leave an idle tracker in slow mode.
- Detached final-refresh work can race with a new turn.

## Parallelism

`sequential`

## Inputs

- REQ-WORKSPACES-LOCAL-REPOSITORIES-004.
- Lifecycle activity leases and workspace mode aggregation.
- Agentctl poll-mode grace and refresh behavior.

## Results

Completed.

- Added the agentctl workspace refresh endpoint and lifecycle final file/Git
  scan before releasing turn activity.
- Updated workspace poll-mode and subscription delivery so idle workspaces
  pause without losing the final state, while manual refresh and user selection
  can recover access.
- Verification passed: targeted lifecycle, agentctl API, and process tests plus
  race-enabled lifecycle and process tests.
