"use client";

import { useState, useCallback, memo, type ReactElement } from "react";
import {
  IconAlertTriangle,
  IconArchive,
  IconTrash,
  IconRefresh,
  IconPlayerPlay,
  IconSparkles,
  IconGitCommit,
} from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useArchiveAndSwitchTask } from "@/hooks/use-task-actions";
import { useTaskRemoval } from "@/hooks/use-task-removal";
import { deleteTask } from "@/lib/api/domains/kanban-api";
import { AuthMethodsPanel, GenericAuthPanel } from "./auth-methods-panel";
import type { Message, TaskSessionState } from "@/lib/types/http";
import type { MessageAction, RecoveryAuthMethod } from "@/components/task/chat/types";

const ICON_MAP: Record<string, React.ElementType> = {
  archive: IconArchive,
  trash: IconTrash,
  refresh: IconRefresh,
  "player-play": IconPlayerPlay,
  sparkles: IconSparkles,
  "git-commit": IconGitCommit,
  "alert-triangle": IconAlertTriangle,
};

type ActionMeta = {
  actions?: MessageAction[];
  variant?: string;
  is_auth_error?: boolean;
  auth_methods?: RecoveryAuthMethod[];
  error_output?: string;
};

function isSessionActive(state?: TaskSessionState) {
  return state === "RUNNING" || state === "STARTING" || state === "COMPLETED";
}

export const ActionMessage = memo(function ActionMessage({
  comment,
  sessionState,
}: {
  comment: Message;
  sessionState?: TaskSessionState;
}) {
  const metadata = comment.metadata as ActionMeta | undefined;
  const isWarning = metadata?.variant === "warning";
  const message = comment.content || "An error occurred";

  // Hide once session is active again (recovery succeeded)
  if (isSessionActive(sessionState)) return null;

  const iconClass = isWarning ? "text-amber-500" : "text-red-500";
  const textClass = isWarning
    ? "text-amber-600 dark:text-amber-400"
    : "text-red-600 dark:text-red-400";

  return (
    <div className="w-full">
      <div className="flex items-start gap-3 w-full rounded px-2 py-1 -mx-2">
        <div className="flex-shrink-0 mt-0.5">
          <IconAlertTriangle className={cn("h-4 w-4", iconClass)} />
        </div>
        <div className="flex-1 min-w-0 pt-0.5">
          <div className={cn("text-xs font-mono", textClass)}>{message}</div>
          <ActionMessageDetails metadata={metadata} />
          {metadata?.actions && metadata.actions.length > 0 && (
            <ActionButtons actions={metadata.actions} taskId={comment.task_id} />
          )}
        </div>
      </div>
    </div>
  );
});

function ActionMessageDetails({ metadata }: { metadata: ActionMeta | undefined }) {
  const store = useAppStoreApi();
  const openTerminalWithCommand = useCallback(
    (command: string) => store.getState().openBottomTerminalWithCommand(command),
    [store],
  );
  const openBottomTerminal = useCallback(() => {
    if (!store.getState().bottomTerminal.isOpen) store.getState().toggleBottomTerminal();
  }, [store]);

  if (!metadata) return null;
  return (
    <>
      {metadata.error_output && (
        <pre className="mt-1.5 text-[11px] font-mono text-muted-foreground bg-muted/50 rounded p-2 overflow-auto max-h-[300px] whitespace-pre-wrap break-words">
          {metadata.error_output}
        </pre>
      )}
      {metadata.is_auth_error && metadata.auth_methods && metadata.auth_methods.length > 0 && (
        <AuthMethodsPanel
          methods={metadata.auth_methods}
          onOpenTerminal={openTerminalWithCommand}
        />
      )}
      {metadata.is_auth_error && (!metadata.auth_methods || metadata.auth_methods.length === 0) && (
        <GenericAuthPanel onOpenTerminal={openBottomTerminal} />
      )}
    </>
  );
}

function ActionButtons({ actions, taskId }: { actions: MessageAction[]; taskId?: string }) {
  return (
    <div className="mt-2 flex items-center gap-2">
      {actions.map((action, i) => (
        <ActionButton key={action.test_id ?? i} action={action} messageTaskId={taskId} />
      ))}
    </div>
  );
}

function ActionButton({
  action,
  messageTaskId,
}: {
  action: MessageAction;
  messageTaskId?: string;
}): ReactElement {
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
          if (client && params) await client.request(params.method, params.payload);
          break;
        }
      }
      setState("done");
    } catch {
      setState("error");
      setTimeout(() => setState("idle"), 3000);
    }
  }, [action, state, taskId, store, archiveAndSwitch, removeTaskFromBoard]);

  const Icon = action.icon ? ICON_MAP[action.icon] : null;
  const disabled = state === "busy" || state === "done";
  const isDestructive = action.variant === "destructive";
  const label =
    state === "done" && action.type === "ws_request" ? `${action.label} requested` : action.label;

  const button = (
    <Button
      variant="outline"
      size="sm"
      className={cn(
        "h-7 text-xs cursor-pointer gap-1.5",
        isDestructive && "text-destructive hover:text-destructive",
      )}
      disabled={disabled}
      onClick={execute}
      data-testid={action.test_id}
    >
      {Icon && <Icon className="h-3 w-3" />}
      {label}
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
