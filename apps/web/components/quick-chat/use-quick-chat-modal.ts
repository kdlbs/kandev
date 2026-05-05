"use client";

import { useCallback, useRef, useState } from "react";
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

type QuickChatStore = ReturnType<typeof useQuickChatStore>;

/** POSTs to start a quick-chat session and returns the response. */
async function startQuickChatForAgent(workspaceId: string, agentId: string, store: QuickChatStore) {
  const agent = store.agentProfiles.find((p) => p.id === agentId);
  const sessionCount = store.sessions.filter((s) => s.sessionId !== "").length + 1;
  const initialName = `${agent?.label || "Agent"} - Chat ${sessionCount}`;
  const response = await startQuickChat(workspaceId, {
    agent_profile_id: agentId,
    title: initialName,
  });
  return { sessionId: response.session_id, name: initialName, taskId: response.task_id };
}

/** Manages the eager agent-init lifecycle for the picker.
 *
 * Eager init means the backend boots a real agent process before responding.
 * Aborting the fetch on a rapid second click would NOT stop the backend agent
 * (it's already running by the time the abort lands), and we'd never see the
 * task_id on the FE — orphaning the task. Instead we let every request run
 * to completion and reconcile by request id: if a newer pick superseded this
 * one before the response arrived, we delete the now-orphaned ephemeral task.
 *
 * Exported for unit testing — see `use-quick-chat-modal.test.ts`. */
export function useAgentSelection(workspaceId: string, store: QuickChatStore) {
  const { toast } = useToast();
  const [pendingAgentId, setPendingAgentId] = useState<string | null>(null);
  // Monotonic request id; the latest click "wins" — older responses get
  // cleaned up if the backend already started their agent.
  const latestRequestId = useRef(0);

  const reset = useCallback(() => {
    latestRequestId.current += 1;
    setPendingAgentId(null);
  }, []);

  const handleSelectAgent = useCallback(
    async (agentId: string, onSuccess: () => void) => {
      const requestId = ++latestRequestId.current;
      setPendingAgentId(agentId);
      try {
        const result = await startQuickChatForAgent(workspaceId, agentId, store);
        if (latestRequestId.current !== requestId) {
          // A newer pick superseded us — the backend already booted this
          // agent, so delete the orphan task. Best-effort: ignore failures.
          deleteQuickChatTask(result.taskId).catch((err) =>
            console.error("Failed to clean up superseded quick chat task:", err),
          );
          return;
        }
        if (store.activeSessionId === "") store.closeQuickChatSession("");
        store.openQuickChat(result.sessionId, workspaceId, agentId);
        store.renameQuickChatSession(result.sessionId, result.name);
        onSuccess();
      } catch (error) {
        if (latestRequestId.current !== requestId) return;
        toast({
          title: "Failed to start quick chat",
          description: error instanceof Error ? error.message : "Unknown error",
          variant: "error",
        });
      } finally {
        if (latestRequestId.current === requestId) {
          setPendingAgentId(null);
        }
      }
    },
    [workspaceId, store, toast],
  );

  return { pendingAgentId, reset, handleSelectAgent };
}

export function useQuickChatModal(workspaceId: string) {
  const { toast } = useToast();
  const store = useQuickChatStore();
  const [showAgentPicker, setShowAgentPicker] = useState(false);
  const [sessionToClose, setSessionToClose] = useState<string | null>(null);
  const {
    pendingAgentId,
    reset,
    handleSelectAgent: doSelectAgent,
  } = useAgentSelection(workspaceId, store);

  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (open) return;
      reset();
      store.closeQuickChat();
      setShowAgentPicker(false);
    },
    [store, reset],
  );

  // Any picker-bypassing user action while a pick is pending should supersede
  // the in-flight start, so the resolved request cleans up its orphan task
  // instead of yanking the user back to that session.
  const handleNewChat = useCallback(() => {
    reset();
    store.openQuickChat("", workspaceId);
  }, [reset, store, workspaceId]);

  const handleSelectAgent = useCallback(
    (agentId: string) => doSelectAgent(agentId, () => setShowAgentPicker(false)),
    [doSelectAgent],
  );

  const setActiveQuickChatSession = useCallback(
    (sessionId: string) => {
      reset();
      store.setActiveQuickChatSession(sessionId);
    },
    [reset, store],
  );

  const handleCloseTab = useCallback(
    (sessionId: string) => {
      reset();
      if (sessionId === "") {
        store.closeQuickChatSession(sessionId);
        return;
      }
      setSessionToClose(sessionId);
    },
    [reset, store],
  );

  const handleRename = useCallback(
    (sessionId: string, name: string) => {
      if (!sessionId) return;
      store.renameQuickChatSession(sessionId, name);
    },
    [store],
  );

  const handleConfirmClose = useCallback(async () => {
    if (!sessionToClose) return;
    const sessionId = sessionToClose;
    setSessionToClose(null);
    const taskId = store.taskSessions[sessionId]?.task_id;
    store.closeQuickChatSession(sessionId);
    if (!taskId) return;
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
  }, [sessionToClose, store, toast]);

  return {
    isOpen: store.isOpen,
    sessions: store.sessions,
    activeSessionId: store.activeSessionId,
    sessionToClose,
    activeSessionNeedsAgent: store.activeSessionId === "" || showAgentPicker,
    pendingAgentId,
    setActiveQuickChatSession,
    setSessionToClose,
    handleOpenChange,
    handleNewChat,
    handleSelectAgent,
    handleCloseTab,
    handleConfirmClose,
    handleRename,
  };
}
