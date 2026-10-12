import { IconUsersGroup } from "@tabler/icons-react";
import {
  getTaskStateIcon,
  shouldUsePermissionTaskIcon,
  shouldUseQuestionTaskIcon,
} from "@/lib/ui/state-icons";
import type { Task } from "@/components/kanban-card";

// Status markers that mask the launch spinner and gate the "resting" no-affordance
// case. Bundled together so renderTaskStatusIcon's own branching stays under the
// complexity limit — see the helpers below for what each flag means for masking.
type StatusMaskFlags = {
  needsMe: boolean;
  showInterrupted: boolean;
  showAutoStartFailed: boolean;
  parkedOnBackgroundWork: boolean;
  showWorkspaceOrphaned: boolean;
};

function hasNoStatusAffordance(
  showRunningSpinner: boolean,
  hasActivity: boolean,
  flags: StatusMaskFlags,
): boolean {
  return (
    !showRunningSpinner &&
    !flags.needsMe &&
    !hasActivity &&
    !flags.showInterrupted &&
    !flags.showAutoStartFailed &&
    !flags.parkedOnBackgroundWork &&
    !flags.showWorkspaceOrphaned
  );
}

// A "needs me" prompt (pending clarification / permission) must not be masked
// by the launch-spinner short-circuit — a mid-turn prompt can coincide with a
// coarse running state. Live foreground activity still wins, handled inside
// getTaskStateIcon. A failed auto-start or an orphaned workspace must not be
// masked either: startTask sets the task to SCHEDULING before the launch, so
// a session-less SCHEDULING/IN_PROGRESS task (whether from a launch failure
// or a vanished parent workspace) reads as showRunningSpinner=true, the exact
// shape both markers exist to surface. The parked affordance (AC-58) is
// likewise never masked by the generic spinner — it renders through
// getTaskStateIcon below.
function resolveForegroundActivity(
  task: Task,
  showRunningSpinner: boolean,
  flags: StatusMaskFlags,
): Task["foregroundActivity"] {
  const shouldForceGenerating =
    showRunningSpinner &&
    !flags.needsMe &&
    !flags.showAutoStartFailed &&
    !flags.parkedOnBackgroundWork &&
    !flags.showWorkspaceOrphaned &&
    task.foregroundActivity !== "background";
  return shouldForceGenerating ? "generating" : task.foregroundActivity;
}

/**
 * Resolves the card status icon, or null when the actions cluster shows none
 * (a resting done/todo task). The backend task-level MOST-ACTIVE-WINS
 * aggregate takes precedence: a background-running task shows the distinct
 * background affordance — even when its primary session has finished and
 * only a secondary session is still working, so it reads as working, not
 * done — while any generating session keeps the spinner. When the aggregate
 * is absent it falls back to the primary-session-driven spinner (covers
 * STARTING/SCHEDULING before a session reads RUNNING) or the pending-input
 * question icon.
 */
export function renderTaskStatusIcon(
  task: Task,
  showRunningSpinner: boolean,
  hasPendingClarification: boolean,
  hasPendingPermission: boolean,
) {
  const flags: StatusMaskFlags = {
    needsMe:
      shouldUseQuestionTaskIcon(task.state, hasPendingClarification) ||
      shouldUsePermissionTaskIcon(hasPendingPermission),
    showInterrupted: !!task.interrupted,
    showAutoStartFailed: !!task.autoStartFailed,
    parkedOnBackgroundWork: !!task.parkedOnBackgroundWork,
    showWorkspaceOrphaned: !!task.workspaceOrphaned,
  };
  const hasActivity =
    task.foregroundActivity === "generating" || task.foregroundActivity === "background";
  if (hasNoStatusAffordance(showRunningSpinner, hasActivity, flags)) return null;
  const foregroundActivity = resolveForegroundActivity(task, showRunningSpinner, flags);
  return getTaskStateIcon(task.state, "h-4 w-4", {
    hasPendingClarification,
    foregroundActivity,
    hasPendingPermission,
    interrupted: flags.showInterrupted,
    autoStartFailed: flags.showAutoStartFailed,
    parkedOnBackgroundWork: flags.parkedOnBackgroundWork,
    workspaceOrphaned: flags.showWorkspaceOrphaned,
  });
}

/**
 * The board's only window into a fan-out. `activeSubagentCount` is derived
 * from the live registry (never a mutable counter) and summed across a
 * task's sessions, so it needs no local reconciliation: at zero there is
 * nothing live and the chip is absent.
 */
export function renderSubagentCountChip(task: Task, label: string) {
  const count = task.activeSubagentCount ?? 0;
  if (count <= 0) return null;
  return (
    <span
      data-testid="task-subagent-count"
      title={label}
      aria-label={label}
      className="flex items-center gap-0.5 text-muted-foreground font-mono text-[10px]"
    >
      <IconUsersGroup className="h-3.5 w-3.5" aria-hidden="true" />
      {count}
    </span>
  );
}
