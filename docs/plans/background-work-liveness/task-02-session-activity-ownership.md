---
id: "02-session-activity-ownership"
title: "Session activity ownership"
status: done
wave: 2
depends_on: ["01-acp-detached-lifecycle"]
plan: "plan.md"
spec: "../../specs/fine-grained-background-running-status-indicator/spec.md"
---

# Task 02: Session activity ownership

- **Acceptance:** Foreground claims/output always rank generating; turn close
  leaves detached registrations background-running and promptable; matching
  workload completion clears them; execution teardown clears all activity; task
  and session DTOs remain correct when coarse state is settled.
- **Verification:** `cd apps/backend && go test ./internal/orchestrator/... ./internal/task/service/... ./internal/task/dto/...`
- **Files likely touched:** `internal/orchestrator/turn_activity.go`,
  `service.go`, stream handlers and their focused tests,
  `internal/task/service/task_activity.go`, and session/task DTO enrichment.
- **Dependencies:** Task 01 event contract.
- **Inputs:** spec three-state precedence and live-propagation sections;
  existing claim-generation and activity-signal tests.
- **Output contract:** Report state transitions, teardown ownership, files
  changed, tests run, blockers/risks, and mark this task plus its plan checkbox
  done.

## Result

Foreground completion now yields to, rather than deletes, outstanding detached
registrations. Async launch-card completion preserves workload liveness;
provider completion retires one registration, and execution teardown remains
the full cleanup boundary. Settled session/task DTOs surface background while
stale settled generating values remain omitted. Focused orchestrator, task
service, and DTO suites pass.
