"use client";

import { useTranslation } from "react-i18next";
import { Fragment, type ReactNode } from "react";
import {
  IconCopy,
  IconEdit,
  IconPencil,
  IconPin,
  IconPinFilled,
  IconTrash,
} from "@tabler/icons-react";
import { ContextMenuItem, ContextMenuSeparator } from "@kandev/ui/context-menu";
import type { TaskMoveWorkflow } from "@/components/task/task-move-context-menu";
import {
  KanbanCardContextMenuItems,
  type KanbanCardMenuEntry,
} from "@/components/kanban-card-menu-items";
import { TaskNestContextMenuItems } from "@/components/task/task-nest-context-menu";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import { useUpdateTaskPriority } from "@/hooks/use-update-task-priority";
import { TaskColorMenu } from "./task-switcher-color-menu";
import { TaskPluginLinkMenu, selectTaskLinkActions } from "./task-switcher-link-menu";
import {
  TaskArchiveItem,
  TaskCreateSubtaskItem,
  TaskDeleteItem,
  TaskDetachItem,
} from "./task-switcher-action-items";
import {
  useTaskPluginLinkActions,
  type PluginLinkMenuAction,
} from "./task-session-sidebar-link-actions";
import type { StepDef, TaskSwitcherItem } from "./task-switcher-types";
import { useTaskPluginPrimaryMenuEntries } from "./task-switcher-plugin-menu-items";
import { TaskPriorityContextMenu } from "./task-priority-context-menu";
import type { TaskContextMenuItemsProps } from "./task-switcher-context-menu";
import { TaskMoveItems } from "./task-switcher-context-menu-move-items";

type SingleSelectionMenuProps = TaskContextMenuItemsProps & {
  actingIds: string[];
  actingOnSelection: boolean;
};

type LinkActions = ReturnType<typeof selectTaskLinkActions>;

type SingleGroupProps = Pick<
  SingleSelectionMenuProps,
  | "task"
  | "isDeleting"
  | "isArchiving"
  | "isPinned"
  | "onTogglePin"
  | "onEditTask"
  | "onRenameTask"
  | "onCreateSubtask"
  | "onDetachTask"
  | "onArchiveTask"
  | "onDeleteTask"
  | "closeMenu"
  | "actingIds"
  | "actingOnSelection"
  | "onBulkArchive"
  | "onClearSelection"
  | "onBulkMove"
  | "workflows"
  | "stepsByWorkflowId"
  | "steps"
  | "onMoveToStep"
  | "isMixedWorkflowSelection"
  | "moveTasks"
> & {
  linkActions?: LinkActions;
  pluginLinkActions?: PluginLinkMenuAction[];
  hasLinkGroup?: boolean;
};

type TaskContextMenuGroup = {
  key: string;
  visible: boolean;
  content: ReactNode;
};

function TaskContextMenuGroups({ groups }: { groups: readonly TaskContextMenuGroup[] }) {
  const visibleGroups = groups.filter((group) => group.visible);
  return (
    <>
      {visibleGroups.map((group, index) => (
        <Fragment key={group.key}>
          {index > 0 && <ContextMenuSeparator />}
          {group.content}
        </Fragment>
      ))}
    </>
  );
}

type SingleMenuState = {
  pluginPrimaryEntries: KanbanCardMenuEntry[];
  pluginLinkActions: PluginLinkMenuAction[];
  linkActions: LinkActions;
  hasMoveGroup: boolean;
  hasLinkGroup: boolean;
  hasRelationshipsGroup: boolean;
  onDelete: ((id: string) => void) | undefined;
  onDetach: ((id: string) => void) | undefined;
};

function useSingleMenuState(props: SingleSelectionMenuProps): SingleMenuState {
  const { task, closeMenu, stepsByWorkflowId, steps, workflows, actingOnSelection } = props;
  const pluginPrimaryEntries = useTaskPluginPrimaryMenuEntries(task, props.isDeleting);
  const pluginLinkActions = useTaskPluginLinkActions(task.id, task.repositoryLinks ?? []);
  const linkActions = selectTaskLinkActions(task, closeMenu, props);
  const workflowId = task.workflowId;
  const moveStepsByWorkflowId =
    stepsByWorkflowId ?? (workflowId && steps ? { [workflowId]: steps } : undefined);
  const currentMoveSteps = workflowId ? (moveStepsByWorkflowId?.[workflowId] ?? []) : [];
  const hasMoveGroup = hasSingleMoveGroup({
    task,
    workflows,
    workflowId,
    currentMoveSteps,
    actingOnSelection,
    isMixedWorkflowSelection: props.isMixedWorkflowSelection,
  });
  const hasLinkGroup = Object.values(linkActions).some(Boolean) || pluginLinkActions.length > 0;
  const hasRelationshipsGroup = Boolean(
    props.onCreateSubtask ||
    (!task.isArchived && task.workflowId) ||
    hasLinkGroup ||
    (task.parentTaskId && props.onDetachTask),
  );
  return {
    pluginPrimaryEntries,
    pluginLinkActions,
    linkActions,
    hasMoveGroup,
    hasLinkGroup,
    hasRelationshipsGroup,
    onDelete: withSelectionClear(actingOnSelection, props.onClearSelection, props.onDeleteTask),
    onDetach: withSelectionClear(actingOnSelection, props.onClearSelection, props.onDetachTask),
  };
}

function SingleSelectionMenuItems(props: SingleSelectionMenuProps) {
  return <SingleSelectionMenuGroups {...props} {...useSingleMenuState(props)} />;
}

function SingleSelectionMenuGroups(props: SingleSelectionMenuProps & SingleMenuState) {
  const { task } = props;
  return (
    <TaskContextMenuGroups
      groups={[
        {
          key: "mark",
          visible: Boolean(props.onTogglePin || !task.isArchived),
          content: <SingleMarkGroup {...props} />,
        },
        {
          key: "edit",
          visible: Boolean(!task.isArchived || props.onRenameTask),
          content: <SingleEditGroup {...props} />,
        },
        {
          key: "relationships",
          visible: props.hasRelationshipsGroup,
          content: <SingleRelationshipsGroup {...props} onDetachTask={props.onDetach} />,
        },
        {
          key: "move",
          visible: props.hasMoveGroup,
          content: <TaskMoveItems {...props} />,
        },
        {
          key: "plugins",
          visible: props.pluginPrimaryEntries.length > 0,
          content: <KanbanCardContextMenuItems entries={props.pluginPrimaryEntries} />,
        },
        {
          key: "remove",
          visible: hasSingleRemoveGroup(props),
          content: <SingleRemoveGroup {...props} onDeleteTask={props.onDelete} />,
        },
      ]}
    />
  );
}

function hasSingleMoveGroup({
  task,
  workflows,
  workflowId,
  currentMoveSteps,
  actingOnSelection,
  isMixedWorkflowSelection,
}: Pick<
  SingleSelectionMenuProps,
  "task" | "workflows" | "actingOnSelection" | "isMixedWorkflowSelection"
> & {
  workflowId: string | undefined;
  currentMoveSteps: StepDef[];
}) {
  if (task.isArchived || !workflowId) return false;
  const hasSameWorkflowMove =
    currentMoveSteps.length > 1 && (!actingOnSelection || !isMixedWorkflowSelection);
  const hasCrossWorkflowMove = (workflows ?? []).some(
    (workflow) => !workflow.hidden && workflow.id !== workflowId,
  );
  return hasSameWorkflowMove || hasCrossWorkflowMove;
}

function hasSingleRemoveGroup({
  actingOnSelection,
  onArchiveTask,
  onBulkArchive,
  onDeleteTask,
}: Pick<
  SingleSelectionMenuProps,
  "actingOnSelection" | "onArchiveTask" | "onBulkArchive" | "onDeleteTask"
>) {
  return Boolean(onArchiveTask || onDeleteTask || (actingOnSelection && onBulkArchive));
}

function SingleMarkGroup({
  task,
  isPinned,
  isDeleting,
  actingOnSelection,
  onClearSelection,
  onTogglePin,
}: Pick<
  SingleGroupProps,
  "task" | "isPinned" | "isDeleting" | "actingOnSelection" | "onClearSelection" | "onTogglePin"
>) {
  const updateTaskPriority = useUpdateTaskPriority();
  return (
    <>
      <TaskPinItem
        taskId={task.id}
        isPinned={isPinned}
        disabled={isDeleting}
        onTogglePin={withSelectionClear(actingOnSelection, onClearSelection, onTogglePin)}
      />
      {!task.isArchived && (
        <TaskColorMenu
          taskId={task.id}
          disabled={isDeleting}
          automaticColorSource={task.automaticColorSource}
        />
      )}
      {!task.isArchived && (
        <TaskPriorityContextMenu
          currentPriority={task.priority}
          disabled={isDeleting}
          onSelect={(priority) => void updateTaskPriority(task.id, priority)}
        />
      )}
    </>
  );
}

function SingleEditGroup({
  task,
  isDeleting,
  onEditTask,
  onRenameTask,
}: Pick<SingleGroupProps, "task" | "isDeleting" | "onEditTask" | "onRenameTask">) {
  const { t } = useTranslation();
  return (
    <>
      <TaskEditItem task={task} disabled={isDeleting} onEditTask={onEditTask} />
      <TaskRenameItem task={task} disabled={isDeleting} onRenameTask={onRenameTask} />
      {!task.isArchived && (
        <ContextMenuItem disabled>
          <IconCopy className="mr-2 h-4 w-4" />
          {t("settings:duplicate")}
        </ContextMenuItem>
      )}
    </>
  );
}

function SingleRelationshipsGroup({
  task,
  isDeleting,
  onCreateSubtask,
  onDetachTask,
  closeMenu,
  linkActions,
  pluginLinkActions,
  hasLinkGroup,
}: Pick<
  SingleGroupProps,
  | "task"
  | "isDeleting"
  | "onCreateSubtask"
  | "onDetachTask"
  | "closeMenu"
  | "linkActions"
  | "pluginLinkActions"
  | "hasLinkGroup"
>) {
  return (
    <>
      <TaskCreateSubtaskItem task={task} disabled={isDeleting} onCreateSubtask={onCreateSubtask} />
      {!task.isArchived && <TaskNestContextMenuItems task={task} disabled={isDeleting} />}
      {hasLinkGroup && linkActions && pluginLinkActions && (
        <TaskPluginLinkMenu
          disabled={isDeleting}
          closeMenu={closeMenu}
          linkActions={linkActions}
          pluginLinkActions={pluginLinkActions}
        />
      )}
      <TaskDetachItem task={task} disabled={isDeleting} onDetachTask={onDetachTask} />
    </>
  );
}

function SingleRemoveGroup({
  task,
  isDeleting,
  isArchiving,
  onArchiveTask,
  onDeleteTask,
  actingIds,
  actingOnSelection,
  onBulkArchive,
}: Pick<
  SingleGroupProps,
  | "task"
  | "isDeleting"
  | "isArchiving"
  | "onArchiveTask"
  | "onDeleteTask"
  | "actingIds"
  | "actingOnSelection"
  | "onBulkArchive"
>) {
  return (
    <>
      <TaskArchiveItem
        taskId={task.id}
        actingIds={actingIds}
        actingOnSelection={actingOnSelection}
        disabled={isDeleting || isArchiving}
        onArchiveTask={onArchiveTask}
        onBulkArchive={onBulkArchive}
      />
      <TaskDeleteItem
        taskId={task.id}
        isDeleting={isDeleting}
        onDeleteTask={onDeleteTask}
        showSeparator={false}
      />
    </>
  );
}

function withSelectionClear(
  actingOnSelection: boolean,
  onClearSelection: (() => void) | undefined,
  handler: ((id: string) => void) | undefined,
) {
  if (!actingOnSelection || !onClearSelection || !handler) return handler;
  return (id: string) => {
    onClearSelection();
    handler(id);
  };
}

type BulkSelectionMenuProps = {
  task: TaskSwitcherItem;
  actingIds: string[];
  workflows?: TaskMoveWorkflow[];
  stepsByWorkflowId?: Record<string, StepDef[]>;
  steps?: StepDef[];
  isMixedWorkflowSelection?: boolean;
  pinnedTaskIds?: string[];
  onBulkPin?: (taskIds: string[]) => void;
  onBulkArchive?: (taskIds: string[]) => void;
  onBulkDelete?: (taskIds: string[]) => void;
  onBulkMove?: (taskIds: string[], targetWorkflowId: string, targetStepId: string) => void;
  closeMenu: () => void;
  moveTasks: ReturnType<typeof useTaskWorkflowMove>;
};

function BulkSelectionMenuItems(props: BulkSelectionMenuProps) {
  const { task, workflows, stepsByWorkflowId, steps, isMixedWorkflowSelection } = props;
  const hasMoveGroup = hasBulkMoveGroup({
    task,
    workflows,
    stepsByWorkflowId,
    steps,
    isMixedWorkflowSelection,
  });
  return (
    <TaskContextMenuGroups
      groups={[
        {
          key: "mark",
          visible: Boolean(props.onBulkPin),
          content: <BulkPinMenuItem {...props} />,
        },
        {
          key: "move",
          visible: hasMoveGroup,
          content: <BulkMoveGroup {...props} />,
        },
        {
          key: "remove",
          visible: Boolean(props.onBulkArchive || props.onBulkDelete),
          content: <BulkRemoveGroup {...props} />,
        },
      ]}
    />
  );
}

function hasBulkMoveGroup({
  task,
  workflows,
  stepsByWorkflowId,
  steps,
  isMixedWorkflowSelection,
}: Pick<
  BulkSelectionMenuProps,
  "task" | "workflows" | "stepsByWorkflowId" | "steps" | "isMixedWorkflowSelection"
>) {
  const workflowId = task.workflowId;
  const moveStepsByWorkflowId =
    stepsByWorkflowId ?? (workflowId && steps ? { [workflowId]: steps } : undefined);
  const currentMoveSteps = workflowId ? (moveStepsByWorkflowId?.[workflowId] ?? []) : [];
  return Boolean(
    workflowId &&
    ((!isMixedWorkflowSelection && currentMoveSteps.length > 1) ||
      (workflows ?? []).some((workflow) => !workflow.hidden && workflow.id !== workflowId)),
  );
}

function BulkPinMenuItem({
  actingIds,
  pinnedTaskIds,
  onBulkPin,
}: Pick<BulkSelectionMenuProps, "actingIds" | "pinnedTaskIds" | "onBulkPin">) {
  const { t } = useTranslation();
  if (!onBulkPin) return null;
  const allPinned =
    actingIds.length > 0 && actingIds.every((id) => pinnedTaskIds?.includes(id) ?? false);
  const label = allPinned
    ? t("task:unpinTasksCount", { count: actingIds.length })
    : t("task:pinTasksCount", { count: actingIds.length });
  return (
    <ContextMenuItem onSelect={() => onBulkPin(actingIds)}>
      {allPinned ? (
        <IconPinFilled className="mr-2 h-4 w-4" />
      ) : (
        <IconPin className="mr-2 h-4 w-4" />
      )}
      {label}
    </ContextMenuItem>
  );
}

function BulkMoveGroup(props: BulkSelectionMenuProps) {
  return (
    <TaskMoveItems
      task={props.task}
      workflows={props.workflows}
      stepsByWorkflowId={props.stepsByWorkflowId}
      steps={props.steps}
      onMoveToStep={undefined}
      actingIds={props.actingIds}
      actingOnSelection
      onBulkMove={props.onBulkMove}
      isMixedWorkflowSelection={props.isMixedWorkflowSelection}
      closeMenu={props.closeMenu}
      moveTasks={props.moveTasks}
    />
  );
}

function BulkRemoveGroup({
  task,
  actingIds,
  onBulkArchive,
  onBulkDelete,
}: Pick<BulkSelectionMenuProps, "task" | "actingIds" | "onBulkArchive" | "onBulkDelete">) {
  const { t } = useTranslation();
  return (
    <>
      <TaskArchiveItem
        taskId={task.id}
        actingIds={actingIds}
        actingOnSelection
        onArchiveTask={undefined}
        onBulkArchive={onBulkArchive}
      />
      {onBulkDelete && (
        <ContextMenuItem variant="destructive" onSelect={() => onBulkDelete(actingIds)}>
          <IconTrash className="mr-2 h-4 w-4" />
          {t("task:deleteTasksCount", { count: actingIds.length })}
        </ContextMenuItem>
      )}
    </>
  );
}

function TaskPinItem({
  taskId,
  isPinned,
  disabled,
  onTogglePin,
}: {
  taskId: string;
  isPinned?: boolean;
  disabled?: boolean;
  onTogglePin?: (taskId: string) => void;
}) {
  const { t } = useTranslation();
  if (!onTogglePin) return null;
  return (
    <ContextMenuItem disabled={disabled} onSelect={() => onTogglePin(taskId)}>
      {isPinned ? <IconPinFilled className="mr-2 h-4 w-4" /> : <IconPin className="mr-2 h-4 w-4" />}
      {isPinned ? t("task:unpin") : t("task:annotationPin")}
    </ContextMenuItem>
  );
}

function TaskRenameItem({
  task,
  disabled,
  onRenameTask,
}: {
  task: TaskSwitcherItem;
  disabled?: boolean;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
}) {
  const { t } = useTranslation();
  if (!onRenameTask) return null;
  return (
    <ContextMenuItem disabled={disabled} onSelect={() => onRenameTask(task.id, task.title)}>
      <IconPencil className="mr-2 h-4 w-4" />
      {t("task:rename")}
    </ContextMenuItem>
  );
}

function TaskEditItem({
  task,
  disabled,
  onEditTask,
}: {
  task: TaskSwitcherItem;
  disabled?: boolean;
  onEditTask?: (task: TaskSwitcherItem) => void;
}) {
  const { t } = useTranslation();
  if (!onEditTask || task.isArchived || !task.workflowId || !task.workflowStepId) return null;
  return (
    <ContextMenuItem disabled={disabled} onSelect={() => onEditTask(task)}>
      <IconEdit className="mr-2 h-4 w-4" />
      {t("common:edit")}
    </ContextMenuItem>
  );
}

export { BulkSelectionMenuItems, SingleSelectionMenuItems };
