import type { Draft } from "immer";
import type { QuickChatState, QuickTerminalTab, QuickTerminalUpdate, UISlice } from "./types";

type ImmerSet = (recipe: (draft: Draft<UISlice>) => void) => void;

function createQuickTerminalId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `quick-terminal-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

function findWorkspaceTerminal(
  terminals: QuickTerminalTab[],
  workspaceId: string,
  tabId: string | null | undefined,
): QuickTerminalTab | undefined {
  return terminals.find(
    (tab) => tab.workspaceId === workspaceId && (!tabId || tab.tabId === tabId),
  );
}

export function activateConversationDraft(
  quickChat: QuickChatState,
  sessionId: string,
  workspaceId: string,
): void {
  const session = quickChat.sessions.find((item) => item.sessionId === sessionId);
  if (!session || session.workspaceId !== workspaceId) return;
  quickChat.activeKind = "conversation";
  quickChat.activeSessionId = sessionId;
  quickChat.isOpen = true;
}

export function activateTerminalDraft(quickChat: QuickChatState, tab: QuickTerminalTab): void {
  quickChat.activeKind = "terminal";
  quickChat.activeTerminalTabId = tab.tabId;
  quickChat.lastTerminalTabIdByWorkspace[tab.workspaceId] = tab.tabId;
  quickChat.isOpen = true;
}

function findTerminalFallback(
  terminals: QuickTerminalTab[],
  workspaceId: string,
  removedSequence: number,
): QuickTerminalTab | undefined {
  const sameWorkspace = terminals.filter((tab) => tab.workspaceId === workspaceId);
  return (
    sameWorkspace.find((tab) => tab.sequence > removedSequence) ??
    [...sameWorkspace].sort((a, b) => b.sequence - a.sequence)[0]
  );
}

export function activateWorkspaceFallback(quickChat: QuickChatState, workspaceId?: string): void {
  if (!workspaceId) {
    quickChat.activeSessionId = null;
    quickChat.isOpen = false;
    return;
  }
  const conversation = quickChat.sessions.find((session) => session.workspaceId === workspaceId);
  if (conversation) {
    activateConversationDraft(quickChat, conversation.sessionId, workspaceId);
    return;
  }
  const terminal = quickChat.terminalTabs.find((tab) => tab.workspaceId === workspaceId);
  if (terminal) {
    activateTerminalDraft(quickChat, terminal);
    return;
  }
  quickChat.activeSessionId = null;
  quickChat.activeKind = "conversation";
  quickChat.isOpen = false;
}

function createQuickTerminalDraft(quickChat: QuickChatState, workspaceId: string): string {
  const workspaceTerminals = quickChat.terminalTabs.filter(
    (tab) => tab.workspaceId === workspaceId,
  );
  const sequence = workspaceTerminals.reduce((max, tab) => Math.max(max, tab.sequence), 0) + 1;
  const tab: QuickTerminalTab = {
    tabId: createQuickTerminalId(),
    workspaceId,
    sessionId: null,
    sequence,
    status: "connecting",
  };
  quickChat.terminalTabs.push(tab);
  activateTerminalDraft(quickChat, tab);
  return tab.tabId;
}

export function buildQuickTerminalActions(set: ImmerSet) {
  return {
    reuseOrCreateQuickTerminal: (workspaceId: string) => {
      let tabId = "";
      set((draft) => {
        const lastId = draft.quickChat.lastTerminalTabIdByWorkspace[workspaceId];
        const existing = findWorkspaceTerminal(draft.quickChat.terminalTabs, workspaceId, lastId);
        const fallback =
          existing ?? findWorkspaceTerminal(draft.quickChat.terminalTabs, workspaceId, undefined);
        tabId = fallback?.tabId ?? createQuickTerminalDraft(draft.quickChat, workspaceId);
        if (fallback) activateTerminalDraft(draft.quickChat, fallback);
      });
      return tabId;
    },
    createQuickTerminal: (workspaceId: string) => {
      let tabId = "";
      set((draft) => {
        tabId = createQuickTerminalDraft(draft.quickChat, workspaceId);
      });
      return tabId;
    },
    updateQuickTerminal: (tabId: string, update: QuickTerminalUpdate) =>
      set((draft) => {
        const tab = draft.quickChat.terminalTabs.find((item) => item.tabId === tabId);
        if (!tab) return;
        if ("sessionId" in update) tab.sessionId = update.sessionId ?? null;
        if (update.status) tab.status = update.status;
        if ("exitCode" in update) {
          if (update.exitCode == null) delete tab.exitCode;
          else tab.exitCode = update.exitCode;
        }
        if ("error" in update) {
          if (!update.error) delete tab.error;
          else tab.error = update.error;
        }
      }),
    activateQuickTerminal: (tabId: string, workspaceId: string) =>
      set((draft) => {
        const tab = draft.quickChat.terminalTabs.find((item) => item.tabId === tabId);
        if (!tab || tab.workspaceId !== workspaceId) return;
        activateTerminalDraft(draft.quickChat, tab);
      }),
    removeQuickTerminal: (tabId: string) =>
      set((draft) => {
        const index = draft.quickChat.terminalTabs.findIndex((tab) => tab.tabId === tabId);
        if (index === -1) return;
        const closing = draft.quickChat.terminalTabs[index];
        draft.quickChat.terminalTabs.splice(index, 1);
        const replacement = findTerminalFallback(
          draft.quickChat.terminalTabs,
          closing.workspaceId,
          closing.sequence,
        );
        if (draft.quickChat.lastTerminalTabIdByWorkspace[closing.workspaceId] === tabId) {
          if (replacement) {
            draft.quickChat.lastTerminalTabIdByWorkspace[closing.workspaceId] = replacement.tabId;
          } else {
            delete draft.quickChat.lastTerminalTabIdByWorkspace[closing.workspaceId];
          }
        }
        if (
          draft.quickChat.activeKind === "terminal" &&
          draft.quickChat.activeTerminalTabId === tabId
        ) {
          if (replacement) activateTerminalDraft(draft.quickChat, replacement);
          else activateWorkspaceFallback(draft.quickChat, closing.workspaceId);
        }
      }),
  };
}
