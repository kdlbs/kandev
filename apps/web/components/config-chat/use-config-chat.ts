"use client";

import { useCallback, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { startConfigChat } from "@/lib/api/domains/workspace-api";

function useConfigChatStore() {
  return useAppStore(
    useShallow((s) => ({
      isOpen: s.configChat.isOpen,
      sessionId: s.configChat.sessionId,
      taskId: s.configChat.taskId,
      workspaceId: s.configChat.workspaceId,
      openConfigChat: s.openConfigChat,
      closeConfigChat: s.closeConfigChat,
    })),
  );
}

export function useConfigChat(workspaceId: string) {
  const { toast } = useToast();
  const store = useConfigChatStore();
  const [isStarting, setIsStarting] = useState(false);

  const workspace = useAppStore(
    (s) => s.workspaces.items.find((w) => w.id === workspaceId) ?? null,
  );

  const open = useCallback(
    async (agentProfileId?: string) => {
      if (isStarting) return;

      // If already have an active session for this workspace, just reopen it
      if (store.sessionId && store.workspaceId === workspaceId) {
        store.openConfigChat(store.sessionId, store.taskId ?? "", workspaceId);
        return;
      }

      setIsStarting(true);
      try {
        const response = await startConfigChat(workspaceId, {
          agent_profile_id: agentProfileId,
        });
        store.openConfigChat(response.session_id, response.task_id, workspaceId);
      } catch (error) {
        toast({
          title: "Failed to start config chat",
          description: error instanceof Error ? error.message : "Unknown error",
          variant: "error",
        });
      } finally {
        setIsStarting(false);
      }
    },
    [workspaceId, isStarting, store, toast],
  );

  const close = useCallback(() => {
    store.closeConfigChat();
  }, [store]);

  return {
    isOpen: store.isOpen,
    sessionId: store.sessionId,
    taskId: store.taskId,
    isStarting,
    workspace,
    open,
    close,
  };
}
