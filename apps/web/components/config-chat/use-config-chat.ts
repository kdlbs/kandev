"use client";

import { useCallback, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { startConfigChat } from "@/lib/api/domains/workspace-api";
import { updateWorkspaceAction } from "@/app/actions/workspaces";

function useConfigChatStore() {
  return useAppStore(
    useShallow((s) => ({
      isOpen: s.configChat.isOpen,
      sessions: s.configChat.sessions,
      activeSessionId: s.configChat.activeSessionId,
      workspaceId: s.configChat.workspaceId,
      openConfigChat: s.openConfigChat,
      openConfigChatModal: s.openConfigChatModal,
      closeConfigChat: s.closeConfigChat,
      closeConfigChatSession: s.closeConfigChatSession,
      setActiveConfigChatSession: s.setActiveConfigChatSession,
      renameConfigChatSession: s.renameConfigChatSession,
    })),
  );
}

function useUpdateWorkspaceInStore() {
  const storeApi = useAppStoreApi();
  return (workspaceId: string, updates: Record<string, unknown>) => {
    const { workspaces, setWorkspaces } = storeApi.getState();
    setWorkspaces(workspaces.items.map((w) => (w.id === workspaceId ? { ...w, ...updates } : w)));
  };
}

export function useConfigChat(workspaceId: string) {
  const store = useConfigChatStore();
  const storeApi = useAppStoreApi();
  const [isStarting, setIsStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pendingPrompt, setPendingPrompt] = useState<string | null>(null);
  const updateWorkspaceInStore = useUpdateWorkspaceInStore();

  const workspace = useAppStore(
    (s) => s.workspaces.items.find((w) => w.id === workspaceId) ?? null,
  );

  const defaultProfileId =
    workspace?.default_config_agent_profile_id ?? workspace?.default_agent_profile_id ?? undefined;

  const open = useCallback(() => {
    if (store.activeSessionId && store.workspaceId === workspaceId) {
      store.openConfigChat(store.activeSessionId, workspaceId);
      return;
    }
    store.openConfigChatModal(workspaceId);
  }, [workspaceId, store]);

  const startSession = useCallback(
    async (agentProfileId: string, prompt?: string) => {
      if (isStarting) return;
      setIsStarting(true);
      setError(null);
      try {
        // Don't send the prompt to the backend — it will be sent via WS
        // message.add by QuickChatContent after the WS subscription is
        // established. This avoids a race condition where the agent completes
        // before the frontend subscribes and events are lost.
        const response = await startConfigChat(workspaceId, {
          agent_profile_id: agentProfileId,
        });

        // Seed the task session in the main store so QuickChatContent can find it
        // immediately. The WS event will merge on top when it arrives.
        storeApi.getState().setTaskSession({
          id: response.session_id,
          task_id: response.task_id,
          state: "CREATED",
          started_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
          agent_profile_id: agentProfileId,
        });

        // Store the prompt so QuickChatContent can send it via WS.
        if (prompt) setPendingPrompt(prompt);

        store.openConfigChat(response.session_id, workspaceId);
        store.renameConfigChatSession(response.session_id, prompt?.slice(0, 40) || "Config Chat");

        // Save the selected profile as the workspace default for future sessions
        if (!workspace?.default_config_agent_profile_id) {
          try {
            await updateWorkspaceAction(workspaceId, {
              default_config_agent_profile_id: agentProfileId,
            });
            updateWorkspaceInStore(workspaceId, {
              default_config_agent_profile_id: agentProfileId,
            });
          } catch {
            // Non-critical — don't fail the chat start for this
          }
        }
      } catch (err) {
        const message = err instanceof Error ? err.message : "Unknown error";
        setError(message);
      } finally {
        setIsStarting(false);
      }
    },
    [
      workspaceId,
      isStarting,
      store,
      storeApi,
      workspace?.default_config_agent_profile_id,
      updateWorkspaceInStore,
    ],
  );

  const close = useCallback(() => {
    store.closeConfigChat();
  }, [store]);

  const newChat = useCallback(() => {
    store.openConfigChatModal(workspaceId);
  }, [store, workspaceId]);

  const clearPendingPrompt = useCallback(() => setPendingPrompt(null), []);

  return {
    isOpen: store.isOpen,
    sessions: store.sessions,
    activeSessionId: store.activeSessionId,
    isStarting,
    error,
    workspace,
    defaultProfileId,
    pendingPrompt,
    clearPendingPrompt,
    open,
    startSession,
    close,
    closeSession: store.closeConfigChatSession,
    setActiveSession: store.setActiveConfigChatSession,
    newChat,
  };
}
