"use client";

import { cloneElement, isValidElement, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconLoader2 } from "@tabler/icons-react";
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
import { ChangeWorkflowDialog } from "./change-workflow-dialog";
import { useTaskPRUnlinkMenu } from "@/hooks/domains/github/use-task-pr-unlink-menu";
import type { KanbanCardMenuEntry } from "@/components/kanban-card-menu-items";

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

// The portal belongs to the sortable row's React tree; these guards stop events before they reach dnd-kit.
function stopMenuEvent(event: { stopPropagation: () => void }) {
  event.stopPropagation();
}

function keepWorkflowMoveOpen(event: { target: EventTarget | null; preventDefault: () => void }) {
  if (event.target && isWorkflowMoveOptionsTarget(event.target)) event.preventDefault();
}

function createCloseMenu(
  setContextOpen: (open: boolean) => void,
  setMenuKey: (update: (key: number) => number) => void,
) {
  return () => {
    setContextOpen(false);
    setMenuKey((key) => key + 1);
  };
}

function createChangeWorkflowOpenHandler(
  taskId: string,
  currentTaskId: { current: string },
  closeMenu: () => void,
  setOpen: (open: boolean) => void,
) {
  return () => {
    closeMenu();
    window.setTimeout(() => {
      if (currentTaskId.current === taskId) setOpen(true);
    }, 300);
  };
}

function buildTaskRowUnlinkEntries(
  menu: ReturnType<typeof useTaskPRUnlinkMenu>,
  translateChoice: (choice: (typeof menu.choices)[number]) => string,
  loadingLabel: string,
): KanbanCardMenuEntry[] {
  const entries: KanbanCardMenuEntry[] = menu.choices.map((choice) => ({
    kind: "item",
    key: `unlink-task-pr-${choice.associationId}`,
    testId: `task-row-unlink-task-pr-${choice.associationId}`,
    label: translateChoice(choice),
    disabled: !menu.canUnlink || choice.pending,
    onSelect: () => void menu.unlink(choice.associationId),
  }));
  if (menu.isLoading) {
    entries.push({
      kind: "item",
      key: "loading-pull-requests",
      testId: "task-row-loading-pull-requests",
      icon: <IconLoader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />,
      label: loadingLabel,
      disabled: true,
    });
  }
  return entries;
}

function useTaskRowMoveMenu(
  props: ContextMenuProps,
  nestSources: OpenedNestSources,
  closeMenu: () => void,
) {
  const { task, stepsByWorkflowId, steps, onRequestMoveOptions, onBeforeMoveOptionsOpen } = props;
  const moveTasks = useTaskWorkflowMove();
  const usesTouchDrawer = useTouchDrawer();
  const moveOptions = useTaskMoveOptions({
    taskId: task.id,
    workflowId: task.workflowId,
    steps: task.workflowId ? (stepsByWorkflowId?.[task.workflowId] ?? steps) : steps,
    closeMenu,
  });
  const onMoveToStepWithOptions = (targetStepId: string) => {
    if (onRequestMoveOptions && task.workflowId) {
      closeMenu();
      onRequestMoveOptions(task.id, task.workflowId, targetStepId);
      return;
    }
    onBeforeMoveOptionsOpen?.();
    moveOptions.openMoveOptions(targetStepId);
  };
  const onSubmitWithOptions =
    !usesTouchDrawer && !onRequestMoveOptions ? moveOptions.submitMoveOptionsForStep : undefined;
  return {
    moveOptions,
    contextMenuProps: buildTaskContextMenuItemsProps(props, nestSources, closeMenu, moveTasks, {
      onMoveToStepWithOptions,
      onSubmitWithOptions,
      isMoving: moveOptions.isMoving,
    }),
  };
}

// This component coordinates the context menu and drag cancellation. Archive
// state lives in its focused adapter so unavailable actions stay unavailable.
export function TaskItemWithContextMenu(props: ContextMenuProps) {
  const { children, ...menuProps } = props;
  const { task } = props;
  const { t } = useTranslation();
  const prUnlinkMenu = useTaskPRUnlinkMenu(task.id, task.prInfo?.number);
  const { contextOpen, setContextOpen, openedNestSources, handleContextOpenChange } =
    useContextMenuOpenState(props);
  const [menuKey, setMenuKey] = useState(0);
  const [changeWorkflowOpen, setChangeWorkflowOpen] = useState(false);
  const taskIdRef = useRef(task.id);
  taskIdRef.current = task.id;
  const closeMenu = createCloseMenu(setContextOpen, setMenuKey);
  const { handleOpenChange, triggerProps } = useMenuTouchDragCancel((open) => {
    handleContextOpenChange(open);
    prUnlinkMenu.onOpenChange(open);
  });
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const archive = useTaskSwitcherArchiveConfirmation({
    task: menuProps.task,
    onArchiveTask: menuProps.onArchiveTask,
    isArchiving: menuProps.isArchiving,
    closeMenu,
  });
  const changeWorkflowTriggerRef = archive.archiveAnchorRef;
  const archiveConfirmation = archive.archiveOpen ? archive.archiveConfirmation : undefined;
  const inlineArchiveConfirmation = isMobile || isFinePointer ? undefined : archiveConfirmation;
  const { moveOptions, contextMenuProps } = useTaskRowMoveMenu(props, openedNestSources, closeMenu);
  const nativeUnlinkEntries = buildTaskRowUnlinkEntries(
    prUnlinkMenu,
    (choice) =>
      t("github:removeFromTask", {
        repo: choice.owner ? `${choice.owner}/${choice.repo}` : choice.repo,
        prnumber: choice.number,
      }),
    t("github:loadingPullRequests"),
  );

  return (
    <>
      <ContextMenu key={menuKey} onOpenChange={handleOpenChange}>
        <ContextMenuTrigger asChild>
          <div ref={archive.archiveAnchorRef} tabIndex={-1} {...triggerProps}>
            {cloneWithMenuOpen(children, contextOpen, inlineArchiveConfirmation)}
            {(isMobile || isFinePointer) && archiveConfirmation}
          </div>
        </ContextMenuTrigger>
        <ContextMenuContent
          onCloseAutoFocus={archive.handleMenuCloseAutoFocus}
          className="w-48"
          onInteractOutside={keepWorkflowMoveOpen}
          onMouseDown={stopMenuEvent}
          onPointerDown={stopMenuEvent}
          onTouchStart={stopMenuEvent}
          onClick={stopMenuEvent}
        >
          <TaskContextMenuItems
            {...contextMenuProps}
            nativeUnlinkEntries={nativeUnlinkEntries}
            onArchiveTask={archive.requestArchive}
            onChangeWorkflow={createChangeWorkflowOpenHandler(
              task.id,
              taskIdRef,
              closeMenu,
              setChangeWorkflowOpen,
            )}
          />
        </ContextMenuContent>
      </ContextMenu>
      <TaskMoveOptionsSurface
        step={moveOptions.moveOptionsStep}
        isMoving={moveOptions.isMoving}
        onClose={moveOptions.closeMoveOptions}
        onSubmit={moveOptions.submitMoveOptions}
      />
      <ChangeWorkflowDialog
        open={changeWorkflowOpen}
        onOpenChange={setChangeWorkflowOpen}
        taskId={task.id}
        workspaceId={task.workspaceId ?? null}
        focusReturnRef={changeWorkflowTriggerRef}
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
  nativeUnlinkEntries?: KanbanCardMenuEntry[];
  onChangeWorkflow?: () => void;
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
