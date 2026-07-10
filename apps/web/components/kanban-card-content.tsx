"use client";

import { useEffect, useState } from "react";
import { CSS, type Transform } from "@dnd-kit/utilities";
import type { DraggableAttributes, DraggableSyntheticListeners } from "@dnd-kit/core";
import {
  IconAlertCircle,
  IconArrowsMaximize,
  IconDots,
  IconLoader2,
  IconSubtask,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent } from "@kandev/ui/card";
import { Checkbox } from "@kandev/ui/checkbox";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { PRTaskIcon } from "@/components/github/pr-task-icon";
import {
  KanbanCardDropdownMenuItems,
  type KanbanCardMenuEntry,
} from "@/components/kanban-card-menu-items";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { RemoteCloudTooltip } from "@/components/task/remote-cloud-tooltip";
import { useTaskPendingClarification } from "@/hooks/use-task-pending-clarification";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import {
  getTaskStateIcon,
  shouldShowTaskRunningSpinner,
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

const REPO_CHIPS_VISIBLE = 2;

function RepoChip({ chip }: { chip: RepositoryChip }) {
  const badge = (
    <span
      title={chip.path}
      className="shrink-0 rounded-sm bg-muted/60 px-1 py-px text-[9px] font-medium text-muted-foreground leading-tight max-w-[8rem] truncate"
    >
      {chip.label}
    </span>
  );
  if (!chip.path) return badge;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{badge}</TooltipTrigger>
      <TooltipContent side="top" align="start">
        <span className="max-w-[22rem] break-all text-xs">{chip.path}</span>
      </TooltipContent>
    </Tooltip>
  );
}

function OverflowRepoTooltip({ chips }: { chips: RepositoryChip[] }) {
  return (
    <div className="flex max-w-[24rem] flex-col gap-1 text-xs">
      {chips.map((chip) => (
        <div key={`${chip.label}:${chip.path ?? ""}`} className="min-w-0">
          <div className="font-medium">{chip.label}</div>
          {chip.path && <div className="break-all text-muted-foreground">{chip.path}</div>}
        </div>
      ))}
    </div>
  );
}

function RepoChipRow({ chips }: { chips: RepositoryChip[] }) {
  if (chips.length === 0) return null;
  const visible = chips.slice(0, REPO_CHIPS_VISIBLE);
  const overflow = chips.slice(REPO_CHIPS_VISIBLE);
  return (
    <div className="mb-1 flex items-center gap-1 min-w-0 overflow-hidden">
      {visible.map((chip) => (
        <RepoChip key={`${chip.label}:${chip.path ?? ""}`} chip={chip} />
      ))}
      {overflow.length > 0 && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="shrink-0 rounded-sm bg-muted px-1 py-px text-[9px] font-medium text-muted-foreground/80">
              +{overflow.length}
            </span>
          </TooltipTrigger>
          <TooltipContent side="top" align="start">
            <OverflowRepoTooltip chips={overflow} />
          </TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}

export function KanbanCardBody({
  task,
  repositoryChips,
  actions,
}: {
  task: Task;
  repositoryChips: RepositoryChip[];
  actions?: React.ReactNode;
}) {
  return (
    <>
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0 flex-1">
          <RepoChipRow chips={repositoryChips} />
          <div className="flex items-center gap-1 min-w-0">
            <p
              data-testid="task-card-title"
              className="text-sm font-medium leading-tight line-clamp-1 min-w-0"
            >
              {task.title}
            </p>
            <PRTaskIcon taskId={task.id} />
          </div>
        </div>
        {task.isRemoteExecutor && (
          <RemoteCloudTooltip
            taskId={task.id}
            sessionId={task.primarySessionId ?? null}
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
      <KanbanCardBadges task={task} />
    </>
  );
}

function KanbanCardBadges({ task }: { task: Task }) {
  const parentTitle = useAppStore((s) => {
    if (!task.parentTaskId) return null;
    return s.kanban.tasks.find((t) => t.id === task.parentTaskId)?.title ?? null;
  });

  const showRow =
    (task.sessionCount && task.sessionCount > 1) ||
    task.reviewStatus === "changes_requested" ||
    task.reviewStatus === "pending" ||
    task.parentTaskId;

  if (!showRow) return null;

  return (
    <div className="flex flex-wrap items-center justify-end gap-2 mt-1 min-w-0">
      {task.parentTaskId && (
        <Badge variant="outline" className="text-xs h-5 gap-1 max-w-[160px] min-w-0">
          <IconSubtask className="h-3 w-3 shrink-0" />
          <span className="truncate">{parentTitle ?? "Subtask"}</span>
        </Badge>
      )}
      {task.sessionCount && task.sessionCount > 1 && (
        <Badge variant="secondary" className="text-xs h-5">
          {task.sessionCount} sessions
        </Badge>
      )}
      {task.reviewStatus === "pending" && task.state !== "IN_PROGRESS" && (
        <div className="flex items-center gap-1 text-amber-700 dark:text-amber-600">
          <IconAlertCircle className="h-3.5 w-3.5" />
          <span className="text-[10px] font-medium">Approval Required</span>
        </div>
      )}
      {task.reviewStatus === "changes_requested" && (
        <Badge
          variant="outline"
          className="border-amber-500 text-amber-600 bg-amber-50 dark:bg-amber-950/50 text-xs h-5"
        >
          Changes Requested
        </Badge>
      )}
    </div>
  );
}

function KanbanCardActions({
  task,
  showMaximizeButton,
  onOpenFullPage,
  menuEntries,
  isDeleting,
  isArchiving,
}: KanbanCardActionProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  const [storePrimarySessionState, setStorePrimarySessionState] = useState<string | null>(null);
  const storeApi = useAppStoreApi();
  const debugEnabled = isDebug();
  const effectiveMenuOpen = menuOpen || Boolean(isDeleting) || Boolean(isArchiving);
  const hasPendingClarificationRequest = useTaskPendingClarification(task);
  const showQuestionIcon = shouldUseQuestionTaskIcon(task.state, hasPendingClarificationRequest);
  const showRunningSpinner = shouldShowTaskRunningSpinner(task.state, task.primarySessionState);
  const storeWouldShowRunningSpinner =
    storePrimarySessionState === null
      ? null
      : shouldShowTaskRunningSpinner(task.state, storePrimarySessionState);
  const hasSpinnerMismatch =
    showRunningSpinner &&
    storeWouldShowRunningSpinner === false &&
    task.primarySessionState !== storePrimarySessionState;
  const statusIcon = showRunningSpinner ? (
    <IconLoader2 className="h-4 w-4 text-blue-500 animate-spin" />
  ) : (
    getTaskStateIcon(task.state, "h-4 w-4", hasPendingClarificationRequest)
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
      {(showRunningSpinner || showQuestionIcon) && statusIcon}
      {showMaximizeButton && onOpenFullPage && hasKnownSession && (
        <button
          type="button"
          className="text-muted-foreground hover:text-foreground hover:bg-accent rounded-sm p-1 -m-1 transition-colors cursor-pointer"
          onClick={(event) => {
            event.stopPropagation();
            onOpenFullPage(task);
          }}
          onPointerDown={(event) => event.stopPropagation()}
          aria-label="Open full page"
          title="Open full page"
        >
          <IconArrowsMaximize className="h-4 w-4" />
        </button>
      )}
      <KanbanCardMenu
        task={task}
        effectiveMenuOpen={effectiveMenuOpen}
        setMenuOpen={setMenuOpen}
        isDeleting={isDeleting}
        isArchiving={isArchiving}
        menuEntries={menuEntries}
      />
    </div>
  );
}

type KanbanCardMenuProps = KanbanCardActionProps & {
  effectiveMenuOpen: boolean;
  setMenuOpen: (open: boolean) => void;
};

function KanbanCardMenu(props: KanbanCardMenuProps) {
  const { effectiveMenuOpen, setMenuOpen, isDeleting, isArchiving } = props;
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
          type="button"
          className="text-muted-foreground hover:text-foreground hover:bg-muted rounded-sm p-1 -m-1 transition-colors cursor-pointer"
          onClick={(e) => e.stopPropagation()}
          onPointerDown={(e) => e.stopPropagation()}
          aria-label="More options"
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
  return (
    <div
      className="mt-0.5 shrink-0"
      onClick={onCheckboxClick}
      onPointerDown={(e) => e.stopPropagation()}
      data-testid={`task-select-checkbox-${taskId}`}
    >
      <Checkbox
        checked={!!isSelected}
        aria-label={`Select task ${taskTitle}`}
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
      className={cn(
        "group max-h-48 bg-card rounded-sm data-[size=sm]:py-1 cursor-pointer mb-2 w-full py-0 relative border border-border overflow-visible shadow-none ring-0",
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
              actions={
                <KanbanCardActionSlot
                  isMultiSelectMode={isMultiSelectMode}
                  task={task}
                  showMaximizeButton={showMaximizeButton}
                  onOpenFullPage={onOpenFullPage}
                  menuEntries={menuEntries}
                  isDeleting={isDeleting}
                  isArchiving={isArchiving}
                />
              }
            />
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
