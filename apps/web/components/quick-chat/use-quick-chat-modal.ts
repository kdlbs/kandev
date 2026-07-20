"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { startQuickChat, type QuickChatRepositoryInput } from "@/lib/api/domains/workspace-api";
import { isQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";
import type { QuickChatSessionKind } from "@/lib/state/slices/ui/types";

const noop = () => {};

async function deleteQuickChatTask(taskId: string) {
  const { deleteTask } = await import("@/lib/api/domains/kanban-api");
  await deleteTask(taskId);
}

function useQuickChatStore(workspaceId: string) {
  const store = useAppStore(
    useShallow((s) => ({
      isOpen: s.quickChat.isOpen,
      sessions: s.quickChat.sessions,
      activeSessionId: s.quickChat.activeSessionId,
      closeQuickChat: s.closeQuickChat,
      closeQuickChatSession: s.closeQuickChatSession,
      setActiveQuickChatSession: s.setActiveQuickChatSession,
      renameQuickChatSession: s.renameQuickChatSession,
      openQuickChat: s.openQuickChat,
      taskSessions: s.taskSessions.items || {},
    })),
  );
  const { agentProfiles } = useSettingsData(true);
  return useMemo(
    () => ({
      ...store,
      sessions: store.sessions.filter((session) => session.workspaceId === workspaceId),
      agentProfiles,
    }),
    [agentProfiles, store, workspaceId],
  );
}

type QuickChatStore = ReturnType<typeof useQuickChatStore>;

function useWorkspaceQuickChat(store: QuickChatStore) {
  const sessions = store.sessions;
  const activeSession = sessions.find((session) => session.sessionId === store.activeSessionId);
  useEffect(() => {
    if (store.isOpen && store.activeSessionId !== null && !activeSession) {
      store.closeQuickChat();
    }
  }, [activeSession, store.isOpen, store.activeSessionId, store.closeQuickChat]);
  return { sessions, activeSession };
}

/** POSTs to start a quick-chat session and returns the response. */
async function startQuickChatForAgent(
  workspaceId: string,
  agentId: string,
  store: QuickChatStore,
  repositories: QuickChatRepositoryInput[],
) {
  const agent = store.agentProfiles.find((p) => p.id === agentId);
  const sessionCount =
    store.sessions.filter(
      (session) =>
        session.workspaceId === workspaceId &&
        (session.kind ?? "chat") === "chat" &&
        !isQuickChatSetupSessionId(session.sessionId),
    ).length + 1;
  const initialName = `${agent?.label || "Agent"} - Chat ${sessionCount}`;
  const response = await startQuickChat(workspaceId, {
    agent_profile_id: agentId,
    title: initialName,
    repositories: repositories.length > 0 ? repositories : undefined,
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
    async (agentId: string, repositories: QuickChatRepositoryInput[] = []) => {
      const requestId = ++latestRequestId.current;
      const setupSessionId = store.activeSessionId;
      setPendingAgentId(agentId);
      try {
        const result = await startQuickChatForAgent(workspaceId, agentId, store, repositories);
        if (latestRequestId.current !== requestId) {
          // A newer pick superseded us — the backend already booted this
          // agent, so delete the orphan task. Best-effort: ignore failures.
          deleteQuickChatTask(result.taskId).catch((err) =>
            console.error("Failed to clean up superseded quick chat task:", err),
          );
          return;
        }
        if (setupSessionId && isQuickChatSetupSessionId(setupSessionId)) {
          store.closeQuickChatSession(setupSessionId);
        }
        store.openQuickChat(result.sessionId, workspaceId, agentId);
        store.renameQuickChatSession(result.sessionId, result.name);
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

function useQuickChatSessionClose(store: QuickChatStore, resetPendingStarts: () => void) {
  const { toast } = useToast();
  const [sessionToClose, setSessionToClose] = useState<string | null>(null);
  const handleCloseTab = useCallback(
    (sessionId: string) => {
      resetPendingStarts();
      if (isQuickChatSetupSessionId(sessionId)) {
        store.closeQuickChatSession(sessionId);
        return;
      }
      setSessionToClose(sessionId);
    },
    [resetPendingStarts, store],
  );
  const handleConfirmClose = useCallback(async () => {
    if (!sessionToClose) return;
    const sessionId = sessionToClose;
    setSessionToClose(null);
    const taskId = store.taskSessions[sessionId]?.task_id;
    if (!taskId) {
      store.closeQuickChatSession(sessionId);
      return;
    }
    try {
      await deleteQuickChatTask(taskId);
      store.closeQuickChatSession(sessionId);
    } catch (error) {
      console.error("Failed to delete quick chat task:", error);
      toast({
        title: "Failed to delete quick chat",
        description: error instanceof Error ? error.message : "Unknown error",
        variant: "error",
      });
    }
  }, [sessionToClose, store, toast]);
  return { sessionToClose, setSessionToClose, handleCloseTab, handleConfirmClose };
}

export function useQuickChatModal(workspaceId: string, onSupersedeConfigStart = noop) {
  const store = useQuickChatStore(workspaceId);
  const { sessions, activeSession } = useWorkspaceQuickChat(store);
  const [setupKey, setSetupKey] = useState(0);
  const {
    pendingAgentId,
    reset,
    handleSelectAgent: doSelectAgent,
  } = useAgentSelection(workspaceId, store);
  const resetPendingStarts = useCallback(() => {
    reset();
    onSupersedeConfigStart();
  }, [onSupersedeConfigStart, reset]);
  const { sessionToClose, setSessionToClose, handleCloseTab, handleConfirmClose } =
    useQuickChatSessionClose(store, resetPendingStarts);

  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (open) return;
      resetPendingStarts();
      sessions
        .filter((session) => isQuickChatSetupSessionId(session.sessionId))
        .forEach((session) => store.closeQuickChatSession(session.sessionId));
      store.closeQuickChat();
    },
    [resetPendingStarts, sessions, store],
  );

  // Any picker-bypassing user action while a pick is pending should supersede
  // the in-flight start, so the resolved request cleans up its orphan task
  // instead of yanking the user back to that session.
  const handleNewChat = useCallback(() => {
    resetPendingStarts();
    setSetupKey((key) => key + 1);
    store.openQuickChat("", workspaceId, undefined, "chat");
  }, [resetPendingStarts, store, workspaceId]);

  const handleSetupKindChange = useCallback(
    (kind: QuickChatSessionKind) => {
      resetPendingStarts();
      if (activeSession && isQuickChatSetupSessionId(activeSession.sessionId)) {
        store.closeQuickChatSession(activeSession.sessionId);
      }
      setSetupKey((key) => key + 1);
      store.openQuickChat("", workspaceId, undefined, kind);
    },
    [activeSession, resetPendingStarts, store, workspaceId],
  );

  const handleSelectAgent = useCallback(
    (agentId: string, repositories: QuickChatRepositoryInput[] = []) =>
      doSelectAgent(agentId, repositories),
    [doSelectAgent],
  );

  const setActiveQuickChatSession = useCallback(
    (sessionId: string) => {
      resetPendingStarts();
      store.setActiveQuickChatSession(sessionId, workspaceId);
    },
    [resetPendingStarts, store, workspaceId],
  );

  const handleRename = useCallback(
    (sessionId: string, name: string) => {
      if (!sessionId) return;
      store.renameQuickChatSession(sessionId, name);
    },
    [store],
  );

  return {
    isOpen: store.isOpen,
    sessions,
    activeSessionId: activeSession?.sessionId ?? null,
    activeSession,
    sessionToClose,
    setupKey,
    activeSessionNeedsAgent: Boolean(
      activeSession && isQuickChatSetupSessionId(activeSession.sessionId),
    ),
    pendingAgentId,
    setActiveQuickChatSession,
    setSessionToClose,
    handleOpenChange,
    handleNewChat,
    handleSetupKindChange,
    handleSelectAgent,
    handleCloseTab,
    handleConfirmClose,
    handleRename,
  };
}
