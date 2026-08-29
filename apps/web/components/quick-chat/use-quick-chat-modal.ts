"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { startQuickChat, type QuickChatRepositoryInput } from "@/lib/api/domains/workspace-api";
import { updateQuickTerminalTab } from "@/lib/api/domains/quick-terminal-api";
import { ApiError } from "@/lib/api/client";
import { type PtyTerminalState } from "@/components/settings/pty-terminal-view";
import { isQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";
import { persistQuickChatRename } from "@/lib/quick-chat/rename";
import { recordAgentProfileRecentUseBestEffort } from "@/lib/agent-profile-recent-use";
import { registerQuickChatCloseHandler } from "./quick-chat-focus";
import type { QuickChatSessionKind, QuickTerminalTab } from "@/lib/state/slices/ui/types";
import { useQuickChatCloseActions, resolveQuickChatTaskId } from "./use-quick-chat-close-actions";
import { useQuickChatTabOrder } from "./use-quick-chat-tab-order";

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
      activeKind: s.quickChat.activeKind,
      activeTerminalTabId: s.quickChat.activeTerminalTabId,
      terminalTabs: s.quickChat.terminalTabs,
      closeQuickChat: s.closeQuickChat,
      closeQuickChatSession: s.closeQuickChatSession,
      removeQuickChatSession: s.removeQuickChatSession,
      setActiveQuickChatSession: s.setActiveQuickChatSession,
      createQuickTerminal: s.createQuickTerminal,
      updateQuickTerminal: s.updateQuickTerminal,
      activateQuickTerminal: s.activateQuickTerminal,
      removeQuickTerminal: s.removeQuickTerminal,
      renameQuickChatSession: s.renameQuickChatSession,
      openQuickChat: s.openQuickChat,
      applyAgentProfileRecentUse: s.applyAgentProfileRecentUse,
      agentProfiles: s.agentProfiles.items ?? [],
      agentGeneratedTaskTitles: s.userSettings.agentGeneratedTaskTitles,
      taskSessions: s.taskSessions.items || {},
    })),
  );
  return useMemo(
    () => ({
      ...store,
      sessions: store.sessions.filter((session) => session.workspaceId === workspaceId),
      terminalTabs: store.terminalTabs.filter((tab) => tab.workspaceId === workspaceId),
    }),
    [store, workspaceId],
  );
}

type QuickChatStore = ReturnType<typeof useQuickChatStore>;

function persistQuickTerminalState(
  store: QuickChatStore,
  tabId: string,
  state: PtyTerminalState,
): void {
  store.updateQuickTerminal(tabId, {
    sessionId: state.sessionId,
    status: state.status,
    exitCode: state.exitCode,
    error: state.error,
  });
  void updateQuickTerminalTab(tabId, {
    sessionId: state.status === "exited" ? null : state.sessionId,
    status: state.status,
    exitCode: state.exitCode,
    error: state.error,
  })
    .then((descriptor) => {
      store.updateQuickTerminal(tabId, {
        sequence: descriptor.sequence,
        sessionId: descriptor.sessionId,
        status: descriptor.status,
        exitCode: descriptor.exitCode,
        error: descriptor.error,
      });
    })
    .catch((error: unknown) => {
      if (error instanceof ApiError && error.status === 404) return;
      store.updateQuickTerminal(tabId, {
        status: "error",
        error: error instanceof Error ? error.message : String(error),
      });
    });
}

function applyQuickTerminalDescriptor(
  store: QuickChatStore,
  tabId: string,
  descriptor: QuickTerminalTab,
): void {
  store.updateQuickTerminal(tabId, {
    sequence: descriptor.sequence,
    sessionId: descriptor.sessionId,
    status: descriptor.status,
    exitCode: descriptor.exitCode,
    error: descriptor.error,
  });
}

function useWorkspaceQuickChat(store: QuickChatStore) {
  const sessions = store.sessions;
  const terminalTabs = store.terminalTabs;
  const activeSession =
    store.activeKind === "conversation"
      ? sessions.find((session) => session.sessionId === store.activeSessionId)
      : undefined;
  const activeTerminalTab =
    store.activeKind === "terminal"
      ? terminalTabs.find((tab) => tab.tabId === store.activeTerminalTabId)
      : undefined;
  useEffect(() => {
    if (
      store.isOpen &&
      store.activeKind === "conversation" &&
      store.activeSessionId &&
      !activeSession
    ) {
      store.closeQuickChat();
    }
    if (
      store.isOpen &&
      store.activeKind === "terminal" &&
      store.activeTerminalTabId &&
      !activeTerminalTab
    ) {
      store.closeQuickChat();
    }
  }, [activeSession, activeTerminalTab, store]);
  return { sessions, terminalTabs, activeSession, activeTerminalTab };
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
  // i18n-exempt: persisted as the quick-chat task title, same contract as use-config-chat.ts.
  const initialName = `${agent?.label || "Agent"} - Chat ${sessionCount}`;
  const response = await startQuickChat(workspaceId, {
    agent_profile_id: agentId,
    title: initialName,
    ...(store.agentGeneratedTaskTitles ? { auto_title: true } : {}),
    repositories: repositories.length > 0 ? repositories : undefined,
  });
  return {
    sessionId: response.session_id,
    name: initialName,
    taskId: response.task_id,
    agentProfileId: response.agent_profile_id ?? agentId,
  };
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
  const { t } = useTranslation();
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
        recordAgentProfileRecentUseBestEffort("quick_chat", result.agentProfileId, (record) =>
          store.applyAgentProfileRecentUse("quick_chat", record),
        );
        if (setupSessionId && isQuickChatSetupSessionId(setupSessionId)) {
          store.closeQuickChatSession(setupSessionId);
        }
        store.openQuickChat(
          result.sessionId,
          workspaceId,
          result.agentProfileId,
          "chat",
          result.taskId,
        );
        store.renameQuickChatSession(result.sessionId, result.name);
      } catch (error) {
        if (latestRequestId.current !== requestId) return;
        toast({
          title: t("chat:failedToStartQuickChat"),
          description: error instanceof Error ? error.message : t("chat:unknownError"),
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

type QuickChatTabActionsOptions = {
  workspaceId: string;
  sessions: QuickChatStore["sessions"];
  activeSession: QuickChatStore["sessions"][number] | undefined;
  store: QuickChatStore;
  resetPendingStarts: () => void;
  removeTabReference: (reference: string) => void;
  tabOrder: string[];
  setSetupKey: React.Dispatch<React.SetStateAction<number>>;
};

function useQuickChatRename(store: QuickChatStore) {
  const { toast } = useToast();
  const { t } = useTranslation();

  return useCallback(
    (sessionId: string, name: string) => {
      if (!sessionId) return;
      store.renameQuickChatSession(sessionId, name);
      persistQuickChatRename(sessionId, resolveQuickChatTaskId(store, sessionId), name).catch(
        () => {
          toast({
            title: t("chat:renameSavedOnThisDeviceOnly"),
            description: t("chat:renameSyncFailedDescription"),
            variant: "error",
          });
        },
      );
    },
    [store, t, toast],
  );
}

function useQuickChatTabActions({
  workspaceId,
  sessions,
  activeSession,
  store,
  resetPendingStarts,
  removeTabReference,
  tabOrder,
  setSetupKey,
}: QuickChatTabActionsOptions) {
  const {
    sessionToClose,
    setSessionToClose,
    handleCloseTab,
    handleConfirmClose,
    handleCloseTerminal,
  } = useQuickChatCloseActions({
    workspaceId,
    store,
    resetPendingStarts,
    removeTabReference,
    tabOrder,
  });
  const handleRename = useQuickChatRename(store);

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

  const handleNewChat = useCallback(() => {
    resetPendingStarts();
    setSetupKey((key) => key + 1);
    store.openQuickChat("", workspaceId, undefined, "chat");
  }, [resetPendingStarts, setSetupKey, store, workspaceId]);

  const handleNewTerminal = useCallback(() => {
    resetPendingStarts();
    store.createQuickTerminal(workspaceId);
  }, [resetPendingStarts, store, workspaceId]);

  const handleActivateTerminal = useCallback(
    (tabId: string) => {
      resetPendingStarts();
      store.activateQuickTerminal(tabId, workspaceId);
    },
    [resetPendingStarts, store, workspaceId],
  );

  const handleTerminalStateChange = useCallback(
    (tabId: string, state: PtyTerminalState) => {
      persistQuickTerminalState(store, tabId, state);
    },
    [store],
  );

  const handleTerminalDescriptorReady = useCallback(
    (tabId: string, descriptor: QuickTerminalTab) => {
      applyQuickTerminalDescriptor(store, tabId, descriptor);
    },
    [store],
  );

  const handleSetupKindChange = useCallback(
    (kind: QuickChatSessionKind) => {
      resetPendingStarts();
      if (activeSession && isQuickChatSetupSessionId(activeSession.sessionId)) {
        store.closeQuickChatSession(activeSession.sessionId);
      }
      setSetupKey((key) => key + 1);
      store.openQuickChat("", workspaceId, undefined, kind);
    },
    [activeSession, resetPendingStarts, setSetupKey, store, workspaceId],
  );

  const setActiveQuickChatSession = useCallback(
    (sessionId: string) => {
      resetPendingStarts();
      store.setActiveQuickChatSession(sessionId, workspaceId);
    },
    [resetPendingStarts, store, workspaceId],
  );

  return {
    sessionToClose,
    setSessionToClose,
    handleOpenChange,
    handleNewChat,
    handleNewTerminal,
    handleActivateTerminal,
    handleTerminalStateChange,
    handleTerminalDescriptorReady,
    handleSetupKindChange,
    setActiveQuickChatSession,
    handleCloseTab,
    handleCloseTerminal,
    handleConfirmClose,
    handleRename,
  };
}

export function useQuickChatModal(workspaceId: string, onSupersedeConfigStart = noop) {
  const store = useQuickChatStore(workspaceId);
  const {
    sessions: workspaceSessions,
    terminalTabs: workspaceTerminalTabs,
    activeSession,
    activeTerminalTab,
  } = useWorkspaceQuickChat(store);
  const tabOrder = useQuickChatTabOrder(workspaceId, workspaceSessions, workspaceTerminalTabs);
  const sessions = tabOrder.sessions;
  const terminalTabs = tabOrder.terminalTabs;
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
  const tabActions = useQuickChatTabActions({
    workspaceId,
    sessions,
    activeSession,
    store,
    resetPendingStarts,
    removeTabReference: tabOrder.removeTabReference,
    tabOrder: tabOrder.order,
    setSetupKey,
  });
  const closeFromLauncher = useCallback(
    () => tabActions.handleOpenChange(false),
    [tabActions.handleOpenChange],
  );
  useEffect(() => registerQuickChatCloseHandler(closeFromLauncher), [closeFromLauncher]);

  const handleSelectAgent = useCallback(
    (agentId: string, repositories: QuickChatRepositoryInput[] = []) =>
      doSelectAgent(agentId, repositories),
    [doSelectAgent],
  );

  return {
    isOpen: store.isOpen,
    sessions,
    terminalTabs,
    activeKind: store.activeKind,
    activeTerminalTabId: activeTerminalTab?.tabId ?? null,
    activeTerminalTab,
    activeSessionId: activeSession?.sessionId ?? null,
    activeSession,
    ...tabActions,
    tabOrder: tabOrder.order,
    persistTabOrder: tabOrder.persistOrder,
    tabOrderSyncError: tabOrder.syncError,
    tabOrderSyncPending: tabOrder.syncPending,
    setupKey,
    activeSessionNeedsAgent: Boolean(
      activeSession && isQuickChatSetupSessionId(activeSession.sessionId),
    ),
    pendingAgentId,
    handleSelectAgent,
  };
}
