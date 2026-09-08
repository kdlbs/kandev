---
id: "12-remove-legacy-launch-guidance"
title: "Remove legacy launch guidance"
status: done
wave: 4
depends_on: ["04-launch-failure-classification"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-launch-failure-recovery.md"
---

# Task 12: Remove legacy launch guidance

Remove the warning-message path that conflicts with typed launch errors.
Keep unrelated recovery and toast behavior.

- **Acceptance:**
  1. Handled missing-branch errors create no warning message or archive/delete action metadata.
  2. The handled path writes no `missing_pr_branch_recovery_claimed` key.
  3. The handled path does not set `suppressToast`, so the pointer toast remains available.
  4. Unrelated auth, resume, and bootstrap toast suppression stays unchanged.
  5. Existing missing-branch tests now assert the typed-error ownership boundary.

- **Verification:**
  `cd apps/backend && go test ./internal/orchestrator/... -race`

- **Files likely touched:**
  `apps/backend/internal/orchestrator/service.go`,
  `apps/backend/internal/orchestrator/task_launch_failure_test.go`,
  related task-operation tests that assert the old claim or message.

- **Dependencies:** Task 04.
- **Parallelism:** sequential.
- **Inputs:** spec "Failure modes" and plan "Legacy launch-guidance removal".

## Results

- Removed the handled launch-failure warning, archive/delete actions, toast
  suppression, and legacy `missing_pr_branch_recovery_claimed` write.
- Removed the unused compatibility callback so older integrations cannot
  recreate the removed guidance path.
- Updated the focused launch-failure tests to assert the executor ownership
  boundary. Early workspace failures remain neutral until executor
  classification exists.
- Verification: `cd apps/backend && go test ./internal/orchestrator/... -race`
  passed (2,841 tests in 9 packages).
