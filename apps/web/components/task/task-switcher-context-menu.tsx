"use client";

import { cloneElement, isValidElement, useState, type ReactNode } from "react";
import { ContextMenu, ContextMenuContent, ContextMenuTrigger } from "@kandev/ui/context-menu";
import {
  TaskMoveOptionsSurface,
  type TaskMoveWorkflow,
  useTaskMoveOptions,
} from "@/components/task/task-move-context-menu";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { isWorkflowMoveOptionsTarget } from "@/components/task/workflow-move-surface";
import type { WorkflowMoveEntryOptions } from "@/lib/api/domains/kanban-api";
import type { TaskLinkHandlers } from "./task-switcher-link-menu";
import type { StepDef, TaskSwitcherItem } from "./task-switcher-types";
import { useTaskSwitcherArchiveConfirmation } from "./task-switcher-archive-confirmation";
import {
  BulkSelectionMenuItems,
  SingleSelectionMenuItems,
} from "./task-switcher-context-menu-items";
import { useMenuTouchDragCancel } from "./task-switcher-touch-drag-cancel";

export type { StepDef } from "./task-switcher-types";
export { createTaskLinkSelectAction } from "./task-switcher-link-menu";

type ContextMenuProps = TaskLinkHandlers & {
  task: TaskSwitcherItem;
  nestCandidateTasks?: TaskSwitcherItem[];
  nestHierarchyTasks?: TaskSwitcherItem[];
  getNestCandidateTasks?: () => TaskSwitcherItem[];
  getNestHierarchyTasks?: () => TaskSwitcherItem[] | undefined;
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
  onRequestMoveOptions?: (taskId: string, workflowId: string, targetStepId: string) => void;
  onBeforeMoveOptionsOpen?: () => void;
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

type OpenedNestSources = {
  candidates?: TaskSwitcherItem[];
  hierarchy?: TaskSwitcherItem[];
};

function useContextMenuOpenState(props: ContextMenuProps) {
  const [contextOpen, setContextOpen] = useState(false);
  const [openedNestSources, setOpenedNestSources] = useState<OpenedNestSources>({});
  const handleContextOpenChange = (open: boolean) => {
    if (open) {
      setOpenedNestSources({
        candidates: props.getNestCandidateTasks?.() ?? props.nestCandidateTasks,
        hierarchy: props.getNestHierarchyTasks?.() ?? props.nestHierarchyTasks,
      });
    }
    setContextOpen(open);
  };
  return { contextOpen, setContextOpen, openedNestSources, handleContextOpenChange };
}

// This component coordinates the context menu and drag cancellation. Archive
// state lives in its focused adapter so unavailable actions stay unavailable.
export function TaskItemWithContextMenu(props: ContextMenuProps) {
  const { children, ...menuProps } = props;
  const { task, stepsByWorkflowId, steps, onRequestMoveOptions, onBeforeMoveOptionsOpen } = props;
  const { contextOpen, setContextOpen, openedNestSources, handleContextOpenChange } =
    useContextMenuOpenState(props);
  const [menuKey, setMenuKey] = useState(0);
  const moveTasks = useTaskWorkflowMove();
  const closeMenu = () => {
    setContextOpen(false);
    setMenuKey((k) => k + 1);
  };
  const { handleOpenChange, triggerProps } = useMenuTouchDragCancel(handleContextOpenChange);
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const usesTouchDrawer = useTouchDrawer();
  const archive = useTaskSwitcherArchiveConfirmation({
    task: menuProps.task,
    onArchiveTask: menuProps.onArchiveTask,
    isArchiving: menuProps.isArchiving,
    closeMenu,
  });
  const archiveConfirmation = archive.archiveOpen ? archive.archiveConfirmation : undefined;
  const inlineArchiveConfirmation = isMobile || isFinePointer ? undefined : archiveConfirmation;
  const portaledArchiveConfirmation = isMobile || isFinePointer ? archiveConfirmation : undefined;
  const {
    moveOptionsStep,
    isMoving,
    openMoveOptions,
    submitMoveOptions,
    submitMoveOptionsForStep,
    closeMoveOptions,
  } = useTaskMoveOptions({
    taskId: task.id,
    workflowId: task.workflowId,
    steps: task.workflowId ? (stepsByWorkflowId?.[task.workflowId] ?? steps) : steps,
    closeMenu,
  });
  const handleMoveToStepWithOptions = (targetStepId: string) => {
    if (onRequestMoveOptions && task.workflowId) {
      closeMenu();
      onRequestMoveOptions(task.id, task.workflowId, targetStepId);
      return;
    }
    onBeforeMoveOptionsOpen?.();
    openMoveOptions(targetStepId);
  };
  // Fine pointers get the options inline in the "Move to" submenu; coarse/touch
  // pointers keep the long-press Drawer surface driven by
  // handleMoveToStepWithOptions. A wired onRequestMoveOptions (mobile sheet)
  // owns its own drawer, so the inline path stays off there too.
  const inlineSubmitWithOptions =
    !usesTouchDrawer && !onRequestMoveOptions ? submitMoveOptionsForStep : undefined;
  const contextMenuProps = buildTaskContextMenuItemsProps(
    props,
    openedNestSources,
    closeMenu,
    moveTasks,
    {
      onMoveToStepWithOptions: handleMoveToStepWithOptions,
      onSubmitWithOptions: inlineSubmitWithOptions,
      isMoving,
    },
  );

  return (
    <>
      <ContextMenu key={menuKey} onOpenChange={handleOpenChange}>
        <ContextMenuTrigger asChild>
          <div ref={archive.archiveAnchorRef} tabIndex={-1} {...triggerProps}>
            {cloneWithMenuOpen(children, contextOpen, inlineArchiveConfirmation)}
            {portaledArchiveConfirmation}
          </div>
        </ContextMenuTrigger>
        <ContextMenuContent
          onCloseAutoFocus={archive.handleMenuCloseAutoFocus}
          className="w-48"
          onInteractOutside={(event) => {
            if (isWorkflowMoveOptionsTarget(event.target)) {
              event.preventDefault();
            }
          }}
          // The menu renders in a portal whose fiber ancestors include the
          // dnd-kit drag handle that wraps the row. React synthetic events
          // bubble through the fiber tree, not the DOM, so without these guards
          // a mousedown/pointerdown/touchstart on any menu item reaches the
          // handle's sensor listeners and starts a row drag, and a click
          // activates the row. Bubble-phase guards run after the item's own
          // handlers, so menu actions still work.
          onMouseDown={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
          onTouchStart={(event) => event.stopPropagation()}
          onClick={(event) => event.stopPropagation()}
        >
          <TaskContextMenuItems {...contextMenuProps} onArchiveTask={archive.requestArchive} />
        </ContextMenuContent>
      </ContextMenu>
      <TaskMoveOptionsSurface
        step={moveOptionsStep}
        isMoving={isMoving}
        onClose={closeMoveOptions}
        onSubmit={submitMoveOptions}
      />
    </>
  );
}

export type TaskContextMenuItemsProps = Omit<
  ContextMenuProps,
  | "children"
  | "onBeforeMoveOptionsOpen"
  | "onRequestMoveOptions"
  | "getNestCandidateTasks"
  | "getNestHierarchyTasks"
> & {
  closeMenu: () => void;
  moveTasks: ReturnType<typeof useTaskWorkflowMove>;
  onMoveToStepWithOptions?: (targetStepId: string) => void;
  onSubmitWithOptions?: (
    stepId: string,
    entryOptions: WorkflowMoveEntryOptions | undefined,
  ) => Promise<boolean>;
  moveOptionsBusy?: boolean;
};

function buildTaskContextMenuItemsProps(
  props: ContextMenuProps,
  nestSources: OpenedNestSources,
  closeMenu: () => void,
  moveTasks: ReturnType<typeof useTaskWorkflowMove>,
  moveOptions: {
    onMoveToStepWithOptions: (targetStepId: string) => void;
    onSubmitWithOptions?: (
      stepId: string,
      entryOptions: WorkflowMoveEntryOptions | undefined,
    ) => Promise<boolean>;
    isMoving?: boolean;
  },
): TaskContextMenuItemsProps {
  const {
    children: _children,
    onRequestMoveOptions: _onRequestMoveOptions,
    onBeforeMoveOptionsOpen: _onBeforeMoveOptionsOpen,
    getNestCandidateTasks: _getNestCandidateTasks,
    getNestHierarchyTasks: _getNestHierarchyTasks,
    ...rest
  } = props;
  return {
    ...rest,
    nestCandidateTasks: nestSources.candidates ?? rest.nestCandidateTasks,
    nestHierarchyTasks: nestSources.hierarchy ?? rest.nestHierarchyTasks,
    onMoveToStepWithOptions: moveOptions.onMoveToStepWithOptions,
    onSubmitWithOptions: moveOptions.onSubmitWithOptions,
    moveOptionsBusy: moveOptions.isMoving,
    closeMenu,
    moveTasks,
  };
}

function TaskContextMenuItems(props: TaskContextMenuItemsProps) {
  const { task, selectedTaskIds } = props;
  // Right-clicking any row that's part of the active selection acts on the
  // whole selection, even a one-row selection. Right-clicking a non-selected
  // row acts on just that task and leaves the selection intact.
  const actingOnSelection = !!selectedTaskIds?.has(task.id);
  const actingIds = actingOnSelection ? [...selectedTaskIds!] : [task.id];

  // With several tasks selected, only actions that make sense for all of them
  // are offered. Single-task actions are hidden for that reduced menu.
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

function cloneWithMenuOpen(
  children: React.ReactElement<{ menuOpen?: boolean; archiveConfirmation?: ReactNode }>,
  menuOpen: boolean,
  archiveConfirmation?: ReactNode,
): React.ReactNode {
  if (isValidElement(children)) return cloneElement(children, { menuOpen, archiveConfirmation });
  return children;
}
