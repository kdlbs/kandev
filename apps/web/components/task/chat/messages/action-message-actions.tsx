"use client";

import { useCallback, useState, type ElementType, type ReactElement } from "react";
import {
  IconAlertTriangle,
  IconArchive,
  IconGitCommit,
  IconPlayerPlay,
  IconRefresh,
  IconSparkles,
  IconTrash,
  IconX,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import { useArchiveAndSwitchTask, useTaskActions } from "@/hooks/use-task-actions";
import { useTaskRemoval } from "@/hooks/use-task-removal";
import type { MessageAction } from "@/components/task/chat/types";
import { TaskDeleteConfirmDialog } from "@/components/task/task-delete-confirm-dialog";

export const ACTION_ICON_MAP: Record<string, ElementType> = {
  archive: IconArchive,
  trash: IconTrash,
  refresh: IconRefresh,
  "player-play": IconPlayerPlay,
  sparkles: IconSparkles,
  "git-commit": IconGitCommit,
  "alert-triangle": IconAlertTriangle,
  x: IconX,
};

export function ActionButtons({
  actions,
  taskId,
  onRecoveryRequested,
  compact = false,
  labelOverride,
}: {
  actions: MessageAction[];
  taskId?: string;
  onRecoveryRequested?: () => void;
  compact?: boolean;
  labelOverride?: string;
}) {
  return (
    <div
      className={cn(
        compact
          ? "flex shrink-0 items-center"
          : "mt-2 flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center",
      )}
    >
      {actions.map((action, i) => (
        <ActionButton
          key={action.test_id ?? i}
          action={action}
          messageTaskId={taskId}
          onCompleted={onRecoveryRequested}
          compact={compact}
          labelOverride={labelOverride}
        />
      ))}
    </div>
  );
}

type ActionButtonProps = {
  action: MessageAction;
  messageTaskId?: string;
  onCompleted?: () => void;
  compact?: boolean;
  labelOverride?: string;
};

export function ActionButton(props: ActionButtonProps): ReactElement | null {
  if (props.action.type === "delete_task") {
    return <DeleteActionButton {...(props as DeleteActionButtonProps)} />;
  }
  return <StandardActionButton {...(props as StandardActionButtonProps)} />;
}

type StandardActionButtonProps = Omit<ActionButtonProps, "action"> & {
  action: MessageAction & { type: "archive_task" | "ws_request" };
};

function StandardActionButton({
  action,
  messageTaskId,
  onCompleted,
  compact = false,
  labelOverride,
}: StandardActionButtonProps): ReactElement | null {
  const [state, setState] = useState<"idle" | "busy" | "done" | "error">("idle");
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const taskId = messageTaskId || activeTaskId;
  const archiveAndSwitch = useArchiveAndSwitchTask();

  const execute = useCallback(async () => {
    if (state === "busy") return;
    setState("busy");
    try {
      switch (action.type) {
        case "archive_task": {
          if (taskId) await archiveAndSwitch(taskId);
          break;
        }
        case "ws_request": {
          const client = getWebSocketClient();
          const params = action.params as
            | { method: string; payload: Record<string, unknown> }
            | undefined;
          // i18n-exempt: technical error for unavailable WebSocket recovery client.
          if (!client || !params) throw new Error("WebSocket recovery request is unavailable");
          await client.request(params.method, params.payload);
          break;
        }
      }
      setState("done");
      if (action.type === "ws_request") onCompleted?.();
    } catch {
      setState("error");
      setTimeout(() => setState("idle"), 3000);
    }
  }, [action, state, taskId, archiveAndSwitch, onCompleted]);

  // Once a ws_request has been fired, hide this button: it's no longer
  // actionable. If the recovery succeeds the whole ActionMessage unmounts via
  // isSessionActive; if it fails, a newer status/error message renders fresh
  // buttons, so this stale one would just confuse the user.
  if (state === "done" && action.type === "ws_request") return null;

  const Icon = action.icon ? ACTION_ICON_MAP[action.icon] : null;
  const disabled = state === "busy" || state === "done";
  const isDestructive = action.variant === "destructive";

  const button = (
    <Button
      variant={compact ? "ghost" : "outline"}
      size="sm"
      className={cn(
        compact
          ? "h-auto min-h-11 shrink-0 px-2 text-xs cursor-pointer sm:min-h-8"
          : "h-auto min-h-11 w-full gap-1.5 text-xs cursor-pointer sm:min-h-8 sm:w-auto",
        isDestructive && "text-destructive hover:text-destructive",
      )}
      disabled={disabled}
      onClick={execute}
      data-testid={action.test_id}
    >
      {Icon && <Icon className="h-3 w-3" />}
      {labelOverride ?? action.label}
    </Button>
  );

  if (action.tooltip) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>{button}</TooltipTrigger>
        <TooltipContent side="top">{action.tooltip}</TooltipContent>
      </Tooltip>
    );
  }
  return button;
}

type DeleteActionButtonProps = Omit<ActionButtonProps, "action"> & {
  action: MessageAction & { type: "delete_task" };
};

function DeleteActionButton({
  action,
  messageTaskId,
  compact = false,
  labelOverride,
}: DeleteActionButtonProps): ReactElement {
  const [state, setState] = useState<"idle" | "busy" | "done" | "error">("idle");
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const taskId = messageTaskId || activeTaskId;
  const messageTask = useAppStore((s) =>
    taskId ? findTaskInSnapshots(taskId, s.kanbanMulti.snapshots, s.kanban.tasks) : null,
  );
  const store = useAppStoreApi();
  const { deleteTaskById } = useTaskActions();
  const { removeTaskFromBoard } = useTaskRemoval({ store });

  const handleDeleteConfirm = useCallback(
    async ({
      cascade,
      discardWorktreeChanges,
    }: {
      cascade: boolean;
      discardWorktreeChanges: boolean;
    }) => {
      if (!taskId || state === "busy") return;
      setState("busy");
      try {
        const { activeTaskId, activeSessionId } = store.getState().tasks;
        await deleteTaskById(taskId, { cascade, discardWorktreeChanges });
        await removeTaskFromBoard(taskId, {
          wasActiveTaskId: activeTaskId,
          wasActiveSessionId: activeSessionId,
        });
        setState("done");
      } catch {
        setState("error");
        setTimeout(() => setState("idle"), 3000);
      }
    },
    [state, taskId, store, deleteTaskById, removeTaskFromBoard],
  );

  const execute = useCallback(() => {
    if (state !== "busy") setDeleteDialogOpen(true);
  }, [state]);

  const Icon = action.icon ? ACTION_ICON_MAP[action.icon] : null;
  const disabled = state === "busy" || state === "done";
  const button = (
    <Button
      variant={compact ? "ghost" : "outline"}
      size="sm"
      className={cn(
        compact
          ? "h-auto min-h-11 shrink-0 px-2 text-xs cursor-pointer sm:min-h-8"
          : "h-auto min-h-11 w-full gap-1.5 text-xs cursor-pointer sm:min-h-8 sm:w-auto",
        "text-destructive hover:text-destructive",
      )}
      disabled={disabled}
      onClick={execute}
      data-testid={action.test_id}
    >
      {Icon && <Icon className="h-3 w-3" />}
      {labelOverride ?? action.label}
    </Button>
  );
  const dialog = (
    <TaskDeleteConfirmDialog
      open={deleteDialogOpen}
      onOpenChange={setDeleteDialogOpen}
      taskTitle={messageTask?.title}
      taskId={taskId ?? undefined}
      executorType={messageTask?.primaryExecutorType}
      requireDiscardConsent={messageTask?.primaryExecutorType == null}
      isDeleting={state === "busy"}
      onConfirm={(opts) => void handleDeleteConfirm(opts)}
    />
  );

  if (action.tooltip) {
    return (
      <>
        <Tooltip>
          <TooltipTrigger asChild>{button}</TooltipTrigger>
          <TooltipContent side="top">{action.tooltip}</TooltipContent>
        </Tooltip>
        {dialog}
      </>
    );
  }
  return (
    <>
      {button}
      {dialog}
    </>
  );
}
