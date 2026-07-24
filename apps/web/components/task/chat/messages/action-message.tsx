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
  IconX,
  IconChevronDown,
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
import { HostShellDialog } from "@/components/settings/host-shell-dialog";
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
  x: IconX,
};

type ActionMeta = {
  actions?: MessageAction[];
  variant?: string;
  is_auth_error?: boolean;
  auth_methods?: RecoveryAuthMethod[];
  error_output?: string;
  failure_kind?: string;
  missing_branch?: string;
};

function isSessionActive(state?: TaskSessionState) {
  return state === "RUNNING" || state === "STARTING" || state === "COMPLETED";
}

export const ActionMessage = memo(function ActionMessage({ comment }: { comment: Message }) {
  // Read session state from the store instead of receiving it as a prop, so a
  // state transition doesn't re-render every message in the list (only the
  // rare action messages that actually depend on it).
  const sessionState = useAppStore((state) =>
    comment.session_id
      ? (state.taskSessions.items[comment.session_id]?.state ?? undefined)
      : undefined,
  );
  const sessionError = useAppStore((state) =>
    comment.session_id
      ? (state.taskSessions.items[comment.session_id]?.error_message as string | undefined)
      : undefined,
  );
  const metadata = comment.metadata as ActionMeta | undefined;
  const isWarning = metadata?.variant === "warning";
  const message = comment.content || "An error occurred";

  // Hide once session is active again (recovery succeeded)
  if (isSessionActive(sessionState)) return null;

  if (metadata?.failure_kind === "missing_pr_branch") {
    return (
      <MissingBranchRecovery
        metadata={metadata}
        taskId={comment.task_id}
        fallbackMessage={message}
        technicalDetails={sessionError}
      />
    );
  }

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
          <div className={cn("text-xs break-words", textClass)}>{message}</div>
          <ActionMessageDetails metadata={metadata} />
          {metadata?.actions && metadata.actions.length > 0 && (
            <ActionButtons actions={metadata.actions} taskId={comment.task_id} />
          )}
        </div>
      </div>
    </div>
  );
});

function MissingBranchRecovery({
  metadata,
  taskId,
  fallbackMessage,
  technicalDetails,
}: {
  metadata: ActionMeta;
  taskId?: string;
  fallbackMessage: string;
  technicalDetails?: string;
}) {
  const branch = metadata.missing_branch?.trim();
  return (
    <section
      data-testid="missing-branch-recovery"
      role="alert"
      className="w-full min-w-0 rounded-md border border-amber-500/25 bg-amber-500/[0.06] p-3 sm:p-4"
    >
      <div className="flex min-w-0 items-start gap-3">
        <IconAlertTriangle
          className="mt-0.5 h-4 w-4 flex-shrink-0 text-amber-500"
          aria-hidden="true"
        />
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-medium text-foreground">Branch is no longer available</h3>
          <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
            {branch ? (
              <>
                This task points to <code className="break-all text-foreground">{branch}</code>, but
                that branch could not be found on the remote repository. It may have been merged or
                deleted.
              </>
            ) : (
              fallbackMessage
            )}
          </p>
          <ActionMessageDetails metadata={metadata} technicalDetails={technicalDetails} />
          {metadata.actions && metadata.actions.length > 0 && (
            <ActionButtons actions={metadata.actions} taskId={taskId} />
          )}
        </div>
      </div>
    </section>
  );
}

function TechnicalDetails({ children }: { children: string }) {
  return (
    <details className="mt-2 min-w-0 text-xs text-muted-foreground">
      <summary className="flex min-h-11 cursor-pointer list-none items-center gap-1.5 sm:min-h-8">
        <IconChevronDown className="h-3.5 w-3.5" />
        Technical details
      </summary>
      <pre className="max-h-[300px] max-w-full overflow-y-auto whitespace-pre-wrap break-words rounded bg-muted/50 p-2 font-mono text-[11px]">
        {children}
      </pre>
    </details>
  );
}

function ActionMessageDetails({
  metadata,
  technicalDetails,
}: {
  metadata: ActionMeta | undefined;
  technicalDetails?: string;
}) {
  const [hostShellOpen, setHostShellOpen] = useState(false);
  const [hostShellCommand, setHostShellCommand] = useState<string | undefined>(undefined);

  // Auth recovery uses the kandev host shell (where the agent CLIs are
  // installed), not the task environment shell - the task env often isn't
  // ready when an auth error fires (no workspace path yet), and the user's
  // agent auth state lives in their home dir on the host anyway.
  const openHostShellWithCommand = useCallback((command: string) => {
    // Trailing newline runs the command immediately. Drop it if you'd rather
    // let the user review first.
    setHostShellCommand(command + "\n");
    setHostShellOpen(true);
  }, []);
  const openHostShell = useCallback(() => {
    setHostShellCommand(undefined);
    setHostShellOpen(true);
  }, []);

  if (!metadata) return null;
  const errorOutput = metadata.error_output || technicalDetails;
  return (
    <>
      {errorOutput && <TechnicalDetails>{errorOutput}</TechnicalDetails>}
      {metadata.is_auth_error && metadata.auth_methods && metadata.auth_methods.length > 0 && (
        <AuthMethodsPanel
          methods={metadata.auth_methods}
          onOpenTerminal={openHostShellWithCommand}
        />
      )}
      {metadata.is_auth_error && (!metadata.auth_methods || metadata.auth_methods.length === 0) && (
        <GenericAuthPanel onOpenTerminal={openHostShell} />
      )}
      <HostShellDialog
        open={hostShellOpen}
        onOpenChange={setHostShellOpen}
        initialInput={hostShellCommand}
      />
    </>
  );
}

function ActionButtons({ actions, taskId }: { actions: MessageAction[]; taskId?: string }) {
  return (
    <div className="mt-2 flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
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

  // Once a ws_request has been fired, hide this button: it's no longer
  // actionable. If the recovery succeeds the whole ActionMessage unmounts via
  // isSessionActive; if it fails, a newer status/error message renders fresh
  // buttons, so this stale one would just confuse the user.
  if (state === "done" && action.type === "ws_request") return null;

  const Icon = action.icon ? ICON_MAP[action.icon] : null;
  const disabled = state === "busy" || state === "done";
  const isDestructive = action.variant === "destructive";

  const button = (
    <Button
      variant="outline"
      size="sm"
      className={cn(
        "h-auto min-h-11 w-full gap-1.5 text-xs cursor-pointer sm:min-h-8 sm:w-auto",
        isDestructive && "text-destructive hover:text-destructive",
      )}
      disabled={disabled}
      onClick={execute}
      data-testid={action.test_id}
    >
      {Icon && <Icon className="h-3 w-3" />}
      {action.label}
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
