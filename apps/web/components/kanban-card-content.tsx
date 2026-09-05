"use client";

import { useEffect, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { CSS, type Transform } from "@dnd-kit/utilities";
import type { DraggableAttributes, DraggableSyntheticListeners } from "@dnd-kit/core";
import { IconArrowsMaximize, IconDots, IconUsersGroup } from "@tabler/icons-react";
import { Card, CardContent } from "@kandev/ui/card";
import { Checkbox } from "@kandev/ui/checkbox";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { PRTaskIcon } from "@/components/github/pr-task-icon";
import { MRTaskIcon } from "@/components/gitlab/mr-task-icon";
import { RegisteredChangeRequestTaskIcon } from "@/components/integrations/registered-change-request-task-icon";
import {
  KanbanCardDropdownMenuItems,
  type KanbanCardMenuEntry,
} from "@/components/kanban-card-menu-items";
import { TaskCardIndicators, TaskCardTags } from "@/components/kanban-card-plugin-slots";
import {
  KanbanCardBadges,
  KanbanCardRelationship,
  RepoChipRow,
} from "@/components/kanban-card-status-strip";
import { KanbanCardPriorityIndicator } from "@/components/kanban-card-priority-indicator";
import { CardTitle } from "@/components/kanban-card-title";
import { useAppStoreApi } from "@/components/state-provider";
import { RemoteCloudTooltip } from "@/components/task/remote-cloud-tooltip";
import { useTaskPendingInput } from "@/hooks/use-task-pending-input";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import {
  getTaskStateIcon,
  shouldShowTaskRunningSpinner,
  shouldUsePermissionTaskIcon,
  shouldUseQuestionTaskIcon,
} from "@/lib/ui/state-icons";
import { cn } from "@/lib/utils";
import { needsAction } from "@/lib/utils/needs-action";
import type { RepositoryChip, Task } from "@/components/kanban-card";

const kanbanStatusDebug = createDebugLogger("kanban:task-status");

type KanbanCardActionProps = {
  task: Task;
  showMaximizeButton?: boolean;
  onOpenFullPage?: (task: Task) => void;
  menuEntries: KanbanCardMenuEntry[];
  isDeleting?: boolean;
  isArchiving?: boolean;
  menuTriggerRef?: RefObject<HTMLButtonElement | null>;
};

type DraggableCardState = {
  attributes: DraggableAttributes;
  listeners: DraggableSyntheticListeners;
  setNodeRef: (element: HTMLElement | null) => void;
  transform: Transform | null;
  isDragging: boolean;
};

export type KanbanCardShellProps = KanbanCardActionProps &
  DraggableCardState & {
    repositoryChips?: RepositoryChip[];
    isSelected?: boolean;
    isMultiSelectMode?: boolean;
    isPreviewed: boolean;
    onClick: (e: React.MouseEvent) => void;
    onCheckboxClick: (e: React.MouseEvent) => void;
  };

export function KanbanCardBody({
  task,
  repositoryChips,
  actions,
  enableTitleHover,
}: {
  task: Task;
  repositoryChips: RepositoryChip[];
  actions?: React.ReactNode;
  enableTitleHover?: boolean;
}) {
  return (
    <>
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0 flex-1">
          <RepoChipRow chips={repositoryChips} />
          <div className="flex items-center gap-1 min-w-0" data-testid="kanban-card-title-row">
            <CardTitle task={task} enableTitleHover={enableTitleHover} />
            <KanbanCardPriorityIndicator priority={task.priority} />
            <PRTaskIcon taskId={task.id} />
            <MRTaskIcon taskId={task.id} />
            <RegisteredChangeRequestTaskIcon taskId={task.id} />
            <TaskCardIndicators task={task} />
          </div>
        </div>
        {task.isRemoteExecutor && (
          <RemoteCloudTooltip
            taskId={task.id}
            sessionId={task.primarySessionId ?? null}
            executorId={task.primaryExecutorId}
            executorType={task.primaryExecutorType}
            fallbackName={task.primaryExecutorName ?? task.primaryExecutorType}
          />
        )}
        {actions}
      </div>
      {task.description && (
        <p className="text-xs text-muted-foreground mt-1 leading-tight line-clamp-1">
          {task.description}
        </p>
      )}
      <KanbanCardRelationship task={task} />
      <KanbanCardBadges task={task} />
      <TaskCardTags task={task} />
    </>
  );
}

// renderTaskStatusIcon resolves the card status icon, or null when the actions
// cluster shows none (a resting done/todo task). The backend task-level
// MOST-ACTIVE-WINS aggregate takes precedence: a
// background-running task shows the distinct background affordance — even when its
// primary session has finished and only a secondary session is still working, so
// it reads as working, not done — while any generating session keeps the spinner.
// When the aggregate is absent it falls back to the primary-session-driven spinner
// (covers STARTING/SCHEDULING before a session reads RUNNING) or the pending-input
// question icon.
export function renderTaskStatusIcon(
  task: Task,
  showRunningSpinner: boolean,
  hasPendingClarification: boolean,
  hasPendingPermission: boolean,
) {
  const showQuestionIcon = shouldUseQuestionTaskIcon(task.state, hasPendingClarification);
  const showPermissionIcon = shouldUsePermissionTaskIcon(hasPendingPermission);
  const needsMe = showQuestionIcon || showPermissionIcon;
  const showInterrupted = !!task.interrupted;
  const showAutoStartFailed = !!task.autoStartFailed;
  const hasActivity =
    task.foregroundActivity === "generating" || task.foregroundActivity === "background";
  if (!showRunningSpinner && !needsMe && !hasActivity && !showInterrupted && !showAutoStartFailed) {
    return null;
  }
  // A "needs me" prompt (pending clarification / permission) must not be masked
  // by the launch-spinner short-circuit — a mid-turn prompt can coincide with a
  // coarse running state. Live foreground activity still wins, handled inside
  // getTaskStateIcon. A failed auto-start must not be masked either: startTask
  // sets the task to SCHEDULING before the launch, so a launch failure before
  // session creation leaves a session-less SCHEDULING/IN_PROGRESS task, which
  // reads as showRunningSpinner=true — the exact shape the failure marker exists
  // to surface.
  const foregroundActivity =
    showRunningSpinner &&
    !needsMe &&
    !showAutoStartFailed &&
    task.foregroundActivity !== "background"
      ? "generating"
      : task.foregroundActivity;
  return getTaskStateIcon(task.state, "h-4 w-4", {
    hasPendingClarification,
    foregroundActivity,
    hasPendingPermission,
    interrupted: showInterrupted,
    autoStartFailed: showAutoStartFailed,
  });
}

// The board's only window into a fan-out. `activeSubagentCount` is derived from
// the live registry (never a mutable counter) and summed across a task's
// sessions, so it needs no local reconciliation: at zero there is nothing live
// and the chip is absent.
export function renderSubagentCountChip(task: Task, label: string) {
  const count = task.activeSubagentCount ?? 0;
  if (count <= 0) return null;
  return (
    <span
      data-testid="task-subagent-count"
      title={label}
      aria-label={label}
      className="flex items-center gap-0.5 text-muted-foreground font-mono text-[10px]"
    >
      <IconUsersGroup className="h-3.5 w-3.5" aria-hidden="true" />
      {count}
    </span>
  );
}

function OpenFullPageButton({
  task,
  onOpenFullPage,
}: {
  task: Task;
  onOpenFullPage: (task: Task) => void;
}) {
  const { t } = useTranslation("common");

  return (
    <button
      type="button"
      className="text-muted-foreground hover:text-foreground hover:bg-accent rounded-sm p-1 -m-1 transition-colors cursor-pointer"
      onClick={(event) => {
        event.stopPropagation();
        onOpenFullPage(task);
      }}
      onPointerDown={(event) => event.stopPropagation()}
      aria-label={t("common:openFullPage")}
      title={t("common:openFullPage")}
    >
      <IconArrowsMaximize className="h-4 w-4" />
    </button>
  );
}

function KanbanCardActions({
  task,
  showMaximizeButton,
  onOpenFullPage,
  menuEntries,
  isDeleting,
  isArchiving,
  menuTriggerRef,
}: KanbanCardActionProps) {
  const { t } = useTranslation("common");
  const [menuOpen, setMenuOpen] = useState(false);
  const [storePrimarySessionState, setStorePrimarySessionState] = useState<string | null>(null);
  const storeApi = useAppStoreApi();
  const debugEnabled = isDebug();
  const effectiveMenuOpen = menuOpen || Boolean(isDeleting) || Boolean(isArchiving);
  const pendingInput = useTaskPendingInput(task.primarySessionId, {
    taskId: task.id,
    taskPendingAction: task.taskPendingAction,
    primarySessionState: task.primarySessionState,
    primarySessionPendingAction: task.primarySessionPendingAction,
  });
  const showRunningSpinner = shouldShowTaskRunningSpinner(task.state, task.primarySessionState);
  const storeWouldShowRunningSpinner =
    storePrimarySessionState === null
      ? null
      : shouldShowTaskRunningSpinner(task.state, storePrimarySessionState);
  const hasSpinnerMismatch =
    showRunningSpinner &&
    storeWouldShowRunningSpinner === false &&
    task.primarySessionState !== storePrimarySessionState;
  const statusIcon = renderTaskStatusIcon(
    task,
    showRunningSpinner,
    pendingInput.clarification,
    pendingInput.permission,
  );
  const hasKnownSession =
    Boolean(task.primarySessionId) || Boolean(task.sessionCount && task.sessionCount > 0);

  useEffect(() => {
    if (!debugEnabled || !task.primarySessionId) {
      setStorePrimarySessionState(null);
      return;
    }

    const primarySessionId = task.primarySessionId;
    const readPrimarySessionState = () =>
      storeApi.getState().taskSessions.items[primarySessionId]?.state ?? null;
    const syncPrimarySessionState = () => {
      const nextState = readPrimarySessionState();
      setStorePrimarySessionState((current) => (current === nextState ? current : nextState));
    };

    syncPrimarySessionState();
    return storeApi.subscribe(syncPrimarySessionState);
  }, [debugEnabled, storeApi, task.primarySessionId]);

  useEffect(() => {
    if (!hasSpinnerMismatch || !debugEnabled) return;
    kanbanStatusDebug("spinner mismatch", {
      task_id: task.id,
      taskState: task.state ?? "-",
      primarySessionId: task.primarySessionId ?? "-",
      taskPrimarySessionState: task.primarySessionState ?? "-",
      storePrimarySessionState: storePrimarySessionState ?? "-",
      showSpinner: showRunningSpinner,
    });
  }, [
    debugEnabled,
    hasSpinnerMismatch,
    showRunningSpinner,
    storePrimarySessionState,
    task.id,
    task.primarySessionId,
    task.primarySessionState,
    task.state,
  ]);

  return (
    <div className="flex items-center gap-2">
      {renderSubagentCountChip(
        task,
        t("common:activeSubagents", { count: task.activeSubagentCount ?? 0 }),
      )}
      {statusIcon}
      {showMaximizeButton && onOpenFullPage && hasKnownSession && (
        <OpenFullPageButton task={task} onOpenFullPage={onOpenFullPage} />
      )}
      <KanbanCardMenu
        task={task}
        effectiveMenuOpen={effectiveMenuOpen}
        setMenuOpen={setMenuOpen}
        isDeleting={isDeleting}
        isArchiving={isArchiving}
        menuEntries={menuEntries}
        menuTriggerRef={menuTriggerRef}
      />
    </div>
  );
}

type KanbanCardMenuProps = KanbanCardActionProps & {
  effectiveMenuOpen: boolean;
  setMenuOpen: (open: boolean) => void;
};

function KanbanCardMenu(props: KanbanCardMenuProps) {
  const { t } = useTranslation();
  const { effectiveMenuOpen, setMenuOpen, isDeleting, isArchiving, menuTriggerRef } = props;
  const { menuEntries } = props;
  const isProcessing = isDeleting || isArchiving;

  return (
    <DropdownMenu
      open={effectiveMenuOpen}
      onOpenChange={(open) => {
        if (!open && isProcessing) return;
        setMenuOpen(open);
      }}
    >
      <DropdownMenuTrigger asChild>
        <button
          ref={menuTriggerRef}
          type="button"
          className="text-muted-foreground hover:text-foreground hover:bg-muted inline-flex h-11 min-h-11 w-11 min-w-11 items-center justify-center rounded-sm p-0 transition-colors cursor-pointer sm:h-auto sm:min-h-0 sm:w-auto sm:min-w-0 sm:p-1 sm:-m-1"
          onClick={(e) => e.stopPropagation()}
          onPointerDown={(e) => e.stopPropagation()}
          aria-label={t("kanban:moreOptions")}
        >
          <IconDots className="h-4 w-4" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <KanbanCardDropdownMenuItems entries={menuEntries} />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function KanbanCardCheckbox({
  taskId,
  taskTitle,
  isSelected,
  onCheckboxClick,
}: {
  taskId: string;
  taskTitle: string;
  isSelected?: boolean;
  onCheckboxClick: (e: React.MouseEvent) => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="mt-0.5 shrink-0"
      onClick={onCheckboxClick}
      onPointerDown={(e) => e.stopPropagation()}
      data-testid={`task-select-checkbox-${taskId}`}
    >
      <Checkbox
        checked={!!isSelected}
        aria-label={t("kanban:selectTask", { title: taskTitle })}
        className="cursor-pointer border-muted-foreground/50"
      />
    </div>
  );
}

function KanbanCardActionSlot({
  isMultiSelectMode,
  task,
  showMaximizeButton,
  onOpenFullPage,
  menuEntries,
  isDeleting,
  isArchiving,
  menuTriggerRef,
}: KanbanCardActionProps & { isMultiSelectMode?: boolean }) {
  if (isMultiSelectMode) return null;
  return (
    <KanbanCardActions
      task={task}
      showMaximizeButton={showMaximizeButton}
      onOpenFullPage={onOpenFullPage}
      menuEntries={menuEntries}
      isDeleting={isDeleting}
      isArchiving={isArchiving}
      menuTriggerRef={menuTriggerRef}
    />
  );
}

export function KanbanCardShell({
  task,
  repositoryChips,
  attributes,
  listeners,
  setNodeRef,
  transform,
  isDragging,
  isPreviewed,
  isSelected,
  isMultiSelectMode,
  showMaximizeButton,
  isDeleting,
  isArchiving,
  onClick,
  onCheckboxClick,
  onOpenFullPage,
  menuEntries,
  menuTriggerRef,
}: KanbanCardShellProps) {
  const showCheckbox = isMultiSelectMode || !!isSelected;
  const style = {
    transform: CSS.Translate.toString(transform),
    transition: "none",
    willChange: isDragging ? "transform" : undefined,
  };

  return (
    <Card
      size="sm"
      ref={setNodeRef}
      style={style}
      data-testid={`task-card-${task.id}`}
      data-kanban-card=""
      className={cn(
        "group max-h-48 bg-card rounded-sm data-[size=sm]:py-1 cursor-pointer mb-2 w-full py-0 relative border border-border overflow-visible shadow-none ring-0",
        "touch-none md:touch-auto",
        needsAction(task) && !isSelected && "border-l-2 border-l-amber-500",
        isDragging && "opacity-50 z-50",
        isSelected && "ring-1 ring-primary/60 border-primary/60",
        isPreviewed && !isSelected && "ring-2 ring-primary border-primary",
      )}
      onClick={onClick}
      {...(!isMultiSelectMode ? listeners : {})}
      {...(!isMultiSelectMode ? attributes : {})}
    >
      <CardContent className="px-2 py-1">
        <div className="flex items-start gap-1.5">
          {showCheckbox && (
            <KanbanCardCheckbox
              taskId={task.id}
              taskTitle={task.title}
              isSelected={isSelected}
              onCheckboxClick={onCheckboxClick}
            />
          )}
          <div className="min-w-0 flex-1">
            <KanbanCardBody
              task={task}
              repositoryChips={repositoryChips ?? []}
              enableTitleHover
              actions={
                <KanbanCardActionSlot
                  isMultiSelectMode={isMultiSelectMode}
                  task={task}
                  showMaximizeButton={showMaximizeButton}
                  onOpenFullPage={onOpenFullPage}
                  menuEntries={menuEntries}
                  isDeleting={isDeleting}
                  isArchiving={isArchiving}
                  menuTriggerRef={menuTriggerRef}
                />
              }
            />
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
