"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import { cancelPtyTerminalStart } from "@/components/settings/pty-terminal-lifecycle";
import { deleteQuickTerminalTab } from "@/lib/api/domains/quick-terminal-api";
import { ApiError } from "@/lib/api/client";
import { isQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";
import type {
  QuickChatActiveKind,
  QuickChatSession,
  QuickTerminalTab,
  QuickTerminalUpdate,
} from "@/lib/state/slices/ui/types";
import {
  adjacentQuickChatTabReference,
  conversationTabReference,
  terminalTabReference,
} from "./use-quick-chat-tab-order";

export type QuickChatCloseStore = {
  sessions: QuickChatSession[];
  terminalTabs: QuickTerminalTab[];
  activeSessionId: string | null;
  activeKind: QuickChatActiveKind;
  activeTerminalTabId: string | null;
  taskSessions: Record<string, { task_id: string }>;
  closeQuickChatSession: (sessionId: string) => void;
  removeQuickChatSession: (sessionId: string) => void;
  setActiveQuickChatSession: (sessionId: string, workspaceId: string) => void;
  activateQuickTerminal: (tabId: string, workspaceId: string) => void;
  updateQuickTerminal: (tabId: string, update: QuickTerminalUpdate) => void;
  removeQuickTerminal: (tabId: string) => void;
};

export function resolveQuickChatTaskId(
  store: Pick<QuickChatCloseStore, "sessions" | "taskSessions">,
  sessionId: string,
): string | undefined {
  return (
    store.sessions.find((session) => session.sessionId === sessionId)?.taskId ??
    store.taskSessions[sessionId]?.task_id
  );
}

async function deleteQuickChatTask(taskId: string) {
  const { deleteTask } = await import("@/lib/api/domains/kanban-api");
  await deleteTask(taskId);
}

type QuickChatTabCloseNavigation = {
  tabOrder: string[];
  activeTabReference: string | undefined;
  onActivateTabReference: (reference: string) => void;
};

function getActiveQuickChatTabReference(
  store: Pick<QuickChatCloseStore, "activeKind" | "activeSessionId" | "activeTerminalTabId">,
): string | undefined {
  if (store.activeKind === "conversation" && store.activeSessionId) {
    return conversationTabReference(store.activeSessionId);
  }
  if (store.activeKind === "terminal" && store.activeTerminalTabId) {
    return terminalTabReference(store.activeTerminalTabId);
  }
  return undefined;
}

function activateQuickChatTabReference(
  store: Pick<QuickChatCloseStore, "setActiveQuickChatSession" | "activateQuickTerminal">,
  workspaceId: string,
  reference: string,
): void {
  if (reference.startsWith("conversation:")) {
    const sessionId = reference.slice("conversation:".length);
    if (sessionId) store.setActiveQuickChatSession(sessionId, workspaceId);
    return;
  }
  if (reference.startsWith("terminal:")) {
    const tabId = reference.slice("terminal:".length);
    if (tabId) store.activateQuickTerminal(tabId, workspaceId);
  }
}

function useQuickChatSessionClose(
  store: QuickChatCloseStore,
  resetPendingStarts: () => void,
  removeTabReference: (reference: string) => void,
  navigation: QuickChatTabCloseNavigation,
) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const { activeTabReference, onActivateTabReference, tabOrder } = navigation;
  const [sessionToClose, setSessionToClose] = useState<string | null>(null);
  const [replacementReference, setReplacementReference] = useState<string | null>(null);

  const handleCloseTab = useCallback(
    (sessionId: string) => {
      resetPendingStarts();
      if (isQuickChatSetupSessionId(sessionId)) {
        setReplacementReference(null);
        store.closeQuickChatSession(sessionId);
        return;
      }
      const reference = conversationTabReference(sessionId);
      setReplacementReference(
        activeTabReference === reference
          ? (adjacentQuickChatTabReference(tabOrder, reference) ?? null)
          : null,
      );
      setSessionToClose(sessionId);
    },
    [activeTabReference, resetPendingStarts, store, tabOrder],
  );

  const handleConfirmClose = useCallback(async () => {
    if (!sessionToClose) return;
    const sessionId = sessionToClose;
    const replacement = replacementReference;
    setSessionToClose(null);
    setReplacementReference(null);
    const taskId = resolveQuickChatTaskId(store, sessionId);
    if (!taskId) {
      store.removeQuickChatSession(sessionId);
      removeTabReference(conversationTabReference(sessionId));
      if (replacement) onActivateTabReference(replacement);
      return;
    }
    try {
      await deleteQuickChatTask(taskId);
      store.removeQuickChatSession(sessionId);
      removeTabReference(conversationTabReference(sessionId));
      if (replacement) onActivateTabReference(replacement);
    } catch (error) {
      console.error("Failed to delete quick chat task:", error);
      toast({
        title: t("chat:failedToDeleteQuickChat"),
        description: error instanceof Error ? error.message : t("chat:unknownError"),
        variant: "error",
      });
    }
  }, [
    onActivateTabReference,
    removeTabReference,
    replacementReference,
    sessionToClose,
    store,
    t,
    toast,
  ]);

  return { sessionToClose, setSessionToClose, handleCloseTab, handleConfirmClose };
}

function useQuickTerminalClose(
  store: QuickChatCloseStore,
  resetPendingStarts: () => void,
  removeTabReference: (reference: string) => void,
  navigation: QuickChatTabCloseNavigation,
) {
  const { toast } = useToast();
  const { t } = useTranslation();
  const { activeTabReference, onActivateTabReference, tabOrder } = navigation;

  const handleCloseTerminal = useCallback(
    async (tabId: string) => {
      resetPendingStarts();
      cancelPtyTerminalStart(tabId);
      const tab = store.terminalTabs.find((item) => item.tabId === tabId);
      if (!tab) return;
      const reference = terminalTabReference(tabId);
      const replacement =
        activeTabReference === reference
          ? adjacentQuickChatTabReference(tabOrder, reference)
          : undefined;
      try {
        await deleteQuickTerminalTab(tabId);
      } catch (error) {
        if (error instanceof ApiError && error.status === 404) {
          store.removeQuickTerminal(tabId);
          removeTabReference(reference);
          if (replacement) onActivateTabReference(replacement);
          return;
        }
        const message = error instanceof Error ? error.message : String(error);
        store.updateQuickTerminal(tabId, { status: "error", error: message });
        toast({
          title: t("sidebar:quickChatTerminals"),
          description: t("sidebar:quickChatTerminalError", { error: message }),
          variant: "error",
        });
        return;
      }
      store.removeQuickTerminal(tabId);
      removeTabReference(reference);
      if (replacement) onActivateTabReference(replacement);
    },
    [
      activeTabReference,
      onActivateTabReference,
      removeTabReference,
      resetPendingStarts,
      store,
      t,
      tabOrder,
      toast,
    ],
  );

  return handleCloseTerminal;
}

type QuickChatCloseActionsOptions = {
  workspaceId: string;
  store: QuickChatCloseStore;
  resetPendingStarts: () => void;
  removeTabReference: (reference: string) => void;
  tabOrder: string[];
};

export function useQuickChatCloseActions({
  workspaceId,
  store,
  resetPendingStarts,
  removeTabReference,
  tabOrder,
}: QuickChatCloseActionsOptions) {
  const activeTabReference = getActiveQuickChatTabReference(store);
  const onActivateTabReference = useCallback(
    (reference: string) => activateQuickChatTabReference(store, workspaceId, reference),
    [store, workspaceId],
  );
  const navigation = { tabOrder, activeTabReference, onActivateTabReference };
  const sessionClose = useQuickChatSessionClose(
    store,
    resetPendingStarts,
    removeTabReference,
    navigation,
  );
  const handleCloseTerminal = useQuickTerminalClose(
    store,
    resetPendingStarts,
    removeTabReference,
    navigation,
  );

  return { ...sessionClose, handleCloseTerminal };
}
