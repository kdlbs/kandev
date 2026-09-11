"use client";

import { cloneElement, isValidElement, useRef, useState, type ReactNode } from "react";
import { ContextMenu, ContextMenuContent, ContextMenuTrigger } from "@kandev/ui/context-menu";
import type { TaskMoveWorkflow } from "@/components/task/task-move-context-menu";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { TaskLinkHandlers } from "./task-switcher-link-menu";
import type { StepDef, TaskSwitcherItem } from "./task-switcher-types";
import { useTaskSwitcherArchiveConfirmation } from "./task-switcher-archive-confirmation";
import {
  BulkSelectionMenuItems,
  SingleSelectionMenuItems,
} from "./task-switcher-context-menu-items";
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
  const { isFinePointer } = useResponsiveBreakpoint();
  const archive = useTaskSwitcherArchiveConfirmation({
    task: menuProps.task,
    onArchiveTask: menuProps.onArchiveTask,
    isArchiving: menuProps.isArchiving,
    closeMenu,
  });
  const archiveConfirmation = archive.archiveOpen ? archive.archiveConfirmation : undefined;
  const inlineArchiveConfirmation = isFinePointer ? undefined : archiveConfirmation;
  const portaledArchiveConfirmation = isFinePointer ? archiveConfirmation : undefined;

  return (
    <ContextMenu key={menuKey} onOpenChange={handleOpenChange}>
      <ContextMenuTrigger asChild>
        <div ref={archive.archiveAnchorRef} tabIndex={-1} {...triggerProps}>
          {cloneWithMenuOpen(children, contextOpen, inlineArchiveConfirmation)}
          {portaledArchiveConfirmation}
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

export type TaskContextMenuItemsProps = Omit<ContextMenuProps, "children"> & {
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

function cloneWithMenuOpen(
  children: React.ReactElement<{ menuOpen?: boolean; archiveConfirmation?: ReactNode }>,
  menuOpen: boolean,
  archiveConfirmation?: ReactNode,
): React.ReactNode {
  if (isValidElement(children)) return cloneElement(children, { menuOpen, archiveConfirmation });
  return children;
}
