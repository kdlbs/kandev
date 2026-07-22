---
id: "06-publish-completion-foreground-yield"
title: "Publish completion-time foreground yield"
status: done
wave: 6
depends_on: ["02-session-activity-ownership"]
plan: "plan.md"
spec: "../../specs/fine-grained-background-running-status-indicator/spec.md"
---

# Task 06: Publish completion-time foreground yield

- **Acceptance:** Normal foreground-turn completion without an explicit
  provider idle frame publishes exactly one per-session background activity
  update and the corresponding task aggregate update when detached work remains;
  repeated completion is idempotent.
- **Verification:** `cd apps/backend && go test -race ./internal/orchestrator/...`
- **Files likely touched:**
  `apps/backend/internal/orchestrator/turn_activity.go`, `service.go`,
  `event_handlers_streaming.go`, and focused activity-signal tests.
- **Dependencies:** Existing detached-work ownership from Task 02.
- **Inputs:** Spec live-propagation requirement; diagnosis that
  `completeTurnForSession` discards the changed result from
  `markForegroundIdle`.
- **Output contract:** Report RED/GREEN evidence, event cardinality, files
  changed, commands run, blockers/risks, and update only this task status.
