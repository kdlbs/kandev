"use client";

import { IconGitPullRequest } from "@tabler/icons-react";
import {
  AutomationIndicatorDots,
  getPRAggregateStatusColor,
  getTaskPRAutomationSummary,
  PRTaskIcon,
  type TaskPRInfo,
} from "@/components/github/pr-task-icon";
import { MRTaskIcon } from "@/components/gitlab/mr-task-icon";
import { cn } from "@/lib/utils";

/** Shows PR icon from store (real data) or from prInfo prop (prototype/mock). */
function TaskPRIcon({ taskId, prInfo }: { taskId?: string; prInfo?: TaskPRInfo }) {
  if (taskId) return <PRTaskIcon taskId={taskId} prInfo={prInfo} />;
  if (!prInfo) return null;
  const color = getPRAggregateStatusColor(prInfo.aggregateState ?? prInfo.state);
  const automation = getTaskPRAutomationSummary([], prInfo);
  return (
    <span
      data-testid={taskId ? `pr-task-icon-${taskId}` : "pr-task-icon"}
      data-pr-state={prInfo.state}
      className={cn("inline-flex items-center shrink-0", color)}
    >
      <span className="relative inline-flex h-3.5 w-3.5 shrink-0">
        <IconGitPullRequest className="h-3.5 w-3.5" />
        <AutomationIndicatorDots
          autoFixEnabled={automation.autoFixEnabled}
          autoMergeEnabled={automation.autoMergeEnabled}
        />
      </span>
    </span>
  );
}

/**
 * PR badge (store or prInfo fallback) and MR badge as siblings, PR first. The
 * MR branch needs its own taskId guard because MRTaskIcon reads task-scoped
 * store data and cannot render without a task identity.
 */
export function TaskContributionIcons({
  taskId,
  prInfo,
}: {
  taskId?: string;
  prInfo?: TaskPRInfo;
}) {
  return (
    <>
      <TaskPRIcon taskId={taskId} prInfo={prInfo} />
      {taskId ? <MRTaskIcon taskId={taskId} /> : null}
    </>
  );
}
