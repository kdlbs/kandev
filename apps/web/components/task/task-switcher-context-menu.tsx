"use client";

import { cloneElement, isValidElement, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import {
  IconCopy,
  IconEdit,
  IconPencil,
  IconPin,
  IconPinFilled,
  IconTrash,
} from "@tabler/icons-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
import {
  TaskMoveContextMenuItems,
  type TaskMoveWorkflow,
} from "@/components/task/task-move-context-menu";
import { TaskNestContextMenuItems } from "@/components/task/task-nest-context-menu";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import { TaskColorMenu } from "./task-switcher-color-menu";
import {
  TaskPluginLinkMenu,
  selectTaskLinkActions,
  type TaskLinkHandlers,
} from "./task-switcher-link-menu";
import {
  TaskArchiveItem,
  TaskCreateSubtaskItem,
  TaskDeleteItem,
  TaskDetachItem,
} from "./task-switcher-action-items";
import type { StepDef, TaskSwitcherItem } from "./task-switcher-types";
import { TaskPluginPrimaryMenuItems } from "./task-switcher-plugin-menu-items";
import { useTaskSwitcherArchiveConfirmation } from "./task-switcher-archive-confirmation";
export type { StepDef } from "./task-switcher-types";
export { createTaskLinkSelectAction } from "./task-switcher-link-menu";

type ContextMenuProps = TaskLinkHandlers & {
  task: TaskSwitcherItem;
  workflows?: TaskMoveWorkflow[];
  stepsByWorkflowId?: Record<string, StepDef[]>;
  steps?: StepDef[];
  children: React.ReactElement<{ menuOpen?: boolean; archiveConfirmation?: ReactNode }>;
  onEditTask?: (task: TaskSwitcherItem) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string, opts?: { cascade?: boolean }) => void;
  onCreateSubtask?: (taskId: string, taskTitle: string) => void;
  onDeleteTask?: (taskId: string) => void;
  onDetachTask?: (taskId: string) => void;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  onTogglePin?: (taskId: string) => void;
  isPinned?: boolean;
  pinnedTaskIds?: string[];
  isDeleting?: boolean;
  isArchiving?: boolean;
  /** Active multi-selection; when this task is part of it, actions apply to the whole set. */
  selectedTaskIds?: Set<string>;
  onBulkArchive?: (taskIds: string[]) => void;
  onBulkDelete?: (taskIds: string[]) => void;
  onBulkPin?: (taskIds: string[]) => void;
  onBulkMove?: (taskIds: string[], targetWorkflowId: string, targetStepId: string) => void;
  onClearSelection?: () => void;
  /** True when the selection spans more than one workflow (disables bulk "Move to step"). */
  isMixedWorkflowSelection?: boolean;
};

/**
 * dnd-kit's TouchSensor arms on touchstart and activates after the 250ms
 * delay — before a long-press (≈700ms) can open this context menu. A
 * stationary long-press therefore starts a row drag that is still live when
 * the menu opens. While a drag is active the TouchSensor listens for
 * `touchcancel` on the element the touch started on, so dispatching one at
 * that element aborts the drag (onDragCancel) instead of dropping it: the
 * row stays put and the menu remains usable. Inert when no touch has started
 * on this row (desktop right-click, or a sensor that already detached after a
 * quick tap), because then nothing listens for the event.
 */
type CancelTouchDrag = (touchStartTarget: EventTarget | null) => void;

const cancelTouchDrag: CancelTouchDrag = (touchStartTarget) => {
  if (touchStartTarget instanceof Element && typeof TouchEvent === "function") {
    touchStartTarget.dispatchEvent(
      new TouchEvent("touchcancel", { bubbles: true, cancelable: true }),
    );
  }
};

/**
 * Coordinates the context menu with the row's touch-drag sensor: remembers
 * the element the touch began on and cancels the in-flight drag when the menu
 * opens. Returns the menu `onOpenChange` handler and the trigger-wrapper
 * capture props.
 */
function useMenuTouchDragCancel(onOpenChange: (open: boolean) => void) {
  const touchStartRef = useRef<{ target: EventTarget; identifier: number } | null>(null);
  const menuOpenRef = useRef(false);
  const handleOpenChange = (open: boolean) => {
    onOpenChange(open);
    menuOpenRef.current = open;
    if (open) {
      // A touch long-press has already armed the row's TouchSensor (250ms)
      // when the menu opens (~700ms); cancel that drag at the touchstart
      // target so the menu gesture never moves the row.
      const target = touchStartRef.current?.target ?? null;
      touchStartRef.current = null;
      cancelTouchDrag(target);
    } else {
      touchStartRef.current = null;
    }
  };
  return {
    handleOpenChange,
    triggerProps: {
      // The TouchSensor attaches its touchcancel listener to the element the
      // touch began on while a drag is active. Track only the first touch of
      // a single-touch gesture (dnd-kit's TouchSensor rejects multi-touch)
      // and only while the menu is closed, and drop the target when that
      // touch ends or the menu closes — not when another finger lifts — so a
      // later open never dispatches a synthetic touchcancel for a gesture
      // that is no longer active (pull-to-refresh and touch-scroll listen for
      // bubbled touchcancel).
      onTouchStartCapture: (event: React.TouchEvent) => {
        if (menuOpenRef.current || event.touches.length !== 1) return;
        if (!touchStartRef.current) {
          touchStartRef.current = {
            target: event.target,
            identifier: event.touches[0].identifier,
          };
        }
      },
      onTouchEndCapture: (event: React.TouchEvent) => {
        const tracked = touchStartRef.current;
        if (
          tracked &&
          Array.from(event.changedTouches).some((t) => t.identifier === tracked.identifier)
        ) {
          touchStartRef.current = null;
        }
      },
      onTouchCancelCapture: (event: React.TouchEvent) => {
        const tracked = touchStartRef.current;
        if (
          tracked &&
          Array.from(event.changedTouches).some((t) => t.identifier === tracked.identifier)
        ) {
          touchStartRef.current = null;
        }
      },
    },
  };
}

// This component coordinates the context menu and drag cancellation. Archive
// state lives in its focused adapter so unavailable actions stay unavailable.
export function TaskItemWithContextMenu(props: ContextMenuProps) {
  const { children, ...menuProps } = props;
  const [contextOpen, setContextOpen] = useState(false);
  const [menuKey, setMenuKey] = useState(0);
  const moveTasks = useTaskWorkflowMove();
  const closeMenu = () => {
    setContextOpen(false);
    setMenuKey((k) => k + 1);
  };
  const { handleOpenChange, triggerProps } = useMenuTouchDragCancel(setContextOpen);
  const archive = useTaskSwitcherArchiveConfirmation({
    task: menuProps.task,
    onArchiveTask: menuProps.onArchiveTask,
    isArchiving: menuProps.isArchiving,
    closeMenu,
  });

  return (
    <ContextMenu key={menuKey} onOpenChange={handleOpenChange}>
      <ContextMenuTrigger asChild>
        <div ref={archive.archiveAnchorRef} tabIndex={-1} {...triggerProps}>
          {cloneWithMenuOpen(
            children,
            contextOpen,
            archive.archiveOpen ? archive.archiveConfirmation : undefined,
          )}
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent
        className="w-48"
        // The menu renders in a portal whose fiber ancestors include the
        // dnd-kit drag handle that wraps the row. React synthetic events
        // bubble through the fiber tree, not the DOM, so without these guards
        // a mousedown/pointerdown/touchstart on any menu item (e.g. the Color
        // submenu trigger or a swatch) reaches the handle's sensor listeners
        // and starts a row drag, and a click activates the row. Bubble-phase
        // guards run after the item's own handlers, so menu actions still work.
        onMouseDown={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
        onTouchStart={(event) => event.stopPropagation()}
        onClick={(event) => event.stopPropagation()}
      >
        <TaskContextMenuItems
          {...menuProps}
          onArchiveTask={archive.requestArchive}
          closeMenu={closeMenu}
          moveTasks={moveTasks}
        />
      </ContextMenuContent>
    </ContextMenu>
  );
}

type TaskContextMenuItemsProps = Omit<ContextMenuProps, "children"> & {
  closeMenu: () => void;
  moveTasks: ReturnType<typeof useTaskWorkflowMove>;
};

function TaskContextMenuItems(props: TaskContextMenuItemsProps) {
  const { task, selectedTaskIds } = props;
  // Right-clicking any row that's part of the active selection acts on the whole
  // selection (even a one-row selection, so the action clears it); right-clicking
  // a non-selected row acts on just that task and leaves the selection intact.
  const actingOnSelection = !!selectedTaskIds?.has(task.id);
  const actingIds = actingOnSelection ? [...selectedTaskIds!] : [task.id];

  // With several tasks selected, only actions that make sense for all of them
  // are offered (Pin / Move / Archive / Delete) — the single-task actions
  // (Rename, Color, Link, Duplicate) are hidden.
  if (actingOnSelection && actingIds.length > 1) {
    return <BulkSelectionMenuItems {...props} actingIds={actingIds} />;
  }
  return (
    <SingleSelectionMenuItems
      {...props}
      actingIds={actingIds}
      actingOnSelection={actingOnSelection}
    />
  );
}

function SingleSelectionMenuItems({
  task,
  workflows,
  stepsByWorkflowId,
  steps,
  onEditTask,
  onRenameTask,
  onArchiveTask,
  onCreateSubtask,
  onDeleteTask,
  onDetachTask,
  onMoveToStep,
  onTogglePin,
  isPinned,
  isDeleting,
  isArchiving,
  onBulkArchive,
  onBulkMove,
  onClearSelection,
  isMixedWorkflowSelection,
  closeMenu,
  moveTasks,
  actingIds,
  actingOnSelection,
  ...linkHandlers
}: TaskContextMenuItemsProps & { actingIds: string[]; actingOnSelection: boolean }) {
  const { t } = useTranslation();
  // Acting on a lone selected row (Pin / Delete) must drop it from the selection
  // so later plain clicks navigate instead of toggling.
  const onDelete = withSelectionClear(actingOnSelection, onClearSelection, onDeleteTask);
  const onDetach = withSelectionClear(actingOnSelection, onClearSelection, onDetachTask);
  return (
    <>
      <TaskPinItem
        taskId={task.id}
        isPinned={isPinned}
        disabled={isDeleting}
        onTogglePin={withSelectionClear(actingOnSelection, onClearSelection, onTogglePin)}
      />
      <TaskEditItem task={task} disabled={isDeleting} onEditTask={onEditTask} />
      <TaskRenameItem task={task} disabled={isDeleting} onRenameTask={onRenameTask} />
      <TaskCreateSubtaskItem task={task} disabled={isDeleting} onCreateSubtask={onCreateSubtask} />
      {!task.isArchived && (
        <ContextMenuItem disabled>
          <IconCopy className="mr-2 h-4 w-4" />
          {t("settings:duplicate")}
        </ContextMenuItem>
      )}
      <TaskArchiveItem
        taskId={task.id}
        actingIds={actingIds}
        actingOnSelection={actingOnSelection}
        disabled={isDeleting || isArchiving}
        onArchiveTask={onArchiveTask}
        onBulkArchive={onBulkArchive}
      />
      {!task.isArchived && <TaskColorMenu taskId={task.id} disabled={isDeleting} />}
      <TaskNestContextMenuItems task={task} disabled={isDeleting} />
      <TaskPluginPrimaryMenuItems task={task} disabled={isDeleting} />
      <TaskPluginLinkMenu
        task={task}
        disabled={isDeleting}
        closeMenu={closeMenu}
        linkActions={selectTaskLinkActions(task, closeMenu, linkHandlers)}
      />
      {!task.isArchived && (
        <TaskMoveItems
          task={task}
          workflows={workflows}
          stepsByWorkflowId={stepsByWorkflowId}
          steps={steps}
          isDeleting={isDeleting}
          onMoveToStep={onMoveToStep}
          actingIds={actingIds}
          actingOnSelection={actingOnSelection}
          onBulkMove={onBulkMove}
          isMixedWorkflowSelection={isMixedWorkflowSelection}
          closeMenu={closeMenu}
          moveTasks={moveTasks}
        />
      )}
      <TaskDetachItem task={task} disabled={isDeleting} onDetachTask={onDetach} />
      <TaskDeleteItem taskId={task.id} isDeleting={isDeleting} onDeleteTask={onDelete} />
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

/** Reduced menu shown when 2+ tasks are selected — only bulk-valid actions. */
function BulkSelectionMenuItems({
  task,
  actingIds,
  workflows,
  stepsByWorkflowId,
  steps,
  isMixedWorkflowSelection,
  pinnedTaskIds,
  onBulkPin,
  onBulkArchive,
  onBulkDelete,
  onBulkMove,
  closeMenu,
  moveTasks,
}: {
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
}) {
  const { t } = useTranslation();
  const n = actingIds.length;
  const allPinned =
    actingIds.length > 0 && actingIds.every((id) => pinnedTaskIds?.includes(id) ?? false);
  const pinLabel = allPinned
    ? t("task:unpinTasksCount", { count: n })
    : t("task:pinTasksCount", { count: n });
  return (
    <>
      {onBulkPin && (
        <ContextMenuItem onSelect={() => onBulkPin(actingIds)}>
          {allPinned ? (
            <IconPinFilled className="mr-2 h-4 w-4" />
          ) : (
            <IconPin className="mr-2 h-4 w-4" />
          )}
          {pinLabel}
        </ContextMenuItem>
      )}
      <TaskArchiveItem
        taskId={task.id}
        actingIds={actingIds}
        actingOnSelection
        onArchiveTask={undefined}
        onBulkArchive={onBulkArchive}
      />
      <TaskMoveItems
        task={task}
        workflows={workflows}
        stepsByWorkflowId={stepsByWorkflowId}
        steps={steps}
        onMoveToStep={undefined}
        actingIds={actingIds}
        actingOnSelection
        onBulkMove={onBulkMove}
        isMixedWorkflowSelection={isMixedWorkflowSelection}
        closeMenu={closeMenu}
        moveTasks={moveTasks}
      />
      {onBulkDelete && (
        <>
          <ContextMenuSeparator />
          <ContextMenuItem variant="destructive" onSelect={() => onBulkDelete(actingIds)}>
            <IconTrash className="mr-2 h-4 w-4" />
            {t("task:deleteTasksCount", { count: n })}
          </ContextMenuItem>
        </>
      )}
    </>
  );
}

function cloneWithMenuOpen(
  children: React.ReactElement<{ menuOpen?: boolean; archiveConfirmation?: ReactNode }>,
  menuOpen: boolean,
  archiveConfirmation?: ReactNode,
): React.ReactNode {
  if (isValidElement(children)) return cloneElement(children, { menuOpen, archiveConfirmation });
  return children;
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

function TaskMoveItems({
  task,
  workflows,
  stepsByWorkflowId,
  steps,
  isDeleting,
  onMoveToStep,
  actingIds,
  actingOnSelection,
  onBulkMove,
  isMixedWorkflowSelection,
  closeMenu,
  moveTasks,
}: Omit<TaskContextMenuItemsProps, "onRenameTask" | "onArchiveTask" | "onDeleteTask"> & {
  actingIds: string[];
  actingOnSelection: boolean;
}) {
  if (!task.workflowId) return null;
  const workflowId = task.workflowId;
  // Moving a selection routes through the sidebar hook's bulkMove, which clears
  // the selection afterwards. Fall back to a raw move when no bulk handler is
  // wired (e.g. the kanban-less callers that don't manage a selection).
  const runSelectionMove = (
    targetWorkflowId: string,
    stepId: string,
    destination: "step" | "workflow",
  ) => {
    closeMenu();
    if (onBulkMove) {
      onBulkMove(actingIds, targetWorkflowId, stepId);
      return;
    }
    void moveTasks(actingIds, targetWorkflowId, stepId, destination).catch(() => {
      // useTaskWorkflowMove already shows the failure toast.
    });
  };

  // Single-task right-click keeps the optimistic same-workflow move. A selection
  // spanning workflows makes "Move to step" of one workflow ambiguous, so disable
  // it there (Send to workflow remains the explicit path).
  let moveToStep: ((stepId: string) => void) | undefined;
  if (actingOnSelection) {
    moveToStep = isMixedWorkflowSelection
      ? undefined
      : (stepId) => runSelectionMove(workflowId, stepId, "step");
  } else {
    moveToStep = (stepId) => {
      closeMenu();
      if (onMoveToStep) {
        onMoveToStep(task.id, workflowId, stepId);
        return;
      }
      void moveTasks([task.id], workflowId, stepId, "step").catch(() => {
        // useTaskWorkflowMove already shows the failure toast.
      });
    };
  }

  return (
    <TaskMoveContextMenuItems
      currentWorkflowId={workflowId}
      // For a selection spanning several steps, don't disable the clicked row's
      // step — the backend bulk move skips tasks already there, and the other
      // selected rows still need it as a target.
      currentStepId={actingOnSelection ? undefined : task.workflowStepId}
      workflows={workflows ?? []}
      stepsByWorkflowId={stepsByWorkflowId ?? (steps ? { [workflowId]: steps } : {})}
      disabled={isDeleting || task.isArchived}
      onMoveToStep={moveToStep}
      onSendToWorkflow={(targetWorkflowId, stepId) => {
        if (actingOnSelection) {
          runSelectionMove(targetWorkflowId, stepId, "workflow");
          return;
        }
        closeMenu();
        void moveTasks([task.id], targetWorkflowId, stepId, "workflow").catch(() => {
          // useTaskWorkflowMove already shows the failure toast.
        });
      }}
    />
  );
}
