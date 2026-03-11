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
      sessionId: s.configChat.sessionId,
      taskId: s.configChat.taskId,
      workspaceId: s.configChat.workspaceId,
      openConfigChat: s.openConfigChat,
      openConfigChatModal: s.openConfigChatModal,
      closeConfigChat: s.closeConfigChat,
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
  const [isStarting, setIsStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const updateWorkspaceInStore = useUpdateWorkspaceInStore();

  const workspace = useAppStore(
    (s) => s.workspaces.items.find((w) => w.id === workspaceId) ?? null,
  );

  const defaultProfileId =
    workspace?.default_config_agent_profile_id ?? workspace?.default_agent_profile_id ?? undefined;

  const open = useCallback(() => {
    if (store.sessionId && store.workspaceId === workspaceId) {
      store.openConfigChat(store.sessionId, store.taskId ?? "", workspaceId);
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
        const response = await startConfigChat(workspaceId, {
          agent_profile_id: agentProfileId,
          prompt,
        });
        store.openConfigChat(response.session_id, response.task_id, workspaceId);

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
    [workspaceId, isStarting, store, workspace?.default_config_agent_profile_id, updateWorkspaceInStore],
  );

  const close = useCallback(() => {
    store.closeConfigChat();
  }, [store]);

  return {
    isOpen: store.isOpen,
    sessionId: store.sessionId,
    taskId: store.taskId,
    isStarting,
    error,
    workspace,
    defaultProfileId,
    open,
    startSession,
    close,
  };
}
