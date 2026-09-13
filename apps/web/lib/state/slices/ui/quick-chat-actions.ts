import type { Draft } from "immer";
import {
  clearMarker,
  closeQuickChatSession,
  pruneStaleSettledLedger,
  reconcileQuickChatSessions,
  reconcileQuickTerminalTabs,
  removeQuickChatSession,
  removeQuickChatSessionsForTask,
  upsertQuickChatSession,
} from "./quick-chat-sync";
import {
  clearRememberedQuickChatSelection,
  clearRememberedQuickChatSession,
  clearSelectionOrderIfEmpty,
  rememberQuickChatSelection,
  restoreQuickChatSession,
  touchQuickChatSelectionOrder,
} from "./quick-chat-selection";
import {
  loadQuickChatSelection,
  persistQuickChatSelection,
} from "@/lib/quick-chat/selection-storage";
import { getQuickChatSetupSessionId, isQuickChatSetupSessionId } from "./quick-chat-session";
import type { QuickChatSession, QuickChatSessionKind, QuickChatState, UISlice } from "./types";

type ImmerSet = (recipe: (draft: Draft<UISlice>) => void) => void;
type ImmerGet = () => Pick<UISlice, "quickChat">;

function findWorkspaceConfigSession(
  sessions: UISlice["quickChat"]["sessions"],
  workspaceId: string,
) {
  return sessions.find(
    (session) => session.workspaceId === workspaceId && session.kind === "config",
  );
}

function upsertQuickChatSessionDraft(
  quickChat: Draft<UISlice["quickChat"]>,
  session: QuickChatSession,
): boolean {
  const { sessionId, workspaceId, agentProfileId, kind, taskId } = session;
  const existing = quickChat.sessions.find((session) => session.sessionId === sessionId);
  if (existing) {
    if (existing.workspaceId !== workspaceId) return false;
    if (agentProfileId) existing.agentProfileId = agentProfileId;
    if (taskId) existing.taskId = taskId;
  } else {
    quickChat.sessions.push({ sessionId, workspaceId, agentProfileId, kind, taskId });
  }
  const ownedTaskId = taskId ?? existing?.taskId ?? quickChat.sessionOwnership[sessionId]?.taskId;
  quickChat.sessionOwnership[sessionId] = { workspaceId, taskId: ownedTaskId };
  quickChat.syncRevisionByWorkspace[workspaceId] =
    (quickChat.syncRevisionByWorkspace[workspaceId] ?? 0) + 1;
  return true;
}

function persistRememberedSelection(get: ImmerGet): void {
  const quickChat = get().quickChat;
  persistQuickChatSelection(
    quickChat.selectionStorageIdentity,
    quickChat.rememberedSelectionByWorkspace,
    quickChat.rememberedSelectionOrder,
  );
}

function bumpSelectionRevision(quickChat: Draft<QuickChatState>, workspaceId: string): void {
  quickChat.selectionRevisionByWorkspace[workspaceId] =
    (quickChat.selectionRevisionByWorkspace[workspaceId] ?? 0) + 1;
}

function rememberSelectedSession(
  quickChat: Draft<QuickChatState>,
  session: QuickChatSession,
): void {
  const kind = session.kind ?? "chat";
  bumpSelectionRevision(quickChat, session.workspaceId);
  if (isQuickChatSetupSessionId(session.sessionId)) return;
  quickChat.rememberedSelectionByWorkspace = rememberQuickChatSelection(
    quickChat.rememberedSelectionByWorkspace,
    session.workspaceId,
    kind,
    session.sessionId,
  );
  quickChat.rememberedSelectionOrder = touchQuickChatSelectionOrder(
    quickChat.rememberedSelectionOrder,
    session.workspaceId,
  );
}

function clearRememberedSession(
  quickChat: Draft<QuickChatState>,
  sessions: QuickChatSession[],
  sessionId: string,
): void {
  const session = sessions.find((item) => item.sessionId === sessionId);
  if (!session) return;
  const next = clearRememberedQuickChatSession(
    quickChat.rememberedSelectionByWorkspace,
    sessions,
    sessionId,
  );
  if (next === quickChat.rememberedSelectionByWorkspace) return;
  quickChat.rememberedSelectionByWorkspace = next;
  quickChat.rememberedSelectionOrder = clearSelectionOrderIfEmpty(
    quickChat.rememberedSelectionOrder,
    next,
  );
  bumpSelectionRevision(quickChat, session.workspaceId);
}

function clearMissingRememberedSelections(
  quickChat: Draft<QuickChatState>,
  workspaceId: string,
  serverSessions: QuickChatSession[],
): void {
  const serverIdsByKind = new Map<QuickChatSessionKind, Set<string>>();
  for (const kind of ["chat", "config"] as const) {
    serverIdsByKind.set(
      kind,
      new Set(
        serverSessions
          .filter((session) => (session.kind ?? "chat") === kind)
          .map((session) => session.sessionId),
      ),
    );
  }
  for (const kind of ["chat", "config"] as const) {
    const selected = quickChat.rememberedSelectionByWorkspace[workspaceId]?.[kind];
    if (!selected || serverIdsByKind.get(kind)?.has(selected)) continue;
    const next = clearRememberedQuickChatSelection(
      quickChat.rememberedSelectionByWorkspace,
      workspaceId,
      kind,
    );
    quickChat.rememberedSelectionByWorkspace = next;
    quickChat.rememberedSelectionOrder = clearSelectionOrderIfEmpty(
      quickChat.rememberedSelectionOrder,
      next,
    );
  }
}

function resolvePendingOpen(
  quickChat: Draft<QuickChatState>,
  workspaceId: string,
  sessions: QuickChatSession[],
): void {
  const pending = quickChat.pendingOpen;
  if (
    !pending ||
    pending.workspaceId !== workspaceId ||
    pending.selectionRevision !== (quickChat.selectionRevisionByWorkspace[workspaceId] ?? 0)
  )
    return;

  const sessionId = restoreQuickChatSession(
    sessions,
    quickChat.rememberedSelectionByWorkspace,
    workspaceId,
    pending.kind,
    pending.tabOrder ?? quickChat.tabOrderByWorkspace[workspaceId],
  );
  if (sessionId) {
    quickChat.activeSessionId = sessionId;
    quickChat.activeKind = "conversation";
  } else {
    const setupSessionId = getQuickChatSetupSessionId(workspaceId, pending.kind);
    if (!quickChat.sessions.some((session) => session.sessionId === setupSessionId)) {
      quickChat.sessions.push({ sessionId: setupSessionId, workspaceId, kind: pending.kind });
    }
    quickChat.activeSessionId = setupSessionId;
    quickChat.activeKind = "conversation";
  }
  quickChat.isOpen = true;
  quickChat.pendingOpen = null;
}

function openQuickChat(set: ImmerSet, get: ImmerGet) {
  return (
    sessionId: string,
    workspaceId: string,
    agentProfileId?: string,
    kind: "chat" | "config" = "chat",
    taskId?: string,
  ) => {
    set((draft) => {
      draft.quickChat.pendingOpen = null;
      if (!sessionId) {
        const existing =
          kind === "config"
            ? findWorkspaceConfigSession(draft.quickChat.sessions, workspaceId)
            : undefined;
        const setupSessionId = getQuickChatSetupSessionId(workspaceId, kind);
        if (
          !existing &&
          !draft.quickChat.sessions.some((session) => session.sessionId === setupSessionId)
        ) {
          draft.quickChat.sessions.push({ sessionId: setupSessionId, workspaceId, kind });
        }
        draft.quickChat.isOpen = true;
        draft.quickChat.activeSessionId = existing?.sessionId ?? setupSessionId;
        draft.quickChat.activeKind = "conversation";
        bumpSelectionRevision(draft.quickChat, workspaceId);
        // The dialog is open now, so every workspace's dots are obsolete.
        draft.quickChat.unseenIdleByWorkspace = {};
        return;
      }
      if (
        !upsertQuickChatSessionDraft(draft.quickChat, {
          sessionId,
          workspaceId,
          agentProfileId,
          kind,
          taskId,
        })
      )
        return;
      draft.quickChat.isOpen = true;
      draft.quickChat.activeSessionId = sessionId;
      draft.quickChat.activeKind = "conversation";
      const session = draft.quickChat.sessions.find((item) => item.sessionId === sessionId);
      if (session) rememberSelectedSession(draft.quickChat, session);
      // Only a successful open clears the dots; a rejected cross-workspace open
      // must not erase markers the user never saw.
      draft.quickChat.unseenIdleByWorkspace = {};
    });
    persistRememberedSelection(get);
  };
}

function addQuickChatSession(set: ImmerSet, get: ImmerGet) {
  return (
    sessionId: string,
    workspaceId: string,
    agentProfileId?: string,
    kind: "chat" | "config" = "chat",
    taskId?: string,
  ) => {
    set((draft) => {
      draft.quickChat.pendingOpen = null;
      const activeWorkspaceId = draft.quickChat.sessions.find(
        (session) => session.sessionId === draft.quickChat.activeSessionId,
      )?.workspaceId;
      if (
        !upsertQuickChatSessionDraft(draft.quickChat, {
          sessionId,
          workspaceId,
          agentProfileId,
          kind,
          taskId,
        })
      )
        return;
      if (!draft.quickChat.isOpen || !activeWorkspaceId || activeWorkspaceId === workspaceId) {
        draft.quickChat.activeSessionId = sessionId;
        draft.quickChat.activeKind = "conversation";
        const active = draft.quickChat.sessions.find((item) => item.sessionId === sessionId);
        if (active) rememberSelectedSession(draft.quickChat, active);
      }
    });
    persistRememberedSelection(get);
  };
}

function buildQuickChatOrderActions(set: ImmerSet) {
  return {
    setQuickChatTabOrder: (workspaceId: string, order: string[]) =>
      set((draft) => {
        draft.quickChat.tabOrderByWorkspace[workspaceId] = [...order];
      }),
    clearQuickChatTabOrder: (workspaceId: string, expectedOrder: string[]) =>
      set((draft) => {
        const current = draft.quickChat.tabOrderByWorkspace[workspaceId];
        if (
          !current ||
          current.length !== expectedOrder.length ||
          current.some((reference, index) => reference !== expectedOrder[index])
        ) {
          return;
        }
        delete draft.quickChat.tabOrderByWorkspace[workspaceId];
      }),
    setQuickChatTabOrderSyncState: (
      workspaceId: string,
      state: { pending: boolean; error: string | null },
    ) =>
      set((draft) => {
        draft.quickChat.tabOrderSyncPendingByWorkspace[workspaceId] = state.pending;
        draft.quickChat.tabOrderSyncErrorByWorkspace[workspaceId] = state.error;
      }),
  };
}

function buildQuickChatSessionActions(set: ImmerSet, get: ImmerGet) {
  return {
    addQuickChatSession: addQuickChatSession(set, get),
    openQuickChat: openQuickChat(set, get),
    closeQuickChat: () =>
      set((draft) => {
        draft.quickChat.isOpen = false;
        if (draft.quickChat.pendingOpen) {
          bumpSelectionRevision(draft.quickChat, draft.quickChat.pendingOpen.workspaceId);
          draft.quickChat.pendingOpen = null;
        }
      }),
    closeQuickChatSession: (sessionId: string) => {
      const sessions = get().quickChat.sessions;
      set((draft) => {
        draft.quickChat = closeQuickChatSession(draft.quickChat, sessionId);
        clearRememberedSession(draft.quickChat, sessions, sessionId);
      });
      persistRememberedSelection(get);
    },
    setActiveQuickChatSession: (sessionId: string, workspaceId: string) => {
      let selected = false;
      set((draft) => {
        const session = draft.quickChat.sessions.find((item) => item.sessionId === sessionId);
        if (!session || session.workspaceId !== workspaceId) return;
        draft.quickChat.unseenIdleByWorkspace = clearMarker(
          draft.quickChat.unseenIdleByWorkspace,
          workspaceId,
          sessionId,
        );
        draft.quickChat.pendingOpen = null;
        draft.quickChat.activeSessionId = sessionId;
        draft.quickChat.activeKind = "conversation";
        rememberSelectedSession(draft.quickChat, session);
        selected = true;
      });
      if (selected) persistRememberedSelection(get);
    },
    renameQuickChatSession: (sessionId: string, name: string) =>
      set((draft) => {
        const session = draft.quickChat.sessions.find((item) => item.sessionId === sessionId);
        if (!session) return;
        session.name = name;
        draft.quickChat.syncRevisionByWorkspace[session.workspaceId] =
          (draft.quickChat.syncRevisionByWorkspace[session.workspaceId] ?? 0) + 1;
      }),
    syncQuickChatSessions: (workspaceId: string, sessions: QuickChatSession[]) => {
      set((draft) => {
        draft.quickChat = reconcileQuickChatSessions(draft.quickChat, workspaceId, sessions);
        clearMissingRememberedSelections(draft.quickChat, workspaceId, sessions);
        draft.quickChat.selectionReadyByWorkspace[workspaceId] = true;
        resolvePendingOpen(draft.quickChat, workspaceId, sessions);
      });
      persistRememberedSelection(get);
    },
    syncQuickTerminalTabs: (workspaceId: string, tabs: UISlice["quickChat"]["terminalTabs"]) =>
      set((draft) => {
        draft.quickChat = reconcileQuickTerminalTabs(draft.quickChat, workspaceId, tabs);
      }),
    upsertQuickChatSessionFromEvent: (session: QuickChatSession) =>
      set((draft) => {
        draft.quickChat = upsertQuickChatSession(draft.quickChat, session);
      }),
    removeQuickChatSessionsForTask: (taskId: string) => {
      const sessions = get().quickChat.sessions.filter((session) => session.taskId === taskId);
      set((draft) => {
        draft.quickChat = removeQuickChatSessionsForTask(draft.quickChat, taskId);
        for (const session of sessions)
          clearRememberedSession(draft.quickChat, sessions, session.sessionId);
      });
      persistRememberedSelection(get);
    },
    removeQuickChatSession: (sessionId: string) => {
      const sessions = get().quickChat.sessions;
      set((draft) => {
        draft.quickChat = removeQuickChatSession(draft.quickChat, sessionId);
        clearRememberedSession(draft.quickChat, sessions, sessionId);
      });
      persistRememberedSelection(get);
    },
  };
}

function buildQuickChatActivityActions(set: ImmerSet) {
  return {
    markQuickChatUnseenIdle: (sessionId: string, workspaceId: string) =>
      set((draft) => {
        draft.quickChat.unseenIdleByWorkspace[workspaceId] = {
          ...draft.quickChat.unseenIdleByWorkspace[workspaceId],
          [sessionId]: true,
        };
      }),
    clearQuickChatUnseenIdle: (sessionId?: string, workspaceId?: string) =>
      set((draft) => {
        if (!sessionId || !workspaceId) {
          draft.quickChat.unseenIdleByWorkspace = {};
          return;
        }
        draft.quickChat.unseenIdleByWorkspace = clearMarker(
          draft.quickChat.unseenIdleByWorkspace,
          workspaceId,
          sessionId,
        );
      }),
    recordQuickChatSettled: (sessionId: string, updatedAt: string) => {
      let recorded = false;
      set((draft) => {
        if (!updatedAt) return;
        // Compare parsed epochs, not strings: an exact-second value ("...00Z")
        // must not be treated as newer than a fractional one ("...00.1Z").
        const updatedTime = Date.parse(updatedAt);
        if (Number.isNaN(updatedTime)) return;
        const existing = draft.quickChat.lastSettledAtBySession[sessionId];
        if (existing) {
          const existingTime = Date.parse(existing);
          if (!Number.isNaN(existingTime) && updatedTime <= existingTime) return;
        }
        draft.quickChat.lastSettledAtBySession = pruneStaleSettledLedger({
          ...draft.quickChat.lastSettledAtBySession,
          [sessionId]: updatedAt,
        });
        recorded = true;
      });
      return recorded;
    },
    setQuickChatInitialPrompt: (sessionId: string, prompt?: string) =>
      set((draft) => {
        const session = draft.quickChat.sessions.find((item) => item.sessionId === sessionId);
        if (session) session.initialPrompt = prompt;
      }),
  };
}

function buildQuickChatSelectionActions(set: ImmerSet) {
  return {
    requestQuickChatOpen: (
      workspaceId: string,
      kind: QuickChatSessionKind = "chat",
      tabOrder?: string[],
    ) =>
      set((draft) => {
        const selectionRevision = draft.quickChat.selectionRevisionByWorkspace[workspaceId] ?? 0;
        draft.quickChat.pendingOpen = {
          workspaceId,
          kind,
          selectionRevision,
          ...(tabOrder ? { tabOrder: [...tabOrder] } : {}),
        };
        draft.quickChat.isOpen = true;
        draft.quickChat.activeKind = "conversation";
        draft.quickChat.activeSessionId = null;
        draft.quickChat.activeTerminalTabId = null;
        draft.quickChat.unseenIdleByWorkspace = {};
      }),
    setQuickChatSelectionIdentity: (identity: string | null) =>
      set((draft) => {
        if (draft.quickChat.selectionStorageIdentity === identity) return;
        const loaded = loadQuickChatSelection(identity);
        draft.quickChat.selectionStorageIdentity = identity;
        draft.quickChat.rememberedSelectionByWorkspace = loaded.selections;
        draft.quickChat.rememberedSelectionOrder = loaded.order;
        draft.quickChat.selectionReadyByWorkspace = {};
        draft.quickChat.selectionRevisionByWorkspace = {};
        draft.quickChat.pendingOpen = null;
      }),
  };
}

export function buildQuickChatActions(set: ImmerSet, get: ImmerGet) {
  return {
    ...buildQuickChatOrderActions(set),
    ...buildQuickChatSessionActions(set, get),
    ...buildQuickChatActivityActions(set),
    ...buildQuickChatSelectionActions(set),
  };
}
