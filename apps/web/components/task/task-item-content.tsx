"use client";

import { ScrollOnOverflow } from "@kandev/ui/scroll-on-overflow";
import { useTranslation } from "react-i18next";
import type { WipQueueStatus } from "@/lib/kanban/wip-queue";
import { RemoteCloudTooltip } from "./remote-cloud-tooltip";
import { TaskItemComparisonUnavailable } from "./task-item-comparison-unavailable";
import { TaskItemLeadingBadges } from "./task-item-leading-badges";
import { TaskItemStatsRow } from "./task-item-stats-row";
import { TaskRowMetadata } from "./task-row-plugin-slots";

export type TaskItemContentProps = {
  title: string;
  autopilot?: boolean;
  taskId?: string;
  workflowStepId?: string | null;
  isRemoteExecutor?: boolean;
  remoteExecutorType?: string;
  remoteExecutorName?: string;
  primarySessionId?: string | null;
  isArchived?: boolean;
  isPinned?: boolean;
  repositoryPath?: string;
  showRepository: boolean;
  updatedAt?: string;
  lastActivityAt?: string;
  showActivityTime?: boolean;
  prInfo?: { number: number; state: string; aggregateState?: string };
  queuedCount?: number;
  wipQueue?: WipQueueStatus;
  issueInfo?: { url: string; number: number };
  agentErrorMessage?: string | null;
  comparisonUnavailable?: boolean;
};

export function TaskItemContent({
  title,
  autopilot,
  taskId,
  workflowStepId,
  isRemoteExecutor,
  remoteExecutorType,
  remoteExecutorName,
  primarySessionId,
  isArchived,
  isPinned,
  repositoryPath,
  showRepository,
  updatedAt,
  lastActivityAt,
  showActivityTime,
  prInfo,
  queuedCount,
  wipQueue,
  issueInfo,
  agentErrorMessage,
  comparisonUnavailable,
}: TaskItemContentProps) {
  const { t } = useTranslation();
  return (
    <div className="flex min-w-0 flex-1 flex-col gap-0.5">
      <span className="flex min-w-0 items-center gap-1 text-[13px] font-medium leading-tight text-foreground">
        <ScrollOnOverflow className="min-w-0">{title}</ScrollOnOverflow>
        <TaskItemLeadingBadges
          autopilot={autopilot}
          isPinned={isPinned}
          taskId={taskId}
          prInfo={prInfo}
          issueInfo={issueInfo}
          agentErrorMessage={agentErrorMessage}
        />
        <TaskItemComparisonUnavailable unavailable={comparisonUnavailable} />
        {isRemoteExecutor && (
          <RemoteCloudTooltip
            taskId={taskId ?? ""}
            sessionId={primarySessionId ?? null}
            executorType={remoteExecutorType}
            fallbackName={remoteExecutorName ?? remoteExecutorType}
            iconClassName="h-3 w-3 text-muted-foreground/60"
          />
        )}
        {isArchived && (
          <span className="rounded bg-amber-500/15 px-1 py-px text-[10px] text-amber-500">
            {t("task:filterDimensionArchived")}
          </span>
        )}
      </span>
      {taskId && (
        <TaskRowMetadata
          taskId={taskId}
          workflowStepId={workflowStepId ?? null}
          surface="sidebar"
        />
      )}
      <TaskItemStatsRow
        updatedAt={showActivityTime ? (lastActivityAt ?? updatedAt) : updatedAt}
        repositoryLabel={showRepository ? repositoryPath : undefined}
        prInfo={prInfo}
        primarySessionId={primarySessionId}
        queuedCount={queuedCount}
        wipQueue={wipQueue}
      />
    </div>
  );
}
