"use client";

import { memo, type ReactNode } from "react";
import {
  IconChevronDown,
  IconCircleCheck,
  IconCircleDashed,
  IconMessageQuestion,
  IconProgressCheck,
  IconShieldQuestion,
} from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { computeRowIndent, resolveRowDepth } from "@/lib/sidebar/row-indent";
import { TaskItemStatsRow } from "./task-item-stats-row";
import { useTaskColor } from "@/hooks/use-task-color";
import { TASK_COLOR_BAR_CLASS, type TaskColor } from "@/lib/task-colors";
import type { ForegroundActivity, TaskState, TaskSessionState } from "@/lib/types/http";
import {
  InterruptedTaskIcon,
  isTerminalInterruptedState,
  shouldUseQuestionTaskIcon,
  shouldUsePermissionTaskIcon,
} from "@/lib/ui/state-icons";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { RemoteCloudTooltip } from "./remote-cloud-tooltip";
import { TaskRowMetadata } from "./task-row-plugin-slots";
import { classifyTask } from "./task-classify";
import { ScrollOnOverflow } from "@kandev/ui/scroll-on-overflow";
import { useTranslation } from "react-i18next";
import { TaskItemComparisonUnavailable } from "./task-item-comparison-unavailable";
import type { WipQueueStatus } from "@/lib/kanban/wip-queue";
import { TaskItemLeadingBadges } from "./task-item-leading-badges";
import {
  resolveTaskRowPresentation,
  type ResolvedTaskRowPresentation,
} from "./task-row-presentation";
import { TaskItemTrailing } from "./task-item-trailing";
import { CompositorSpin } from "@kandev/ui/compositor-spin";

type DiffStats = {
  additions: number;
  deletions: number;
};

type TaskItemProps = {
  title: string;
  autopilot?: boolean;
  state?: TaskState;
  sessionState?: TaskSessionState;
  /**
   * Task-level most-active-wins busy aggregate (ADR-0049) carried on the task
   * record. Drives the sidebar row's generating/background tiers so it agrees
   * with the board card and open-task header instead of re-deriving from a
   * single session's substate.
   */
  foregroundActivity?: ForegroundActivity | null;
  isArchived?: boolean;
  isSelected?: boolean;
  /** Whether this row is part of an active multi-selection (distinct from the active-task highlight). */
  isMultiSelected?: boolean;
  onClick?: () => void;
  /**
   * Modifier-aware activation handler. When provided, both mouse clicks and
   * keyboard Enter/Space delegate here (cmd/shift/plain dispatch lives in the
   * parent); `onClick` is the fallback when no selection handler is wired.
   */
  onSelect?: (e: React.MouseEvent | React.KeyboardEvent) => void;
  diffStats?: DiffStats;
  comparisonUnavailable?: boolean;
  isRemoteExecutor?: boolean;
  remoteExecutorType?: string;
  remoteExecutorName?: string;
  updatedAt?: string;
  lastActivityAt?: string;
  showActivityTime?: boolean;
  menuOpen?: boolean;
  archiveConfirmation?: ReactNode;
  isDeleting?: boolean;
  taskId?: string;
  /** Drives the `task-row-metadata` plugin slot's `workflowStepId`. */
  workflowStepId?: string | null;
  primarySessionId?: string | null;
  hasPendingClarification?: boolean;
  hasPendingPermission?: boolean;
  /** True when the task's session was mid-turn when the backend died. */
  interrupted?: boolean;
  parentTaskTitle?: string;
  isSubTask?: boolean;
  /** Whether the task is currently on the final ordered step of its workflow. */
  isOnLastWorkflowStep?: boolean;
  /**
   * Nesting depth in the sidebar tree (0 = root). Drives left indentation so
   * arbitrarily deep subtask trees read as a hierarchy. Falls back to
   * `isSubTask` (depth 1) when omitted.
   */
  depth?: number;
  /** Number of subtasks under this parent task. Only set for parent rows. */
  subtaskCount?: number;
  /** Whether the subtasks of this parent are currently hidden. */
  subtasksCollapsed?: boolean;
  /** Toggles subtask visibility when the chevron is clicked. */
  onToggleSubtasks?: () => void;
  /**
   * The task's repository as a stable slug (or name). A multi-repository task
   * carries its primary repository here for compatibility. The complete
   * ordered repository combination is projected through
   * `TaskSwitcherItem.repositories` for sidebar grouping.
   */
  repositoryPath?: string;
  /**
   * Whether this row should name its repository. False when the list is grouped
   * by repository, where the section header already says it.
   */
  showRepository?: boolean;
  prInfo?: { number: number; state: string; aggregateState?: string };
  /** Number of prompts currently en-queued for this task (mail badge). */
  queuedCount?: number;
  /** Destination-resident WIP queue status, separate from queued prompts. */
  wipQueue?: WipQueueStatus;
  issueInfo?: { url: string; number: number };
  isPinned?: boolean;
  agentErrorMessage?: string | null;
  taskRowPresentation?: import("@/lib/state/slices/ui/sidebar-task-row-presentation").SidebarTaskRowPresentation;
};

// Delegates to the shared classifier in task-switcher so the sidebar bucket
// and the per-task running spinner always agree. A task whose workflow state
// is REVIEW or COMPLETED must not render as "running" when its session
// transiently cycles through STARTING/RUNNING (e.g. during an agent auto-
// resume after a backend restart).
function computeIsInProgress(state?: TaskState, sessionState?: TaskSessionState): boolean {
  return classifyTask(sessionState, state) === "in_progress";
}

function computeIsPreparing(state?: TaskState, sessionState?: TaskSessionState): boolean {
  if (state === "SCHEDULING") return true;
  return sessionState === "STARTING" && classifyTask(sessionState, state) !== "review";
}

function handleTaskItemKeyDown(
  e: React.KeyboardEvent<HTMLDivElement>,
  onSelect: ((e: React.KeyboardEvent) => void) | undefined,
  onClick: (() => void) | undefined,
): void {
  if (e.key !== "Enter" && e.key !== " ") return;
  e.preventDefault();
  // Keyboard activation mirrors mouse: when a selection-aware handler is wired,
  // Enter/Space toggles/extends the selection just like a click would.
  if (onSelect) onSelect(e);
  else onClick?.();
}

/** State attributes for the row: active-task (`aria-current`) + multi-selected. */
function taskItemStateAttrs(isSelected: boolean, isMultiSelected: boolean) {
  return {
    "data-active": isSelected ? "true" : "false",
    "data-multiselected": isMultiSelected ? "true" : undefined,
    "aria-current": isSelected ? ("true" as const) : undefined,
    // Surface the multi-selected state to assistive tech.
    "aria-selected": isMultiSelected ? true : undefined,
  };
}

function taskItemRowClassName(
  isSelected: boolean,
  isMultiSelected: boolean,
  isRoot: boolean,
  hasDetails: boolean,
): string {
  const rowSurfaceClass = isSelected
    ? "border-y border-primary/50 bg-primary/15 hover:bg-primary/20"
    : "hover:bg-foreground/[0.05]";

  return cn(
    "group relative flex w-full gap-2 py-2 pr-3 text-left text-sm outline-none cursor-pointer",
    hasDetails ? "items-start" : "items-center",
    "transition-colors duration-75",
    rowSurfaceClass,
    // When a row is multi-selected, keep the active background when applicable
    // and add only the existing selection ring for the multi-selection state.
    isMultiSelected && !isSelected && "bg-primary/5",
    isMultiSelected && "ring-1 ring-inset ring-primary/40",
    isRoot && "pl-3",
  );
}

/** Mouse uses the modifier-aware `onSelect`; falls back to the plain `onClick`. */
function taskItemRowClick(
  onSelect: ((e: React.MouseEvent | React.KeyboardEvent) => void) | undefined,
  onClick: (() => void) | undefined,
): (e: React.MouseEvent) => void {
  return (e) => (onSelect ? onSelect(e) : onClick?.());
}

function BackgroundWorkTaskIcon() {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          aria-label={t("task:backgroundWorkIsRunning")}
          tabIndex={0}
          className="mt-[1px] flex shrink-0 rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:ring-offset-1"
        >
          <CompositorSpin
            aria-hidden="true"
            data-testid="task-state-background-running"
            className="h-3.5 w-3.5 shrink-0 text-violet-500"
          >
            <IconCircleDashed className="size-full" />
          </CompositorSpin>
        </span>
      </TooltipTrigger>
      <TooltipContent side="right">{t("task:backgroundWorkIsRunning")}</TooltipContent>
    </Tooltip>
  );
}

function TaskRunningIcon({
  phase,
  className,
}: {
  phase: "running" | "preparing";
  className: string;
}) {
  return (
    <CompositorSpin
      data-testid="task-state-running"
      data-loading-phase={phase}
      className={className}
    >
      <IconCircleDashed className="size-full" />
    </CompositorSpin>
  );
}

function TaskStateIcon({
  sessionState,
  state,
  foregroundActivity,
  isInProgress,
  hasPendingClarification,
  hasPendingPermission,
  isOnLastWorkflowStep,
  interrupted,
}: {
  sessionState?: TaskSessionState;
  state?: TaskState;
  foregroundActivity?: ForegroundActivity | null;
  isInProgress: boolean;
  hasPendingClarification?: boolean;
  hasPendingPermission?: boolean;
  isOnLastWorkflowStep?: boolean;
  /** True when the task's session was mid-turn when the backend died. */
  interrupted?: boolean;
}) {
  if (shouldUsePermissionTaskIcon(hasPendingPermission)) {
    return (
      <IconShieldQuestion
        data-testid="task-state-pending-permission"
        className="mt-[1px] h-3.5 w-3.5 shrink-0 text-amber-500"
      />
    );
  }
  if (hasPendingClarification) {
    return (
      <IconMessageQuestion
        data-testid="task-state-waiting-for-input"
        className="mt-[1px] h-3.5 w-3.5 shrink-0 text-yellow-500"
      />
    );
  }
  if (foregroundActivity === "generating") {
    return (
      <TaskRunningIcon phase="running" className="mt-[1px] h-3.5 w-3.5 shrink-0 text-yellow-500" />
    );
  }
  if (foregroundActivity === "background") {
    return <BackgroundWorkTaskIcon />;
  }
  if (shouldUseQuestionTaskIcon(state)) {
    return (
      <IconMessageQuestion
        data-testid="task-state-waiting-for-input"
        className="mt-[1px] h-3.5 w-3.5 shrink-0 text-yellow-500"
      />
    );
  }
  if (computeIsPreparing(state, sessionState)) {
    return (
      <TaskRunningIcon
        phase="preparing"
        className="mt-[1px] h-3.5 w-3.5 shrink-0 text-muted-foreground/40 [animation-duration:2s]"
      />
    );
  }
  // When the aggregate is unknown, a live turn safely falls back to the
  // established generating spinner rather than a done affordance.
  if (isInProgress) {
    return (
      <TaskRunningIcon phase="running" className="mt-[1px] h-3.5 w-3.5 shrink-0 text-yellow-500" />
    );
  }
  // The task's session was mid-turn when the backend died (startup
  // reconciliation marker). Show the red interruption icon instead of the
  // idle/done affordances; every active/pending state above already won, and
  // terminal states keep their own icons.
  if (interrupted && !isTerminalInterruptedState(state, sessionState)) {
    return <InterruptedTaskIcon className="mt-[1px] h-3.5 w-3.5 shrink-0" />;
  }
  if (classifyTask(sessionState, state) === "review") {
    if (isOnLastWorkflowStep) {
      return (
        <IconCircleCheck
          data-testid="task-state-workflow-complete"
          className="mt-[1px] h-3.5 w-3.5 shrink-0 text-green-500"
        />
      );
    }
    return (
      <IconProgressCheck
        data-testid="task-state-turn-finished"
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

function TaskItemTitle({ title }: { title: string }) {
  return <ScrollOnOverflow className="min-w-0">{title}</ScrollOnOverflow>;
}

function TaskItemContent({
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
  prInfo,
  queuedCount,
  wipQueue,
  issueInfo,
  agentErrorMessage,
  comparisonUnavailable,
  resolvedTaskRow,
  relativeTime,
}: {
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
  prInfo?: { number: number; state: string; aggregateState?: string };
  queuedCount?: number;
  wipQueue?: WipQueueStatus;
  issueInfo?: { url: string; number: number };
  agentErrorMessage?: string | null;
  comparisonUnavailable?: boolean;
  resolvedTaskRow: ResolvedTaskRowPresentation;
  relativeTime?: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex min-w-0 flex-1 flex-col gap-0.5">
      <span className="flex items-center gap-1 min-w-0 text-[13px] font-medium text-foreground leading-tight">
        <TaskItemTitle title={title} />
        <TaskItemLeadingBadges
          autopilot={autopilot}
          isPinned={isPinned}
          taskId={taskId}
          prInfo={prInfo}
          showChangeRequestStatus={resolvedTaskRow.trailing !== "change_request_status"}
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
          <span className="rounded px-1 py-px text-[10px] bg-amber-500/15 text-amber-500">
            {t("task:filterDimensionArchived")}
          </span>
        )}
      </span>
      {taskId && resolvedTaskRow.detailsEnabled && (
        <TaskRowMetadata
          taskId={taskId}
          workflowStepId={workflowStepId ?? null}
          surface="sidebar"
        />
      )}
      {resolvedTaskRow.detailsEnabled && (
        <TaskItemStatsRow
          updatedAt={relativeTime}
          repositoryLabel={resolvedTaskRow.showRepository ? repositoryPath : undefined}
          prInfo={resolvedTaskRow.showPullRequestNumber ? prInfo : undefined}
          primarySessionId={primarySessionId}
          queuedCount={queuedCount}
          wipQueue={wipQueue}
          detailOrder={resolvedTaskRow.detailOrder}
          showRelativeTime={resolvedTaskRow.showRelativeTime}
          showRepository={resolvedTaskRow.showRepository}
          showPullRequestNumber={resolvedTaskRow.showPullRequestNumber}
        />
      )}
    </div>
  );
}

function TaskItemActions({
  archiveConfirmation,
  resolvedTaskRow,
  diffStats,
  menuOpen,
  effectiveMenuOpen,
  relativeTime,
  taskId,
  prInfo,
}: {
  archiveConfirmation?: ReactNode;
  resolvedTaskRow: ResolvedTaskRowPresentation;
  diffStats?: DiffStats;
  menuOpen: boolean;
  effectiveMenuOpen: boolean;
  relativeTime?: string;
  taskId?: string;
  prInfo?: { number: number; state: string; aggregateState?: string };
}) {
  if (archiveConfirmation) {
    return (
      <div className="min-w-0 basis-full flex items-center justify-end">{archiveConfirmation}</div>
    );
  }
  return (
    <TaskItemTrailing
      trailing={resolvedTaskRow.trailing}
      diffStats={diffStats}
      menuOpen={menuOpen}
      effectiveMenuOpen={effectiveMenuOpen}
      relativeTime={relativeTime}
      taskId={taskId}
      prInfo={prInfo}
    />
  );
}

// eslint-disable-next-line max-lines-per-function
export const TaskItem = memo(function TaskItem({
  title,
  autopilot,
  state,
  sessionState,
  foregroundActivity,
  isArchived,
  isSelected = false,
  isMultiSelected = false,
  onClick,
  onSelect,
  diffStats,
  archiveConfirmation,
  comparisonUnavailable,
  isRemoteExecutor,
  remoteExecutorType,
  remoteExecutorName,
  updatedAt,
  lastActivityAt,
  showActivityTime = false,
  menuOpen = false,
  isDeleting,
  taskId,
  workflowStepId,
  primarySessionId,
  hasPendingClarification,
  hasPendingPermission,
  interrupted,
  isSubTask,
  depth,
  subtaskCount,
  subtasksCollapsed,
  onToggleSubtasks,
  repositoryPath,
  showRepository = true,
  prInfo,
  queuedCount,
  wipQueue,
  issueInfo,
  isPinned,
  agentErrorMessage,
  isOnLastWorkflowStep = false,
  taskRowPresentation,
}: TaskItemProps) {
  const effectiveMenuOpen = menuOpen || isDeleting === true;
  const resolvedTaskRow = resolveTaskRowPresentation(taskRowPresentation, { showRepository });
  const relativeTime = showActivityTime ? (lastActivityAt ?? updatedAt) : updatedAt;
  const taskColor = useTaskColor(taskId);
  const indent = computeRowIndent(resolveRowDepth(depth, isSubTask));

  return (
    <div
      role="button"
      tabIndex={0}
      data-testid="sidebar-task-item"
      data-task-row-id={taskId}
      {...taskItemStateAttrs(isSelected, isMultiSelected)}
      onClick={taskItemRowClick(onSelect, onClick)}
      onKeyDown={(e) => handleTaskItemKeyDown(e, onSelect, onClick)}
      style={indent.depth > 0 ? { paddingLeft: indent.paddingLeftPx } : undefined}
      className={cn(
        taskItemRowClassName(
          isSelected,
          isMultiSelected,
          indent.depth === 0,
          resolvedTaskRow.detailsEnabled,
        ),
        archiveConfirmation && "flex-wrap",
      )}
    >
      <SelectionBar isSelected={isSelected} color={taskColor} />
      <RowConnector depth={indent.depth} leftPx={indent.connectorLeftPx} />
      <TaskStateIcon
        sessionState={sessionState}
        state={state}
        foregroundActivity={foregroundActivity}
        isInProgress={computeIsInProgress(state, sessionState)}
        hasPendingClarification={hasPendingClarification}
        hasPendingPermission={hasPendingPermission}
        interrupted={interrupted}
        isOnLastWorkflowStep={isOnLastWorkflowStep}
      />
      <TaskItemContent
        title={title}
        autopilot={autopilot}
        taskId={taskId}
        workflowStepId={workflowStepId}
        isRemoteExecutor={isRemoteExecutor}
        remoteExecutorType={remoteExecutorType}
        remoteExecutorName={remoteExecutorName}
        primarySessionId={primarySessionId}
        isArchived={isArchived}
        isPinned={isPinned}
        repositoryPath={repositoryPath}
        prInfo={prInfo}
        queuedCount={queuedCount}
        wipQueue={wipQueue}
        issueInfo={issueInfo}
        agentErrorMessage={agentErrorMessage}
        comparisonUnavailable={comparisonUnavailable}
        resolvedTaskRow={resolvedTaskRow}
        relativeTime={relativeTime}
      />
      <TaskItemActions
        archiveConfirmation={archiveConfirmation}
        resolvedTaskRow={resolvedTaskRow}
        diffStats={diffStats}
        menuOpen={menuOpen}
        effectiveMenuOpen={effectiveMenuOpen}
        relativeTime={relativeTime}
        taskId={taskId}
        prInfo={prInfo}
      />
      {!!subtaskCount && subtaskCount > 0 && !!onToggleSubtasks && (
        <SubtaskToggle
          taskId={taskId}
          count={subtaskCount}
          collapsed={!!subtasksCollapsed}
          onToggle={onToggleSubtasks}
        />
      )}
    </div>
  );
});

// Nested-subtask connector glyph. Renders nothing at the top level (depth 0).
function RowConnector({ depth, leftPx }: { depth: number; leftPx: number }) {
  if (depth === 0) return null;
  return (
    <span
      style={{ left: leftPx }}
      className="absolute top-[10px] select-none text-[11px] text-muted-foreground/30"
    >
      ↳
    </span>
  );
}

function SelectionBar({ isSelected, color }: { isSelected: boolean; color: TaskColor | null }) {
  if (!color) return null;

  return (
    <div
      className={cn(
        "absolute left-0 top-0 bottom-0 w-[3px] transition-opacity",
        TASK_COLOR_BAR_CLASS[color],
        isSelected ? "opacity-100" : "opacity-60",
      )}
    />
  );
}

function SubtaskToggle({
  taskId,
  count,
  collapsed,
  onToggle,
}: {
  taskId?: string;
  count: number;
  collapsed: boolean;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      data-testid="sidebar-subtask-toggle"
      data-task-id={taskId}
      aria-label={collapsed ? t("task:expandSubtasks") : t("task:collapseSubtasks")}
      aria-expanded={!collapsed}
      onClick={(e) => {
        e.stopPropagation();
        onToggle();
      }}
      onKeyDown={(e) => e.stopPropagation()}
      className="self-center flex items-center gap-0.5 shrink-0 cursor-pointer text-[11px] text-muted-foreground/60 hover:text-foreground"
    >
      <IconChevronDown className={cn("h-3 w-3 transition-transform", collapsed && "-rotate-90")} />
      <span>{count}</span>
    </button>
  );
}
