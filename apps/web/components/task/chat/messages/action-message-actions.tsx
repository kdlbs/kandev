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
import { useArchiveAndSwitchTask } from "@/hooks/use-task-actions";
import { useTaskRemoval } from "@/hooks/use-task-removal";
import { deleteTask } from "@/lib/api/domains/kanban-api";
import type { MessageAction } from "@/components/task/chat/types";

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

export function ActionButton({
  action,
  messageTaskId,
  onCompleted,
  compact = false,
  labelOverride,
}: {
  action: MessageAction;
  messageTaskId?: string;
  onCompleted?: () => void;
  compact?: boolean;
  labelOverride?: string;
}): ReactElement | null {
  const [state, setState] = useState<"idle" | "busy" | "done" | "error">("idle");
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const taskId = messageTaskId || activeTaskId;
  const store = useAppStoreApi();
  const archiveAndSwitch = useArchiveAndSwitchTask();
  const { removeTaskFromBoard } = useTaskRemoval({ store });

  const execute = useCallback(async () => {
    if (state === "busy") return;
    setState("busy");
    try {
      switch (action.type) {
        case "archive_task": {
          if (taskId) await archiveAndSwitch(taskId);
          break;
        }
        case "delete_task": {
          if (taskId) {
            const { activeTaskId, activeSessionId } = store.getState().tasks;
            await deleteTask(taskId);
            await removeTaskFromBoard(taskId, {
              wasActiveTaskId: activeTaskId,
              wasActiveSessionId: activeSessionId,
            });
          }
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
  }, [action, state, taskId, store, archiveAndSwitch, removeTaskFromBoard, onCompleted]);

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
