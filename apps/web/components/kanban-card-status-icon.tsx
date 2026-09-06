"use client";

import { IconUsersGroup } from "@tabler/icons-react";
import {
  getTaskStateIcon,
  shouldUsePermissionTaskIcon,
  shouldUseQuestionTaskIcon,
} from "@/lib/ui/state-icons";
import type { Task } from "@/components/kanban-card";

// renderTaskStatusIcon resolves the card status icon, or null when the actions
// cluster shows none (a resting done/todo task). The backend task-level
// MOST-ACTIVE-WINS aggregate takes precedence: a
// background-running task shows the distinct background affordance — even when its
// primary session has finished and only a secondary session is still working, so
// it reads as working, not done — while any generating session keeps the spinner.
// When the aggregate is absent it falls back to the primary-session-driven spinner
// (covers STARTING/SCHEDULING before a session reads RUNNING) or the pending-input
// question icon.
export function renderTaskStatusIcon(
  task: Task,
  showRunningSpinner: boolean,
  hasPendingClarification: boolean,
  hasPendingPermission: boolean,
) {
  const showQuestionIcon = shouldUseQuestionTaskIcon(task.state, hasPendingClarification);
  const showPermissionIcon = shouldUsePermissionTaskIcon(hasPendingPermission);
  const needsMe = showQuestionIcon || showPermissionIcon;
  const showInterrupted = !!task.interrupted;
  const showAutoStartFailed = !!task.autoStartFailed;
  const hasActivity =
    task.foregroundActivity === "generating" || task.foregroundActivity === "background";
  const parkedOnBackgroundWork = !!task.parkedOnBackgroundWork;
  if (
    !showRunningSpinner &&
    !needsMe &&
    !hasActivity &&
    !showInterrupted &&
    !showAutoStartFailed &&
    !parkedOnBackgroundWork
  ) {
    return null;
  }
  // A "needs me" prompt (pending clarification / permission) must not be masked
  // by the launch-spinner short-circuit — a mid-turn prompt can coincide with a
  // coarse running state. Live foreground activity still wins, handled inside
  // getTaskStateIcon. A failed auto-start must not be masked either: startTask
  // sets the task to SCHEDULING before the launch, so a launch failure before
  // session creation leaves a session-less SCHEDULING/IN_PROGRESS task, which
  // reads as showRunningSpinner=true — the exact shape the failure marker exists
  // to surface. Parked tasks are excluded too, so they render the background
  // spinner rather than the plain launch spinner.
  const foregroundActivity =
    showRunningSpinner &&
    !needsMe &&
    !showAutoStartFailed &&
    !parkedOnBackgroundWork &&
    task.foregroundActivity !== "background"
      ? "generating"
      : task.foregroundActivity;
  return getTaskStateIcon(task.state, "h-4 w-4", {
    hasPendingClarification,
    foregroundActivity,
    hasPendingPermission,
    interrupted: showInterrupted,
    autoStartFailed: showAutoStartFailed,
    parkedOnBackgroundWork,
  });
}

// The board's only window into a fan-out. `activeSubagentCount` is derived from
// the live registry (never a mutable counter) and summed across a task's
// sessions, so it needs no local reconciliation: at zero there is nothing live
// and the chip is absent.
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
