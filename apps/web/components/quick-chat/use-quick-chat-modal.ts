"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import type { QuickChatRepositoryInput } from "@/lib/api/domains/workspace-api";
import { isQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";
import { persistQuickChatRename } from "@/lib/quick-chat/rename";
import { registerQuickChatCloseHandler } from "./quick-chat-focus";
import type { QuickChatOpeningPayload, QuickChatSessionKind } from "@/lib/state/slices/ui/types";
import { useQuickChatCloseActions, resolveQuickChatTaskId } from "./use-quick-chat-close-actions";
import { useQuickChatTabOrder } from "./use-quick-chat-tab-order";
import { useAgentSelection } from "./use-quick-chat-agent-selection";
import { useQuickTerminalLifecycle } from "@/hooks/domains/session/use-quick-terminal-lifecycle";

export { useAgentSelection };

const noop = () => {};

function useQuickChatStore(workspaceId: string) {
  const store = useAppStore(
    useShallow((s) => ({
      isOpen: s.quickChat.isOpen,
      sessions: s.quickChat.sessions,
      activeSessionId: s.quickChat.activeSessionId,
      activeKind: s.quickChat.activeKind,
      activeTerminalTabId: s.quickChat.activeTerminalTabId,
      pendingQuickChatOpen: s.quickChat.pendingOpen,
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
      setQuickChatInitialPrompt: s.setQuickChatInitialPrompt,
      openQuickChat: s.openQuickChat,
      upsertQuickChatSessionFromEvent: s.upsertQuickChatSessionFromEvent,
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

export type QuickChatStore = ReturnType<typeof useQuickChatStore>;

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

type QuickChatTabActionsOptions = {
  workspaceId: string;
  sessions: QuickChatStore["sessions"];
  activeSession: QuickChatStore["sessions"][number] | undefined;
  store: QuickChatStore;
  resetPendingStarts: () => void;
  removeTabReference: (reference: string) => void;
  tabOrder: string[];
  setSetupKey: React.Dispatch<React.SetStateAction<number>>;
  terminalLifecycle: ReturnType<typeof useQuickTerminalLifecycle>;
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
  terminalLifecycle,
}: QuickChatTabActionsOptions) {
  const {
    sessionToClose,
    taskIdToClose,
    setSessionToClose,
    handleCloseTab,
    handleConfirmClose,
    handleCloseTerminal,
  } = useQuickChatCloseActions({
    workspaceId,
    store,
    resetPendingStarts,
    beginTerminalClose: terminalLifecycle.beginClose,
    finishTerminalClose: terminalLifecycle.finishClose,
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

  const handleSetupKindChange = useCallback(
    (kind: QuickChatSessionKind) => {
      resetPendingStarts();
      if (activeSession && isQuickChatSetupSessionId(activeSession.sessionId)) {
        store.closeQuickChatSession(activeSession.sessionId);
      }
      store.openQuickChat("", workspaceId, undefined, kind);
    },
    [activeSession, resetPendingStarts, store, workspaceId],
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
    taskIdToClose,
    setSessionToClose,
    handleOpenChange,
    handleNewChat,
    handleNewTerminal,
    handleActivateTerminal,
    handleTerminalStateChange: terminalLifecycle.handleTerminalStateChange,
    handleTerminalDescriptorReady: terminalLifecycle.handleTerminalDescriptorReady,
    handleRecoverTerminal: terminalLifecycle.handleRecoverTerminal,
    terminalRecoveryByTab: terminalLifecycle.terminalRecoveryByTab,
    handleSetupKindChange,
    setActiveQuickChatSession,
    handleCloseTab,
    handleCloseTerminal,
    handleConfirmClose,
    handleRename,
  };
}

export function useQuickChatModal(
  workspaceId: string,
  onSupersedeConfigStart = noop,
  onDiscardSetupDraft = noop,
) {
  const store = useQuickChatStore(workspaceId);
  const terminalLifecycle = useQuickTerminalLifecycle(workspaceId, store);
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
    setupError,
    reportSetupError,
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
    terminalLifecycle,
  });
  const closeFromLauncher = useCallback(() => {
    onDiscardSetupDraft();
    tabActions.handleOpenChange(false);
  }, [onDiscardSetupDraft, tabActions.handleOpenChange]);
  useEffect(() => registerQuickChatCloseHandler(closeFromLauncher), [closeFromLauncher]);

  const handleSelectAgent = useCallback(
    (
      agentId: string,
      repositories: QuickChatRepositoryInput[] = [],
      openingPayload?: QuickChatOpeningPayload,
      creationId?: string,
      onFailure?: (error: unknown) => void,
    ) => doSelectAgent(agentId, repositories, openingPayload, creationId, onFailure),
    [doSelectAgent],
  );

  return {
    isOpen: store.isOpen,
    sessions,
    terminalTabs,
    activeKind: store.activeKind,
    activeTerminalTabId: activeTerminalTab?.tabId ?? null,
    pendingQuickChatOpen: store.pendingQuickChatOpen,
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
    setupError,
    reportSetupError,
    handleSelectAgent,
  };
}
