"use client";

import { useCallback, useEffect, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { TaskActionsMenuTrigger } from "@/components/task/task-actions-menu-trigger";
import { TaskActionsMenuDialogs } from "@/components/task/task-actions-menu-dialogs";
import { useTaskActionsMenu, type TaskActionsMenuBoardRow } from "@/hooks/use-task-actions-menu";
import { useArchiveAndSwitchTask, useDeleteAndSwitchTask } from "@/hooks/use-task-actions";

/**
 * The detail surface has no board of its own to lean on, so a workflow move
 * (Move to / Send to workflow) landing the subject in a workflow this client
 * has never tracked would otherwise leave it absent from every board
 * collection even though it still exists. Subscribing to the task directly
 * guarantees its `task.updated` reaches this client regardless of workflow
 * tracking, and the resulting cache update (removal from the old workflow,
 * placeholder-snapshot insertion into the new one) lands in the same state
 * transition, so `existsInBoard` below never observes a false gap for it.
 */
function useSubscribeToTask(taskId: string | null) {
  useEffect(() => {
    if (!taskId) return;
    const client = getWebSocketClient();
    if (!client) return;
    const unsubscribe = client.subscribe(taskId);
    return () => {
      unsubscribe();
    };
  }, [taskId]);
}

/**
 * Closes the menu only when the subject is genuinely gone -- deleted, or
 * archived by another actor (AC-TASKS-TASK-ACTIONS-MENU-004.5). This
 * deliberately listens for the task's own lifecycle events rather than
 * inferring removal from board-collection presence: a task's board row can
 * also disappear while the task is still alive (e.g. a move to a workflow
 * this client has not tracked as a snapshot yet), which is the unresolved-row
 * case AC-TASKS-TASK-ACTIONS-MENU-002.6/004.1c governs and must demote
 * entries in place rather than close the menu. Board presence and "genuinely
 * gone" happen to coincide for an ordinary delete/archive, but they are not
 * the same predicate, so this hook never reads board state at all. Also
 * resets any pending confirm/link dialog: a subject that is gone must not
 * leave a stale, still-actionable confirmation open
 * (AC-TASKS-TASK-ACTIONS-MENU-004.5a's no-retarget rule applies here too).
 */
function useCloseMenuOnTaskGone(
  taskId: string | null,
  setOpen: (open: boolean) => void,
  closeDialogs: () => void,
) {
  useSubscribeToTask(taskId);
  useEffect(() => {
    if (!taskId) return;
    const client = getWebSocketClient();
    if (!client) return;
    const close = () => {
      setOpen(false);
      closeDialogs();
    };
    const unsubscribeDeleted = client.on("task.deleted", (message) => {
      if (message.payload.task_id === taskId) close();
    });
    const unsubscribeUpdated = client.on("task.updated", (message) => {
      if (message.payload.task_id === taskId && message.payload.archived_at) close();
    });
    return () => {
      unsubscribeDeleted();
      unsubscribeUpdated();
    };
  }, [taskId, setOpen, closeDialogs]);
}

type TaskTopBarActionsMenuProps = {
  /** The subject's own identifier/title, independent of `boardRow`: known as
   * soon as the detail page has loaded the task, even when the board row is
   * unresolvable, so the identifier-only tier (AC-TASKS-TASK-ACTIONS-MENU-002.5)
   * stays reachable instead of the trigger disappearing entirely. */
  taskId: string | null;
  taskTitle: string;
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
  taskId,
  taskTitle,
  boardRow,
  workspaceId,
  isArchived,
}: TaskTopBarActionsMenuProps) {
  const [open, setOpen] = useState(false);
  const archiveAndSwitch = useArchiveAndSwitchTask();
  const deleteAndSwitch = useDeleteAndSwitchTask();
  const archiving = useTrackedSwitchAfterAction(archiveAndSwitch);
  const deleting = useTrackedSwitchAfterAction(deleteAndSwitch);

  const menu = useTaskActionsMenu({
    taskId,
    taskTitle,
    workspaceId,
    isArchived: Boolean(isArchived),
    boardRow,
    isArchiving: archiving.pending,
    isDeleting: deleting.pending,
    onArchive: (opts) => (taskId ? archiving.invoke(taskId, opts) : undefined),
    onDelete: (opts) => (taskId ? deleting.invoke(taskId, opts) : undefined),
  });
  useCloseMenuOnTaskGone(taskId, setOpen, menu.closeDialogs);

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
        taskTitle={taskTitle}
        workspaceId={workspaceId}
        boardRow={boardRow}
        isArchiving={archiving.pending}
        isDeleting={deleting.pending}
        menu={menu}
      />
    </>
  );
}
