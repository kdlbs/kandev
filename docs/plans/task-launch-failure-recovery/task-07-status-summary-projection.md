---
id: "07-status-summary-projection"
title: "Project bounded launch errors"
status: done
wave: 4
depends_on: ["01-failure-taxonomy-contracts", "04-launch-failure-classification", "05-pr-review-autostart-gating"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-launch-failure-recovery.md"
---

# Task 07: Project bounded launch errors

Project session-owned and task-owned errors through one bounded status contract.

- **Acceptance:**
  1. `active_error` gains optional session and task-repository IDs, category, and actions.
  2. Category has a 64-byte UTF-8 bound. Row identity has a 256-byte bound.
  3. Actions accept known values only, keep source order, remove duplicates, and stop at three.
  4. Live projection and rebuild read both persisted error sources.
  5. The newest active record wins by occurrence time, then stable stamp.
  6. Malformed task metadata is ignored and cannot invalidate the full summary.
  7. DTO, API conversion, boot reads, and status-summary events carry the complete shape.
  8. No transcript or unbounded provider payload enters the summary.

- **Verification:**
  `cd apps/backend && go test ./internal/task/... ./internal/orchestrator/... -race`

- **Files likely touched:**
  `apps/backend/internal/task/statussummary/model.go`,
  projector and rebuild files under that package,
  `apps/backend/internal/task/service/service_status_summary_rebuild.go`,
  owning DTO conversion files and focused tests.

- **Dependencies:** Tasks 01, 04, and 05.
- **Parallelism:** sequential.
- **Inputs:** both affected specs and plan "Bounded status projection".

## Results

- Extended the bounded active-error contract with task/session ownership,
  category, repository row, stamp, and normalized recovery actions.
- Added authoritative rebuild and live projector handling for both task-owned
  and session-owned errors, including malformed-task metadata preservation and
  deterministic timestamp/stamp ties.
- Updated task-service rebuild and gateway conversion paths, with focused
  bounds, action, malformed-source, and task-error lifecycle tests.
- Verification: `cd apps/backend && go test ./internal/task/... ./internal/orchestrator/... -race`
  passed (5,749 tests in 20 packages).
