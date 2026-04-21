"use client";

import { memo } from "react";
import {
  IconCircleCheck,
  IconCircleDashed,
  IconDots,
  IconGitPullRequest,
} from "@tabler/icons-react";
import { PRTaskIcon } from "@/components/github/pr-task-icon";
import { IssueTaskIcon } from "@/components/github/issue-task-icon";
import { useAppStore } from "@/components/state-provider";
import { cn } from "@/lib/utils";
import { DEBUG_UI } from "@/lib/config";
import type { TaskState, TaskSessionState } from "@/lib/types/http";
import type { SessionPollMode } from "@/lib/state/slices/session-runtime/types";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { RemoteCloudTooltip } from "./remote-cloud-tooltip";
import { classifyTask } from "./task-classify";
import { ScrollOnOverflow } from "@kandev/ui/scroll-on-overflow";

type DiffStats = {
  additions: number;
  deletions: number;
};

type TaskItemProps = {
  title: string;
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
  issueInfo?: { url: string; number: number };
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

// Delegates to the shared classifier in task-switcher so the sidebar bucket
// and the per-task running spinner always agree. A task whose workflow state
// is REVIEW or COMPLETED must not render as "running" when its session
// transiently cycles through STARTING/RUNNING (e.g. during an agent auto-
// resume after a backend restart).
function computeIsInProgress(state?: TaskState, sessionState?: TaskSessionState): boolean {
  return classifyTask(sessionState, state) === "in_progress";
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
    return (
      <IconCircleDashed
        data-testid="task-state-running"
        className="mt-[1px] h-3.5 w-3.5 shrink-0 text-yellow-500 animate-spin"
      />
    );
  }
  if (classifyTask(sessionState, state) === "review") {
    return (
      <IconCircleCheck
        data-testid="task-state-review"
        className="mt-[1px] h-3.5 w-3.5 shrink-0 text-green-500"
      />
    );
  }
  return (
    <IconCircleDashed
      data-testid="task-state-backlog"
      className="mt-[1px] h-3.5 w-3.5 shrink-0 text-muted-foreground/40"
    />
  );
}

const POLL_MODE_CONFIG: Record<SessionPollMode, { letter: string; color: string; label: string }> =
  {
    fast: { letter: "F", color: "text-emerald-500", label: "focused, 2s polling" },
    slow: { letter: "S", color: "text-yellow-500", label: "subscribed, 30s polling" },
    paused: { letter: "P", color: "text-muted-foreground/40", label: "no subscribers" },
  };

function TaskItemStatsRow({
  updatedAt,
  prInfo,
  primarySessionId,
}: {
  updatedAt?: string;
  prInfo?: { number: number; state: string };
  primarySessionId?: string | null;
}) {
  const pollMode = useAppStore((s) =>
    DEBUG_UI && primarySessionId ? (s.sessionPollMode.bySessionId[primarySessionId] ?? null) : null,
  );

  if (!updatedAt && !prInfo && !pollMode) return null;

  const modeConfig = pollMode ? POLL_MODE_CONFIG[pollMode] : null;

  return (
    <span className="flex items-center gap-1.5 text-[11px]">
      {updatedAt && (
        <span className="text-muted-foreground/50">{formatRelativeTime(updatedAt)}</span>
      )}
      {prInfo && <span className="text-muted-foreground/50">#{prInfo.number}</span>}
      {modeConfig && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className={cn("font-mono text-[10px] font-semibold", modeConfig.color)}>
              {modeConfig.letter}
            </span>
          </TooltipTrigger>
          <TooltipContent side="right">
            Git poll: {pollMode} ({modeConfig.label})
          </TooltipContent>
        </Tooltip>
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
  issueInfo,
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
  issueInfo?: { url: string; number: number };
  reserveMenuSpace: boolean;
}) {
  return (
    <div
      className={cn("flex min-w-0 flex-1 flex-col gap-0.5", reserveMenuSpace && "group-hover:pr-5")}
    >
      <span className="flex items-center gap-1 min-w-0 text-[13px] font-medium text-foreground leading-tight">
        <ScrollOnOverflow className="min-w-0">{title}</ScrollOnOverflow>
        <TaskPRIcon taskId={taskId} prInfo={prInfo} />
        {issueInfo && <IssueTaskIcon issueInfo={issueInfo} />}
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
      <TaskItemStatsRow updatedAt={updatedAt} prInfo={prInfo} primarySessionId={primarySessionId} />
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
  issueInfo,
}: TaskItemProps) {
  const effectiveMenuOpen = menuOpen || isDeleting === true;
  const isInProgress = computeIsInProgress(state, sessionState);
  const hasDiffStats = !!diffStats && (diffStats.additions > 0 || diffStats.deletions > 0);

  return (
    <div
      role="button"
      tabIndex={0}
      data-testid="sidebar-task-item"
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
        issueInfo={issueInfo}
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
