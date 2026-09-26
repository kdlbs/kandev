"use client";

import { getTaskStateIcon } from "@/lib/ui/state-icons";
import type { ForegroundActivity, TaskState } from "@/lib/types/http";

type TaskStateActionsProps = {
  state?: TaskState;
  className?: string;
  /**
   * Task-level MOST-ACTIVE-WINS activity aggregate.
   * When set it drives the open-task header status icon so a background-running
   * task shows the distinct background affordance rather than a done check.
   */
  foregroundActivity?: ForegroundActivity | null;
  /** Message-derived pending-clarification flag. */
  hasPendingClarification?: boolean;
  /** Message-derived pending-permission flag. */
  hasPendingPermission?: boolean;
  /** True when the task's session was mid-turn when the backend died. */
  interrupted?: boolean;
};

export function TaskStateActions({
  state,
  className,
  foregroundActivity,
  hasPendingClarification = false,
  hasPendingPermission = false,
  interrupted = false,
}: TaskStateActionsProps) {
  return (
    <div className="flex items-center justify-end">
      {getTaskStateIcon(state, className, {
        hasPendingClarification,
        foregroundActivity,
        hasPendingPermission,
        interrupted,
      })}
    </div>
  );
}
