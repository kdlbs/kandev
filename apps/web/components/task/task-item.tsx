"use client";

import { memo } from "react";
import {
  IconCircleCheck,
  IconCircleDashed,
  IconDots,
  IconGitPullRequest,
} from "@tabler/icons-react";
import { PRTaskIcon } from "@/components/github/pr-task-icon";
import { useAppStore } from "@/components/state-provider";
import { cn } from "@/lib/utils";
import type { TaskState, TaskSessionState } from "@/lib/types/http";
import { RemoteCloudTooltip } from "./remote-cloud-tooltip";
import { ScrollOnOverflow } from "@kandev/ui/scroll-on-overflow";

type DiffStats = {
  additions: number;
  deletions: number;
};

type TaskItemProps = {
  title: string;
  description?: string;
  stepName?: string;
  state?: TaskState;
  sessionState?: TaskSessionState;
  isArchived?: boolean;
  isSelected?: boolean;
  onClick?: () => void;
  diffStats?: DiffStats;
  isRemoteExecutor?: boolean;
  remoteExecutorType?: string;
  remoteExecutorName?: string;
  updatedAt?: string;
  menuOpen?: boolean;
  isDeleting?: boolean;
  taskId?: string;
  primarySessionId?: string | null;
  parentTaskTitle?: string;
  isSubTask?: boolean;
  repositories?: string[];
  prInfo?: { number: number; state: string };
  hasDiffStats?: boolean;
};

function formatRelativeTime(dateString: string): string {
  const date = new Date(dateString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSecs = Math.floor(diffMs / 1000);
  const diffMins = Math.floor(diffSecs / 60);
  const diffHours = Math.floor(diffMins / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffSecs < 60) return "just now";
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString();
}

function handleTaskItemKeyDown(e: React.KeyboardEvent<HTMLDivElement>, onClick?: () => void): void {
  if (e.key !== "Enter" && e.key !== " ") return;
  e.preventDefault();
  onClick?.();
}

function TaskStateIcon({
  sessionState,
  state,
  isInProgress,
}: {
  sessionState?: TaskSessionState;
  state?: TaskState;
  isInProgress: boolean;
}) {
  if (isInProgress) {
    return <IconCircleDashed className="mt-[1px] h-3.5 w-3.5 shrink-0 text-yellow-500 animate-spin" />;
  }
  const isReview =
    sessionState === "WAITING_FOR_INPUT" ||
    sessionState === "COMPLETED" ||
    sessionState === "FAILED" ||
    sessionState === "CANCELLED" ||
    state === "REVIEW" ||
    state === "COMPLETED";
  if (isReview) {
    return <IconCircleCheck className="mt-[1px] h-3.5 w-3.5 shrink-0 text-green-500" />;
  }
  return <IconCircleDashed className="mt-[1px] h-3.5 w-3.5 shrink-0 text-muted-foreground/40" />;
}

function TaskItemStatsRow({
  updatedAt,
  prInfo,
}: {
  updatedAt?: string;
  prInfo?: { number: number; state: string };
}) {
  if (!updatedAt && !prInfo) return null;
  return (
    <span className="flex items-center gap-1.5 text-[11px]">
      {updatedAt && (
        <span className="text-muted-foreground/50">{formatRelativeTime(updatedAt)}</span>
      )}
      {prInfo && (
        <span className="text-muted-foreground/50">#{prInfo.number}</span>
      )}
    </span>
  );
}

function DiffStatsRight({ diffStats, menuOpen }: { diffStats: DiffStats; menuOpen: boolean }) {
  return (
    <div
      className={cn(
        "shrink-0 self-center font-mono text-[11px] transition-opacity duration-100",
        menuOpen ? "opacity-0" : "group-hover:opacity-0",
      )}
    >
      <span className="text-emerald-500">+{diffStats.additions}</span>{" "}
      <span className="text-rose-500">-{diffStats.deletions}</span>
    </div>
  );
}

/** Shows PR icon from store (real data) or from prInfo prop (prototype/mock). */
function TaskPRIcon({
  taskId,
  prInfo,
}: {
  taskId?: string;
  prInfo?: { number: number; state: string };
}) {
  const storePr = useAppStore((s) => (taskId ? (s.taskPRs.byTaskId[taskId] ?? null) : null));
  if (storePr) return <PRTaskIcon taskId={taskId!} />;
  if (!prInfo) return null;
  const state = prInfo.state.toLowerCase();
  let color = "text-muted-foreground";
  if (state === "merged") color = "text-purple-500";
  else if (state === "closed") color = "text-red-500";
  return (
    <span className={cn("inline-flex items-center shrink-0", color)}>
      <IconGitPullRequest className="h-3.5 w-3.5" />
    </span>
  );
}

function TaskItemContent({
  title,
  taskId,
  isRemoteExecutor,
  remoteExecutorType,
  remoteExecutorName,
  primarySessionId,
  isArchived,
  repositories,
  updatedAt,
  prInfo,
  reserveMenuSpace,
}: {
  title: string;
  taskId?: string;
  isRemoteExecutor?: boolean;
  remoteExecutorType?: string;
  remoteExecutorName?: string;
  primarySessionId?: string | null;
  isArchived?: boolean;
  repositories?: string[];
  updatedAt?: string;
  prInfo?: { number: number; state: string };
  reserveMenuSpace: boolean;
}) {
  return (
    <div className={cn("flex min-w-0 flex-1 flex-col gap-0.5", reserveMenuSpace && "group-hover:pr-5")}>
      <span className="flex items-center gap-1 min-w-0 text-[13px] font-medium text-foreground leading-tight">
        <ScrollOnOverflow className="min-w-0">{title}</ScrollOnOverflow>
        <TaskPRIcon taskId={taskId} prInfo={prInfo} />
        {isRemoteExecutor && (
          <RemoteCloudTooltip
            taskId={taskId ?? ""}
            sessionId={primarySessionId ?? null}
            fallbackName={remoteExecutorName ?? remoteExecutorType}
            iconClassName="h-3 w-3 text-muted-foreground/60"
          />
        )}
        {isArchived && (
          <span className="rounded px-1 py-px text-[10px] bg-amber-500/15 text-amber-500">
            Archived
          </span>
        )}
      </span>
      {repositories && repositories.length > 1 && (
        <span className="truncate text-[11px] text-muted-foreground/50">
          {repositories.join(" · ")}
        </span>
      )}
      <TaskItemStatsRow updatedAt={updatedAt} prInfo={prInfo} />
    </div>
  );
}

export const TaskItem = memo(function TaskItem({
  title,
  state,
  sessionState,
  isArchived,
  isSelected = false,
  onClick,
  diffStats,
  isRemoteExecutor,
  remoteExecutorType,
  remoteExecutorName,
  updatedAt,
  menuOpen = false,
  isDeleting,
  taskId,
  primarySessionId,
  isSubTask,
  repositories,
  prInfo,
}: TaskItemProps) {
  const effectiveMenuOpen = menuOpen || isDeleting === true;
  const isInProgress =
    state === "IN_PROGRESS" ||
    state === "SCHEDULING" ||
    sessionState === "STARTING" ||
    sessionState === "RUNNING";
  const hasDiffStats = !isInProgress && !!diffStats && (diffStats.additions > 0 || diffStats.deletions > 0);

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(e) => handleTaskItemKeyDown(e, onClick)}
      className={cn(
        "group relative flex w-full items-start gap-2 py-2 pr-3 text-left text-sm outline-none cursor-pointer",
        "transition-colors duration-75 hover:bg-foreground/[0.05]",
        isSelected && "bg-primary/10",
        isSubTask ? "pl-8" : "pl-3",
      )}
    >
      <div
        className={cn(
          "absolute left-0 top-0 bottom-0 w-[2px] transition-opacity",
          isSelected ? "bg-primary opacity-100" : "opacity-0",
        )}
      />
      {isSubTask && (
        <span className="absolute left-3.5 top-[10px] select-none text-[11px] text-muted-foreground/30">
          ↳
        </span>
      )}
      <TaskStateIcon sessionState={sessionState} state={state} isInProgress={isInProgress} />
      <TaskItemContent
        title={title}
        taskId={taskId}
        isRemoteExecutor={isRemoteExecutor}
        remoteExecutorType={remoteExecutorType}
        remoteExecutorName={remoteExecutorName}
        primarySessionId={primarySessionId}
        isArchived={isArchived}
        repositories={repositories}
        updatedAt={updatedAt}
        prInfo={prInfo}
        reserveMenuSpace={!hasDiffStats}
      />
      {hasDiffStats && <DiffStatsRight diffStats={diffStats!} menuOpen={effectiveMenuOpen} />}
      <TaskMenuButton visible={effectiveMenuOpen} />
    </div>
  );
});

function TaskMenuButton({ visible }: { visible: boolean }) {
  return (
    <div
      className={cn(
        "absolute right-1 inset-y-0 flex items-center gap-0.5 transition-opacity duration-100",
        visible ? "opacity-100" : "opacity-0 group-hover:opacity-100",
      )}
    >
      <button
        type="button"
        className={cn(
          "flex h-6 w-6 items-center justify-center rounded-md cursor-pointer",
          "text-muted-foreground hover:text-foreground hover:bg-foreground/10",
          "focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring transition-colors",
        )}
        onClick={(e) => {
          e.stopPropagation();
          e.preventDefault();
          e.currentTarget.dispatchEvent(
            new MouseEvent("contextmenu", {
              bubbles: true,
              clientX: e.clientX,
              clientY: e.clientY,
            }),
          );
        }}
        aria-label="Task actions"
      >
        <IconDots className="h-4 w-4" />
      </button>
    </div>
  );
}
