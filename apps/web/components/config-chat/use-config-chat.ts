"use client";

import { useCallback, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateWorkspaceAction } from "@/app/actions/workspaces";
import { startConfigChat } from "@/lib/api/domains/workspace-api";
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

export function useConfigChat(workspaceId: string) {
  const store = useConfigChatStore();
  const storeApi = useAppStoreApi();
  const updateWorkspaceInStore = useUpdateWorkspaceInStore();
  const [isStarting, setIsStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pendingPrompt, setPendingPrompt] = useState<PendingConfigPrompt | null>(null);
  const workspace = useAppStore(
    (state) => state.workspaces.items.find((item) => item.id === workspaceId) ?? null,
  );
  const defaultProfileId =
    workspace?.default_config_agent_profile_id ?? workspace?.default_agent_profile_id ?? undefined;

  const startSession = useCallback(
    async (agentProfileId: string, prompt: string) => {
      if (isStarting) return;
      setIsStarting(true);
      setError(null);
      try {
        const profile = storeApi
          .getState()
          .agentProfiles.items.find((item) => item.id === agentProfileId);
        const isPassthrough = profile?.cli_passthrough === true;
        // ACP chats send through the subscribed shell so a fast turn cannot
        // finish before WS attaches. Passthrough chats render only a terminal.
        const response = await startConfigChat(workspaceId, {
          agent_profile_id: agentProfileId,
          ...(isPassthrough ? { prompt } : {}),
        });
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
        store.closeQuickChatSession("");
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
        setError(err instanceof Error ? err.message : "Unknown error");
      } finally {
        setIsStarting(false);
      }
    },
    [
      isStarting,
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
    startSession,
    clearPendingPrompt,
  };
}
