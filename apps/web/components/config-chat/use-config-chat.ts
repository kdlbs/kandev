"use client";

import { useCallback, useRef, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateWorkspaceAction } from "@/app/actions/workspaces";
import { startConfigChat } from "@/lib/api/domains/workspace-api";
import { getQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";
import {
  agentProfileId as toAgentProfileId,
  sessionId as toSessionId,
  taskId as toTaskId,
} from "@/lib/types/ids";

type PendingConfigPrompt = {
  sessionId: string;
  prompt: string;
};

function useConfigChatStore() {
  return useAppStore(
    useShallow((state) => ({
      openQuickChat: state.openQuickChat,
      closeQuickChatSession: state.closeQuickChatSession,
      renameQuickChatSession: state.renameQuickChatSession,
    })),
  );
}

function useUpdateWorkspaceInStore() {
  const storeApi = useAppStoreApi();
  return useCallback(
    (workspaceId: string, updates: Record<string, unknown>) => {
      const { workspaces, setWorkspaces } = storeApi.getState();
      setWorkspaces(workspaces.items.map((w) => (w.id === workspaceId ? { ...w, ...updates } : w)));
    },
    [storeApi],
  );
}

async function deleteSupersededConfigChatTask(taskId: string) {
  const { deleteTask } = await import("@/lib/api/domains/kanban-api");
  deleteTask(taskId).catch((error) =>
    console.error("Failed to clean up superseded config chat task:", error),
  );
}

export function useConfigChat(workspaceId: string) {
  const store = useConfigChatStore();
  const storeApi = useAppStoreApi();
  const updateWorkspaceInStore = useUpdateWorkspaceInStore();
  const [isStarting, setIsStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pendingPrompt, setPendingPrompt] = useState<PendingConfigPrompt | null>(null);
  const latestRequestId = useRef(0);
  const activeRequestId = useRef<number | null>(null);
  const workspace = useAppStore(
    (state) => state.workspaces.items.find((item) => item.id === workspaceId) ?? null,
  );
  const defaultProfileId =
    workspace?.default_config_agent_profile_id ?? workspace?.default_agent_profile_id ?? undefined;

  const reset = useCallback(() => {
    latestRequestId.current += 1;
    activeRequestId.current = null;
    setIsStarting(false);
    setError(null);
  }, []);

  const startSession = useCallback(
    async (agentProfileId: string, prompt: string) => {
      if (activeRequestId.current !== null) return;
      const profile = storeApi
        .getState()
        .agentProfiles.items.find((item) => item.id === agentProfileId);
      if (!profile) {
        setError("The selected agent profile is not available yet. Try again shortly.");
        return;
      }
      const requestId = ++latestRequestId.current;
      activeRequestId.current = requestId;
      setIsStarting(true);
      setError(null);
      try {
        const isPassthrough = profile.cli_passthrough === true;
        // ACP chats send through the subscribed shell so a fast turn cannot
        // finish before WS attaches. Passthrough chats render only a terminal.
        const response = await startConfigChat(workspaceId, {
          agent_profile_id: agentProfileId,
          ...(isPassthrough ? { prompt } : {}),
        });
        if (latestRequestId.current !== requestId) {
          await deleteSupersededConfigChatTask(response.task_id);
          return;
        }
        const now = new Date().toISOString();
        storeApi.getState().setTaskSession({
          id: toSessionId(response.session_id),
          task_id: toTaskId(response.task_id),
          state: "CREATED",
          started_at: now,
          updated_at: now,
          agent_profile_id: toAgentProfileId(agentProfileId),
        });

        setPendingPrompt(isPassthrough ? null : { sessionId: response.session_id, prompt });
        store.closeQuickChatSession(getQuickChatSetupSessionId(workspaceId, "config"));
        store.openQuickChat(response.session_id, workspaceId, agentProfileId, "config");
        store.renameQuickChatSession(response.session_id, prompt.slice(0, 40) || "Config Chat");

        if (!workspace?.default_config_agent_profile_id) {
          try {
            await updateWorkspaceAction(workspaceId, {
              default_config_agent_profile_id: agentProfileId,
            });
            updateWorkspaceInStore(workspaceId, {
              default_config_agent_profile_id: agentProfileId,
            });
          } catch {
            // The chat is usable even when saving the future default fails.
          }
        }
      } catch (err) {
        if (latestRequestId.current !== requestId) return;
        setError(err instanceof Error ? err.message : "Unknown error");
      } finally {
        if (latestRequestId.current === requestId) {
          activeRequestId.current = null;
          setIsStarting(false);
        }
      }
    },
    [
      store,
      storeApi,
      updateWorkspaceInStore,
      workspace?.default_config_agent_profile_id,
      workspaceId,
    ],
  );

  const clearPendingPrompt = useCallback((sessionId: string) => {
    setPendingPrompt((current) => (current?.sessionId === sessionId ? null : current));
  }, []);

  return {
    isStarting,
    error,
    defaultProfileId,
    pendingPrompt,
    reset,
    startSession,
    clearPendingPrompt,
  };
}
