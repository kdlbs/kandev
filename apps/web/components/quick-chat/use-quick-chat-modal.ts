"use client";

import { useCallback, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { startQuickChat } from "@/lib/api/domains/workspace-api";

async function deleteQuickChatTask(taskId: string) {
  const { deleteTask } = await import("@/lib/api/domains/kanban-api");
  await deleteTask(taskId);
}

function useQuickChatStore() {
  return useAppStore(
    useShallow((s) => ({
      isOpen: s.quickChat.isOpen,
      sessions: s.quickChat.sessions,
      activeSessionId: s.quickChat.activeSessionId,
      closeQuickChat: s.closeQuickChat,
      closeQuickChatSession: s.closeQuickChatSession,
      setActiveQuickChatSession: s.setActiveQuickChatSession,
      renameQuickChatSession: s.renameQuickChatSession,
      openQuickChat: s.openQuickChat,
      agentProfiles: s.agentProfiles.items ?? [],
      taskSessions: s.taskSessions.items || {},
    })),
  );
}

export function useQuickChatModal(workspaceId: string) {
  const { toast } = useToast();
  const store = useQuickChatStore();
  const [isCreating, setIsCreating] = useState(false);
  const [showAgentPicker, setShowAgentPicker] = useState(false);
  const [sessionToClose, setSessionToClose] = useState<string | null>(null);

  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (!open) {
        store.closeQuickChat();
        setShowAgentPicker(false);
      }
    },
    [store],
  );

  const handleNewChat = useCallback(() => {
    store.openQuickChat("", workspaceId);
  }, [store, workspaceId]);

  const handleSelectAgent = useCallback(
    async (agentId: string) => {
      if (isCreating) return;
      setIsCreating(true);
      try {
        const agent = store.agentProfiles.find((p) => p.id === agentId);
        const sessionCount = store.sessions.filter((s) => s.sessionId !== "").length + 1;
        const initialName = `${agent?.label || "Agent"} - Chat ${sessionCount}`;
        const response = await startQuickChat(workspaceId, {
          agent_profile_id: agentId,
          title: initialName,
        });
        if (store.activeSessionId === "") store.closeQuickChatSession("");
        store.openQuickChat(response.session_id, workspaceId, agentId);
        store.renameQuickChatSession(response.session_id, initialName);
        setShowAgentPicker(false);
      } catch (error) {
        toast({
          title: "Failed to start quick chat",
          description: error instanceof Error ? error.message : "Unknown error",
          variant: "error",
        });
      } finally {
        setIsCreating(false);
      }
    },
    [workspaceId, isCreating, store, toast],
  );

  const handleCloseTab = useCallback(
    (sessionId: string) => {
      if (sessionId === "") {
        store.closeQuickChatSession(sessionId);
        return;
      }
      setSessionToClose(sessionId);
    },
    [store],
  );

  const handleConfirmClose = useCallback(async () => {
    if (!sessionToClose) return;
    const sessionId = sessionToClose;
    setSessionToClose(null);
    const taskId = store.taskSessions[sessionId]?.task_id;
    store.closeQuickChatSession(sessionId);
    if (taskId) {
      try {
        await deleteQuickChatTask(taskId);
      } catch (error) {
        console.error("Failed to delete quick chat task:", error);
        toast({
          title: "Failed to delete quick chat",
          description: error instanceof Error ? error.message : "Unknown error",
          variant: "error",
        });
      }
    }
  }, [sessionToClose, store, toast]);

  return {
    isOpen: store.isOpen,
    sessions: store.sessions,
    activeSessionId: store.activeSessionId,
    sessionToClose,
    activeSessionNeedsAgent: store.activeSessionId === "" || showAgentPicker,
    setActiveQuickChatSession: store.setActiveQuickChatSession,
    setSessionToClose,
    handleOpenChange,
    handleNewChat,
    handleSelectAgent,
    handleCloseTab,
    handleConfirmClose,
  };
}
