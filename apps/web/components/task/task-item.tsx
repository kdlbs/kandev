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
import { TaskTitleHoverCard } from "@/components/task/task-title-hover-card";
import type { WipQueueStatus } from "@/lib/kanban/wip-queue";
import { TaskItemComparisonUnavailable } from "./task-item-comparison-unavailable";
import { TaskMenuButton } from "./task-item-menu-button";
import { TaskItemLeadingBadges } from "./task-item-leading-badges";

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
   * carries its primary repository here; enumerating the rest is deferred,
   * since `TaskSwitcherItem.repositories` is not populated from live data.
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
): string {
  return cn(
    "group relative flex w-full items-start gap-2 py-2 pr-3 text-left text-sm outline-none cursor-pointer",
    "transition-colors duration-75 hover:bg-foreground/[0.05]",
    isSelected && "bg-primary/10",
    // When a row is both the active task and multi-selected, keep the stronger
    // active background and just add the selection ring on top.
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
          <IconCircleDashed
            aria-hidden="true"
            data-testid="task-state-background-running"
            className="h-3.5 w-3.5 shrink-0 animate-spin text-violet-500"
          />
        </span>
      </TooltipTrigger>
      <TooltipContent side="right">{t("task:backgroundWorkIsRunning")}</TooltipContent>
    </Tooltip>
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
      <IconCircleDashed
        data-testid="task-state-running"
        data-loading-phase="running"
        className="mt-[1px] h-3.5 w-3.5 shrink-0 text-yellow-500 animate-spin"
      />
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
      <IconCircleDashed
        data-testid="task-state-running"
        data-loading-phase="preparing"
        className="mt-[1px] h-3.5 w-3.5 shrink-0 animate-spin text-muted-foreground/40 [animation-duration:2s]"
      />
    );
  }
  // When the aggregate is unknown, a live turn safely falls back to the
  // established generating spinner rather than a done affordance.
  if (isInProgress) {
    return (
      <IconCircleDashed
        data-testid="task-state-running"
        data-loading-phase="running"
        className="mt-[1px] h-3.5 w-3.5 shrink-0 text-yellow-500 animate-spin"
      />
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

function DiffStatsRight({ diffStats, menuOpen }: { diffStats: DiffStats; menuOpen: boolean }) {
  return (
    <div
      data-testid="sidebar-task-diff-stats"
      className={cn(
        "mobile-task-diff-stats shrink-0 self-center font-mono text-[11px] transition-opacity duration-100",
        menuOpen
          ? "opacity-0"
          : "[@media(hover:hover)]:group-hover:opacity-0 group-focus-within/actions:opacity-0",
      )}
    >
      <span className="text-emerald-500">+{diffStats.additions}</span>{" "}
      <span className="text-rose-500">-{diffStats.deletions}</span>
    </div>
  );
}

function TaskItemTitle({ taskId, title }: { taskId?: string; title: string }) {
  // w-full: ScrollOnOverflow's root is inline-block, so once it sits inside
  // the title-preview trigger's <button> (task-title-hover-card.tsx) rather
  // than being the flex row's direct child, shrink-to-fit sizing lets it grow
  // past the button's flex-shrunk width instead of clipping to it — losing
  // the overflow the hover-scroll marquee depends on.
  const content = <ScrollOnOverflow className="min-w-0 w-full">{title}</ScrollOnOverflow>;
  if (!taskId) return content;
  return (
    <TaskTitleHoverCard taskId={taskId} title={title} side="right" align="start">
      {content}
    </TaskTitleHoverCard>
  );
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
}) {
  const { t } = useTranslation();
  return (
    <div className="flex min-w-0 flex-1 flex-col gap-0.5">
      <span className="flex items-center gap-1 min-w-0 text-[13px] font-medium text-foreground leading-tight">
        <TaskItemTitle taskId={taskId} title={title} />
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
          <span className="rounded px-1 py-px text-[10px] bg-amber-500/15 text-amber-500">
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

function TaskItemActions({
  archiveConfirmation,
  diffStats,
  hasDiffStats,
  menuOpen,
  effectiveMenuOpen,
}: {
  archiveConfirmation?: ReactNode;
  diffStats?: DiffStats;
  hasDiffStats: boolean;
  menuOpen: boolean;
  effectiveMenuOpen: boolean;
}) {
  if (archiveConfirmation) {
    return (
      <div className="min-w-0 basis-full flex items-center justify-end">{archiveConfirmation}</div>
    );
  }
  if (hasDiffStats) {
    return (
      <div className="mobile-task-actions-with-stats group/actions relative shrink-0 self-center flex items-center">
        <DiffStatsRight diffStats={diffStats!} menuOpen={effectiveMenuOpen} />
        <div className="mobile-task-actions-slot absolute inset-0 flex items-center justify-end">
          <TaskMenuButton visible={effectiveMenuOpen} expanded={menuOpen} />
        </div>
      </div>
    );
  }
  return <TaskMenuButton visible={effectiveMenuOpen} expanded={menuOpen} rowFocus />;
}

export const TaskItem = memo(function TaskItem(props: TaskItemProps) {
  const isSelected = props.isSelected ?? false;
  const isMultiSelected = props.isMultiSelected ?? false;
  const menuOpen = props.menuOpen ?? false;
  const effectiveMenuOpen = menuOpen || props.isDeleting === true;
  const hasDiffStats =
    !!props.diffStats && (props.diffStats.additions > 0 || props.diffStats.deletions > 0);
  const taskColor = useTaskColor(props.taskId);
  const indent = computeRowIndent(resolveRowDepth(props.depth, props.isSubTask));

  return (
    <div
      role="button"
      tabIndex={0}
      data-testid="sidebar-task-item"
      data-task-row-id={props.taskId}
      {...taskItemStateAttrs(isSelected, isMultiSelected)}
      onClick={taskItemRowClick(props.onSelect, props.onClick)}
      onKeyDown={(e) => handleTaskItemKeyDown(e, props.onSelect, props.onClick)}
      style={indent.depth > 0 ? { paddingLeft: indent.paddingLeftPx } : undefined}
      className={cn(
        taskItemRowClassName(isSelected, isMultiSelected, indent.depth === 0),
        props.archiveConfirmation && "flex-wrap",
      )}
    >
      <SelectionBar isSelected={isSelected} color={taskColor} />
      <RowConnector depth={indent.depth} leftPx={indent.connectorLeftPx} />
      <TaskStateIcon
        sessionState={props.sessionState}
        state={props.state}
        foregroundActivity={props.foregroundActivity}
        isInProgress={computeIsInProgress(props.state, props.sessionState)}
        hasPendingClarification={props.hasPendingClarification}
        hasPendingPermission={props.hasPendingPermission}
        interrupted={props.interrupted}
        isOnLastWorkflowStep={props.isOnLastWorkflowStep ?? false}
      />
      <TaskItemContent
        title={props.title}
        autopilot={props.autopilot}
        taskId={props.taskId}
        workflowStepId={props.workflowStepId}
        isRemoteExecutor={props.isRemoteExecutor}
        remoteExecutorType={props.remoteExecutorType}
        remoteExecutorName={props.remoteExecutorName}
        primarySessionId={props.primarySessionId}
        isArchived={props.isArchived}
        isPinned={props.isPinned}
        repositoryPath={props.repositoryPath}
        showRepository={props.showRepository ?? true}
        updatedAt={props.updatedAt}
        lastActivityAt={props.lastActivityAt}
        showActivityTime={props.showActivityTime ?? false}
        prInfo={props.prInfo}
        queuedCount={props.queuedCount}
        wipQueue={props.wipQueue}
        issueInfo={props.issueInfo}
        agentErrorMessage={props.agentErrorMessage}
        comparisonUnavailable={props.comparisonUnavailable}
      />
      <TaskItemActions
        archiveConfirmation={props.archiveConfirmation}
        diffStats={props.diffStats}
        hasDiffStats={hasDiffStats}
        menuOpen={menuOpen}
        effectiveMenuOpen={effectiveMenuOpen}
      />
      {!!props.subtaskCount && props.subtaskCount > 0 && !!props.onToggleSubtasks && (
        <SubtaskToggle
          taskId={props.taskId}
          count={props.subtaskCount}
          collapsed={!!props.subtasksCollapsed}
          onToggle={props.onToggleSubtasks}
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
  if (color) {
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
  return (
    <div
      className={cn(
        "absolute left-0 top-0 bottom-0 w-[2px] bg-primary transition-opacity",
        isSelected ? "opacity-100" : "opacity-0",
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
