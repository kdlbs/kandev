"use client";

import { useCallback, useEffect } from "react";
import { DockviewDefaultTab, type IDockviewPanelHeaderProps } from "dockview-react";
import { IconStar } from "@tabler/icons-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
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
    const taskId = state.tasks.activeTaskId;
    if (!taskId || !sessionId) return false;
    const task = state.kanban.tasks.find((t: { id: string }) => t.id === taskId);
    return task?.primarySessionId === sessionId;
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
  return { isPrimary, sessionState, taskId, agentLabel };
}

function useSessionTabActions(
  sessionId: string | undefined,
  taskId: string | null,
  api: IDockviewPanelHeaderProps["api"],
  containerApi: IDockviewPanelHeaderProps["containerApi"],
) {
  const { toast, updateToast } = useToast();

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
    if (!sessionId) return;
    await wsAction("session.delete", "Deleting session", { session_id: sessionId });
    const panel = containerApi.getPanel(api.id);
    if (panel) containerApi.removePanel(panel);
  }, [sessionId, wsAction, api.id, containerApi]);
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
  const { isPrimary, sessionState, taskId, agentLabel } = useSessionTabState(sessionId);
  const actions = useSessionTabActions(sessionId, taskId, api, containerApi);

  useEffect(() => {
    if (agentLabel && api.title !== agentLabel) api.setTitle(agentLabel);
  }, [agentLabel, api]);

  return (
    <ContextMenu>
      <ContextMenuTrigger className="flex h-full items-center">
        <div className="flex items-center gap-1 px-1">
          {isPrimary && (
            <IconStar className="h-3 w-3 text-amber-500 fill-amber-500 shrink-0" />
          )}
          <DockviewDefaultTab {...props} />
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent>
        <ContextMenuItem className="cursor-pointer" onSelect={actions.handleSetPrimary} disabled={isPrimary}>
          Set as Primary
        </ContextMenuItem>
        <ContextMenuSeparator />
        {sessionState && isStoppable(sessionState) && (
          <ContextMenuItem className="cursor-pointer" onSelect={actions.handleStop}>Stop</ContextMenuItem>
        )}
        {sessionState && isResumable(sessionState) && (
          <ContextMenuItem className="cursor-pointer" onSelect={actions.handleResume}>Resume</ContextMenuItem>
        )}
        {sessionState && isDeletable(sessionState) && (
          <ContextMenuItem className="cursor-pointer text-destructive" onSelect={actions.handleDelete}>
            Delete
          </ContextMenuItem>
        )}
        <ContextMenuSeparator />
        <ContextMenuItem className="cursor-pointer" onSelect={actions.handleCloseOthers}>
          Close Others
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  );
}
