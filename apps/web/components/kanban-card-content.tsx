"use client";

import { type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { CSS, type Transform } from "@dnd-kit/utilities";
import type { DraggableAttributes, DraggableSyntheticListeners } from "@dnd-kit/core";
import { Card, CardContent } from "@kandev/ui/card";
import { Checkbox } from "@kandev/ui/checkbox";
import { PRTaskIcon } from "@/components/github/pr-task-icon";
import { MRTaskIcon } from "@/components/gitlab/mr-task-icon";
import { RegisteredChangeRequestTaskIcon } from "@/components/integrations/registered-change-request-task-icon";
import { KanbanCardActions } from "@/components/kanban-card-actions";
import { type KanbanCardMenuEntry } from "@/components/kanban-card-menu-items";
import { TaskCardIndicators, TaskCardTags } from "@/components/kanban-card-plugin-slots";
import {
  KanbanCardBadges,
  KanbanCardRelationship,
  RepoChipRow,
} from "@/components/kanban-card-status-strip";
import { KanbanCardPriorityIndicator } from "@/components/kanban-card-priority-indicator";
import { CardTitle } from "@/components/kanban-card-title";
import { RemoteCloudTooltip } from "@/components/task/remote-cloud-tooltip";
import { taskPRInfoFromSummary } from "@/lib/task-pr-info";
import { cn } from "@/lib/utils";
import { needsAction } from "@/lib/utils/needs-action";
import type { RepositoryChip, Task } from "@/components/kanban-card";

export {
  renderSubagentCountChip,
  renderTaskStatusIcon,
} from "@/components/kanban-card-status-icon";

export type KanbanCardActionProps = {
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
    /** Keyboard reorder (REQ-TASKS-KANBAN-TASK-REORDERING-001.12). */
    onKeyDown?: (event: React.KeyboardEvent) => void;
    isPickedUpForReorder?: boolean;
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
            <PRTaskIcon taskId={task.id} prInfo={taskPRInfoFromSummary(task.statusSummary)} />
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

function getKanbanCardShellClassName(
  task: Task,
  state: {
    isSelected?: boolean;
    isDragging?: boolean;
    isPreviewed?: boolean;
    isPickedUpForReorder?: boolean;
  },
): string {
  const { isSelected, isDragging, isPreviewed, isPickedUpForReorder } = state;
  return cn(
    "group max-h-48 bg-card rounded-sm data-[size=sm]:py-1 cursor-pointer mb-2 w-full py-0 relative border border-border overflow-visible shadow-none ring-0",
    "touch-none md:touch-auto",
    needsAction(task) && !isSelected && "border-l-2 border-l-amber-500",
    isDragging && "opacity-50 z-50",
    isSelected && "ring-1 ring-primary/60 border-primary/60",
    isPreviewed && !isSelected && "ring-2 ring-primary border-primary",
    isPickedUpForReorder && "ring-2 ring-primary border-primary",
  );
}

/** Suppresses dnd-kit's drag/keyboard wiring while multi-select is active. */
function getDragInteractionProps(
  isMultiSelectMode: boolean | undefined,
  listeners: DraggableSyntheticListeners,
  attributes: DraggableAttributes,
  onKeyDown: ((event: React.KeyboardEvent) => void) | undefined,
) {
  if (isMultiSelectMode) return {};
  return { ...listeners, ...attributes, onKeyDown };
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
  onKeyDown,
  isPickedUpForReorder,
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
      className={getKanbanCardShellClassName(task, {
        isSelected,
        isDragging,
        isPreviewed,
        isPickedUpForReorder,
      })}
      aria-grabbed={isPickedUpForReorder || undefined}
      onClick={onClick}
      {...getDragInteractionProps(isMultiSelectMode, listeners, attributes, onKeyDown)}
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
