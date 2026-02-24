"use client";

import { memo, useState } from "react";
import { IconLoader2 } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { TaskState, TaskSessionState } from "@/lib/types/http";
import { TaskItemMenu } from "./task-item-menu";
import { RemoteCloudTooltip } from "./remote-cloud-tooltip";

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
  onRename?: () => void;
  onDuplicate?: () => void;
  onReview?: () => void;
  onArchive?: () => void;
  onDelete?: () => void;
  isDeleting?: boolean;
  taskId?: string;
  primarySessionId?: string | null;
};

// Helper to format relative time
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

function TaskItemMeta({
  effectiveMenuOpen,
  isRemoteExecutor,
  taskId,
  primarySessionId,
  remoteExecutorName,
  remoteExecutorType,
  isArchived,
  stepName,
  isInProgress,
  diffStats,
  updatedAt,
}: {
  effectiveMenuOpen: boolean;
  isRemoteExecutor?: boolean;
  taskId?: string;
  primarySessionId?: string | null;
  remoteExecutorName?: string;
  remoteExecutorType?: string;
  isArchived?: boolean;
  stepName?: string;
  isInProgress: boolean;
  diffStats?: DiffStats;
  updatedAt?: string;
}) {
  return (
    <div
      className={cn(
        "flex flex-col items-end gap-0.5 transition-opacity duration-100",
        effectiveMenuOpen ? "opacity-0" : "group-hover:opacity-0",
      )}
    >
      <div className="flex items-center gap-1">
        {isRemoteExecutor && (
          <RemoteCloudTooltip
            taskId={taskId ?? ""}
            sessionId={primarySessionId ?? null}
            fallbackName={remoteExecutorName ?? remoteExecutorType}
            iconClassName="h-3.5 w-3.5 text-muted-foreground/80"
          />
        )}
        <TaskItemStepBadge isArchived={isArchived} stepName={stepName} />
      </div>
      <TaskItemRightMeta isInProgress={isInProgress} diffStats={diffStats} updatedAt={updatedAt} />
    </div>
  );
}

export const TaskItem = memo(function TaskItem({
  title,
  description,
  stepName,
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
  onRename,
  onDuplicate,
  onReview,
  onArchive,
  onDelete,
  isDeleting,
  taskId,
  primarySessionId,
}: TaskItemProps) {
  const [menuOpen, setMenuOpen] = useState(false);

  const effectiveMenuOpen = menuOpen || isDeleting === true;
  const isInProgress =
    state === "IN_PROGRESS" ||
    state === "SCHEDULING" ||
    sessionState === "STARTING" ||
    sessionState === "RUNNING";
  const hasDiffStats = diffStats && (diffStats.additions > 0 || diffStats.deletions > 0);

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(e) => handleTaskItemKeyDown(e, onClick)}
      className={cn(
        "group relative flex w-full items-center gap-2 px-3 py-2 text-left text-sm outline-none cursor-pointer",
        "transition-colors duration-75",
        "hover:bg-foreground/[0.05]",
        isSelected && "bg-primary/10",
      )}
    >
      {/* Selection indicator */}
      <div
        className={cn(
          "absolute left-0 top-0 bottom-0 w-[2px] transition-opacity",
          isSelected ? "bg-primary opacity-100" : "opacity-0",
        )}
      />

      {/* Content */}
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="line-clamp-1 min-w-0 text-[13px] font-medium text-foreground">
          {title}
        </span>
        {description && (
          <span className="text-[11px] text-muted-foreground/60 truncate">{description}</span>
        )}
      </div>

      {/* Right side: step name + meta, or action buttons on hover */}
      <div className="relative flex items-center shrink-0">
        <TaskItemMeta
          effectiveMenuOpen={effectiveMenuOpen}
          isRemoteExecutor={isRemoteExecutor}
          taskId={taskId}
          primarySessionId={primarySessionId}
          remoteExecutorName={remoteExecutorName}
          remoteExecutorType={remoteExecutorType}
          isArchived={isArchived}
          stepName={stepName}
          isInProgress={isInProgress}
          diffStats={hasDiffStats ? diffStats : undefined}
          updatedAt={updatedAt}
        />

        <div
          className={cn(
            "absolute right-0 flex items-center gap-0.5",
            "transition-opacity duration-100",
            effectiveMenuOpen ? "opacity-100" : "opacity-0 group-hover:opacity-100",
          )}
        >
          <TaskItemMenu
            open={effectiveMenuOpen}
            onOpenChange={(open) => {
              if (!open && isDeleting) return;
              setMenuOpen(open);
            }}
            onRename={onRename}
            onDuplicate={onDuplicate}
            onReview={onReview}
            onArchive={onArchive}
            onDelete={onDelete}
            isDeleting={isDeleting}
          />
        </div>
      </div>
    </div>
  );
});

/** Step badge or "Archived" badge */
function TaskItemStepBadge({ isArchived, stepName }: { isArchived?: boolean; stepName?: string }) {
  if (isArchived) {
    return (
      <span className="text-[10px] text-muted-foreground/70 bg-amber-500/15 text-amber-500 px-1.5 py-px rounded-[6px]">
        Archived
      </span>
    );
  }
  if (stepName) {
    return (
      <span className="text-[10px] text-muted-foreground/70 bg-foreground/[0.06] px-1.5 py-px rounded-[6px]">
        {stepName}
      </span>
    );
  }
  return null;
}

/** Right-side meta: spinner, diff stats, or relative time */
function TaskItemRightMeta({
  isInProgress,
  diffStats,
  updatedAt,
}: {
  isInProgress: boolean;
  diffStats?: DiffStats;
  updatedAt?: string;
}) {
  if (isInProgress) {
    return <IconLoader2 className="h-3.5 w-3.5 text-blue-500 animate-spin" />;
  }
  if (diffStats) {
    return (
      <span className="text-[11px] font-mono text-muted-foreground">
        <span className="text-emerald-500">+{diffStats.additions}</span>{" "}
        <span className="text-rose-500">-{diffStats.deletions}</span>
      </span>
    );
  }
  if (updatedAt) {
    return (
      <span className="text-[11px] text-muted-foreground/60">{formatRelativeTime(updatedAt)}</span>
    );
  }
  return null;
}
