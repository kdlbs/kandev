"use client";

import { IconGitPullRequest } from "@tabler/icons-react";
import { getPRAggregateStatusColor, PRTaskIcon } from "@/components/github/pr-task-icon";
import { MRTaskIcon } from "@/components/gitlab/mr-task-icon";
import { useAppStore } from "@/components/state-provider";
import { cn } from "@/lib/utils";

/** Shows PR icon from store (real data) or from prInfo prop (prototype/mock). */
function TaskPRIcon({
  taskId,
  prInfo,
}: {
  taskId?: string;
  prInfo?: { number: number; state: string; aggregateState?: string };
}) {
  const hasStorePR = useAppStore((s) => !!taskId && (s.taskPRs.byTaskId[taskId]?.length ?? 0) > 0);
  if (hasStorePR) return <PRTaskIcon taskId={taskId!} />;
  if (!prInfo) return null;
  const color = getPRAggregateStatusColor(prInfo.aggregateState ?? prInfo.state);
  return (
    <span
      data-testid={taskId ? `pr-task-icon-${taskId}` : "pr-task-icon"}
      data-pr-state={prInfo.state}
      className={cn("inline-flex items-center shrink-0", color)}
    >
      <IconGitPullRequest className="h-3.5 w-3.5" />
    </span>
  );
}

/**
 * PR badge (store or prInfo fallback) and MR badge as siblings, PR first. The
 * MR branch needs its own taskId guard: hasStorePR inside TaskPRIcon is false
 * both when taskId is absent and when it's present with no PRs, so it can't
 * gate the MRTaskIcon call.
 */
export function TaskContributionIcons({
  taskId,
  prInfo,
}: {
  taskId?: string;
  prInfo?: { number: number; state: string; aggregateState?: string };
}) {
  return (
    <>
      <TaskPRIcon taskId={taskId} prInfo={prInfo} />
      {taskId ? <MRTaskIcon taskId={taskId} /> : null}
    </>
  );
}
