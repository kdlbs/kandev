"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import { TaskActionsMenuTrigger } from "@/components/task/task-actions-menu-trigger";
import { TaskActionsMenuDialogs } from "@/components/task/task-actions-menu-dialogs";
import { useTaskActionsMenu, type TaskActionsMenuBoardRow } from "@/hooks/use-task-actions-menu";
import { useArchiveAndSwitchTask, useDeleteAndSwitchTask } from "@/hooks/use-task-actions";

/**
 * Closes the menu when the subject leaves the board's task collections
 * (AC-TASKS-TASK-ACTIONS-MENU-004.5): the detail route's own task record can
 * outlive a peer's delete (it is seeded once from the initial page load and
 * has no WS-driven unmount for this case), so the board's live collections,
 * not the page's own task prop, are the observation this criterion names.
 * Guarded by "seen present, now absent" rather than "currently absent" so a
 * task that simply has not loaded into the board yet never falsely closes
 * the menu.
 */
function useCloseMenuOnTaskRemoved(taskId: string | null, setOpen: (open: boolean) => void) {
  const existsInBoard = useAppStore((state) => {
    if (!taskId) return false;
    if (state.kanban.tasks.some((task) => task.id === taskId)) return true;
    return findTaskInSnapshots(taskId, state.kanbanMulti.snapshots) != null;
  });
  const seenRef = useRef(false);
  useEffect(() => {
    if (existsInBoard) {
      seenRef.current = true;
      return;
    }
    if (seenRef.current) setOpen(false);
  }, [existsInBoard, setOpen]);
}

type TaskTopBarActionsMenuProps = {
  boardRow: TaskActionsMenuBoardRow | null;
  workspaceId: string | null;
  isArchived?: boolean;
};

/** Tracks in-flight state for a one-shot switch-after-action call, since
 * `useArchiveAndSwitchTask`/`useDeleteAndSwitchTask` carry no pending flag of
 * their own (unlike `useTaskCRUD`, which the board and preview surfaces use). */
function useTrackedSwitchAfterAction(
  run: (taskId: string, opts?: { cascade?: boolean }) => Promise<unknown>,
) {
  const [pending, setPending] = useState(false);
  const invoke = useCallback(
    async (taskId: string, opts: { cascade: boolean }) => {
      setPending(true);
      try {
        await run(taskId, opts);
      } finally {
        setPending(false);
      }
    },
    [run],
  );
  return { pending, invoke };
}

/**
 * Detail top bar's "More options" trigger. Unlike the board and preview
 * surfaces, the detail surface has no board subscription to prune
 * optimistically, so a confirmed Archive or Delete uses the same
 * archive-and-switch / switch-to-next-task outcome as every other
 * detail-surface entry point (AC-TASKS-TASK-ACTIONS-MENU-003.4/003.5).
 */
export function TaskTopBarActionsMenu({
  boardRow,
  workspaceId,
  isArchived,
}: TaskTopBarActionsMenuProps) {
  const [open, setOpen] = useState(false);
  const archiveAndSwitch = useArchiveAndSwitchTask();
  const deleteAndSwitch = useDeleteAndSwitchTask();
  const archiving = useTrackedSwitchAfterAction(archiveAndSwitch);
  const deleting = useTrackedSwitchAfterAction(deleteAndSwitch);

  const taskId = boardRow?.id ?? null;
  useCloseMenuOnTaskRemoved(taskId, setOpen);
  const menu = useTaskActionsMenu({
    taskId,
    taskTitle: boardRow?.title ?? "",
    workspaceId,
    isArchived: Boolean(isArchived),
    boardRow,
    isArchiving: archiving.pending,
    isDeleting: deleting.pending,
    onArchive: (opts) => (taskId ? archiving.invoke(taskId, opts) : undefined),
    onDelete: (opts) => (taskId ? deleting.invoke(taskId, opts) : undefined),
  });

  if (!taskId) return null;

  return (
    <>
      <TaskActionsMenuTrigger
        entries={menu.entries}
        testId="task-topbar-actions-menu"
        triggerRef={menu.triggerRef}
        open={open}
        onOpenChange={setOpen}
      />
      <TaskActionsMenuDialogs
        taskId={taskId}
        taskTitle={boardRow?.title ?? ""}
        workspaceId={workspaceId}
        boardRow={boardRow}
        isArchiving={archiving.pending}
        isDeleting={deleting.pending}
        menu={menu}
      />
    </>
  );
}
