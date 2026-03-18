"use client";

import { useCallback, useEffect, useState } from "react";
import { DockviewDefaultTab, type IDockviewPanelHeaderProps } from "dockview-react";
import { IconStar } from "@tabler/icons-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { TaskSessionState } from "@/lib/types/http";

function isStoppable(s: TaskSessionState) {
  return s === "RUNNING" || s === "STARTING" || s === "WAITING_FOR_INPUT";
}
function isDeletable(s: TaskSessionState) {
  return s !== "RUNNING" && s !== "STARTING";
}
function isResumable(s: TaskSessionState) {
  return s === "COMPLETED" || s === "FAILED" || s === "CANCELLED";
}

function useSessionTabState(sessionId: string | undefined) {
  const isPrimary = useAppStore((state) => {
    const activeTaskId = state.tasks.activeTaskId;
    if (!activeTaskId || !sessionId) return false;
    const task = state.kanban.tasks.find((t: { id: string }) => t.id === activeTaskId);
    if (task?.primarySessionId) return task.primarySessionId === sessionId;
    return state.taskSessions.items[sessionId]?.is_primary === true;
  });
  const sessionState = useAppStore((state) => {
    if (!sessionId) return null;
    return state.taskSessions.items[sessionId]?.state ?? null;
  }) as TaskSessionState | null;
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const agentLabel = useAppStore((state) => {
    if (!sessionId) return null;
    const session = state.taskSessions.items[sessionId];
    if (!session?.agent_profile_id) return null;
    const profile = state.agentProfiles.items.find(
      (p: { id: string }) => p.id === session.agent_profile_id,
    );
    return profile?.label ?? null;
  });
  const sessionNumber = useAppStore((state) => {
    if (!sessionId) return null;
    const activeTaskId = state.tasks.activeTaskId;
    const sessions = activeTaskId ? state.taskSessionsByTask.itemsByTaskId[activeTaskId] : null;
    if (!sessions) return null;
    // Sort chronologically (oldest first) so indexes are stable regardless of
    // which session is primary or the backend's default DESC ordering.
    const sorted = [...sessions].sort(
      (a: { started_at: string }, b: { started_at: string }) =>
        new Date(a.started_at).getTime() - new Date(b.started_at).getTime(),
    );
    const idx = sorted.findIndex((s: { id: string }) => s.id === sessionId);
    return idx >= 0 ? idx + 1 : null;
  });
  const sessionCount = useAppStore((state) => {
    const activeTaskId = state.tasks.activeTaskId;
    if (!activeTaskId) return 0;
    return state.taskSessionsByTask.itemsByTaskId[activeTaskId]?.length ?? 0;
  });
  return { isPrimary, sessionState, taskId, agentLabel, sessionNumber, sessionCount };
}

function useSessionTabActions(
  sessionId: string | undefined,
  taskId: string | null,
  api: IDockviewPanelHeaderProps["api"],
  containerApi: IDockviewPanelHeaderProps["containerApi"],
) {
  const { toast, updateToast } = useToast();
  const removeTaskSession = useAppStore((state) => state.removeTaskSession);

  const wsAction = useCallback(
    async (action: string, label: string, payload: Record<string, unknown>, timeout = 15000) => {
      const client = getWebSocketClient();
      if (!client) return;
      const toastId = toast({ title: `${label}...`, variant: "loading" });
      try {
        await client.request(action, payload, timeout);
        updateToast(toastId, { title: `${label} successful`, variant: "success" });
      } catch (error) {
        const msg = error instanceof Error ? error.message : "Unknown error";
        updateToast(toastId, { title: `${label} failed`, description: msg, variant: "error" });
      }
    },
    [toast, updateToast],
  );

  const handleSetPrimary = useCallback(
    () => sessionId && wsAction("session.set_primary", "Set primary", { session_id: sessionId }),
    [sessionId, wsAction],
  );
  const handleStop = useCallback(
    () => sessionId && wsAction("session.stop", "Stopping session", { session_id: sessionId }),
    [sessionId, wsAction],
  );
  const handleResume = useCallback(
    () =>
      sessionId &&
      taskId &&
      wsAction(
        "session.launch",
        "Resuming session",
        { task_id: taskId, intent: "resume", session_id: sessionId },
        30000,
      ),
    [sessionId, taskId, wsAction],
  );
  const handleDelete = useCallback(async () => {
    if (!sessionId || !taskId) return;
    await wsAction("session.delete", "Deleting session", { session_id: sessionId });
    removeTaskSession(taskId, sessionId);
    const panel = containerApi.getPanel(api.id);
    if (panel) containerApi.removePanel(panel);
  }, [sessionId, taskId, wsAction, removeTaskSession, api.id, containerApi]);
  const handleCloseOthers = useCallback(() => {
    const toClose = api.group.panels.filter((p) => p.id !== api.id);
    for (const panel of toClose) containerApi.removePanel(panel);
  }, [api, containerApi]);

  return { handleSetPrimary, handleStop, handleResume, handleDelete, handleCloseOthers };
}

/**
 * Custom dockview tab for session panels.
 * Shows a star icon for primary sessions; right-click context menu for lifecycle actions.
 */
export function SessionTab(props: IDockviewPanelHeaderProps) {
  const { api, containerApi } = props;
  const sessionId = api.id.startsWith("session:") ? api.id.slice("session:".length) : undefined;
  const { isPrimary, sessionState, taskId, agentLabel, sessionNumber, sessionCount } =
    useSessionTabState(sessionId);
  const actions = useSessionTabActions(sessionId, taskId, api, containerApi);
  const [confirmDelete, setConfirmDelete] = useState(false);

  useEffect(() => {
    const label = sessionNumber && agentLabel ? `#${sessionNumber} ${agentLabel}` : agentLabel;
    if (label && api.title !== label) api.setTitle(label);
  }, [agentLabel, sessionNumber, api]);

  return (
    <>
      <ContextMenu>
        <ContextMenuTrigger className="flex h-full items-center">
          <div className="flex items-center">
            {isPrimary && (
              <IconStar className="h-3 w-3 fill-foreground/50 stroke-0 shrink-0 ml-2" />
            )}
            <DockviewDefaultTab {...props} />
          </div>
        </ContextMenuTrigger>
        <ContextMenuContent>
          <ContextMenuItem
            className="cursor-pointer"
            onSelect={actions.handleSetPrimary}
            disabled={isPrimary}
          >
            Set as Primary
          </ContextMenuItem>
          <ContextMenuSeparator />
          {sessionState && isStoppable(sessionState) && (
            <ContextMenuItem className="cursor-pointer" onSelect={actions.handleStop}>
              Stop
            </ContextMenuItem>
          )}
          {sessionState && isResumable(sessionState) && (
            <ContextMenuItem className="cursor-pointer" onSelect={actions.handleResume}>
              Resume
            </ContextMenuItem>
          )}
          {sessionState && isDeletable(sessionState) && (
            <ContextMenuItem
              className="cursor-pointer text-destructive"
              onSelect={() => setConfirmDelete(true)}
            >
              Delete
            </ContextMenuItem>
          )}
          <ContextMenuSeparator />
          <ContextMenuItem className="cursor-pointer" onSelect={actions.handleCloseOthers}>
            Close Others
          </ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>
      <AlertDialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete session?</AlertDialogTitle>
            <AlertDialogDescription asChild>
              <div>
                <p>
                  This will permanently delete this session, including all conversation history.
                </p>
                {isPrimary && sessionCount > 1 && (
                  <p className="mt-2 font-medium">
                    This is the primary session. Another session will be promoted.
                  </p>
                )}
                {sessionCount === 1 && (
                  <p className="mt-2 font-medium">This is the only session for this task.</p>
                )}
              </div>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                setConfirmDelete(false);
                actions.handleDelete();
              }}
              className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
