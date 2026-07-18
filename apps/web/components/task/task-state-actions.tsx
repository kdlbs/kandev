"use client";

import { getTaskStateIcon } from "@/lib/ui/state-icons";
import type { ForegroundActivity, TaskState } from "@/lib/types/http";

type TaskStateActionsProps = {
  state?: TaskState;
  className?: string;
  /**
   * Task-level MOST-ACTIVE-WINS activity aggregate (§spec:task-level-indicator).
   * When set it drives the open-task header status icon so a background-running
   * task shows the distinct background affordance rather than a done check.
   */
  foregroundActivity?: ForegroundActivity | null;
};

export function TaskStateActions({ state, className, foregroundActivity }: TaskStateActionsProps) {
  return (
    <div className="flex items-center justify-end">
      {getTaskStateIcon(state, className, false, foregroundActivity)}
    </div>
  );
}
