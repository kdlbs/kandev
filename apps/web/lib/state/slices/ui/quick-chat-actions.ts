import type { Draft } from "immer";
import { getQuickChatSetupSessionId } from "./quick-chat-session";
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
import type { QuickChatSession, UISlice } from "./types";

type ImmerSet = (recipe: (draft: Draft<UISlice>) => void) => void;

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

function openQuickChat(set: ImmerSet) {
  return (
    sessionId: string,
    workspaceId: string,
    agentProfileId?: string,
    kind: "chat" | "config" = "chat",
    taskId?: string,
  ) =>
    set((draft) => {
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
      // Only a successful open clears the dots; a rejected cross-workspace open
      // must not erase markers the user never saw.
      draft.quickChat.unseenIdleByWorkspace = {};
    });
}

function addQuickChatSession(set: ImmerSet) {
  return (
    sessionId: string,
    workspaceId: string,
    agentProfileId?: string,
    kind: "chat" | "config" = "chat",
    taskId?: string,
  ) =>
    set((draft) => {
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
      }
    });
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

export function buildQuickChatActions(set: ImmerSet) {
  return {
    ...buildQuickChatOrderActions(set),
    addQuickChatSession: addQuickChatSession(set),
    openQuickChat: openQuickChat(set),
    closeQuickChat: () =>
      set((draft) => {
        draft.quickChat.isOpen = false;
      }),
    closeQuickChatSession: (sessionId: string) =>
      set((draft) => {
        draft.quickChat = closeQuickChatSession(draft.quickChat, sessionId);
      }),
    setActiveQuickChatSession: (sessionId: string, workspaceId: string) =>
      set((draft) => {
        const session = draft.quickChat.sessions.find((item) => item.sessionId === sessionId);
        if (!session || session.workspaceId !== workspaceId) return;
        draft.quickChat.unseenIdleByWorkspace = clearMarker(
          draft.quickChat.unseenIdleByWorkspace,
          workspaceId,
          sessionId,
        );
        draft.quickChat.activeSessionId = sessionId;
        draft.quickChat.activeKind = "conversation";
      }),
    renameQuickChatSession: (sessionId: string, name: string) =>
      set((draft) => {
        const session = draft.quickChat.sessions.find((item) => item.sessionId === sessionId);
        if (!session) return;
        session.name = name;
        draft.quickChat.syncRevisionByWorkspace[session.workspaceId] =
          (draft.quickChat.syncRevisionByWorkspace[session.workspaceId] ?? 0) + 1;
      }),
    syncQuickChatSessions: (workspaceId: string, sessions: QuickChatSession[]) =>
      set((draft) => {
        draft.quickChat = reconcileQuickChatSessions(draft.quickChat, workspaceId, sessions);
      }),
    syncQuickTerminalTabs: (workspaceId: string, tabs: UISlice["quickChat"]["terminalTabs"]) =>
      set((draft) => {
        draft.quickChat = reconcileQuickTerminalTabs(draft.quickChat, workspaceId, tabs);
      }),
    upsertQuickChatSessionFromEvent: (session: QuickChatSession) =>
      set((draft) => {
        draft.quickChat = upsertQuickChatSession(draft.quickChat, session);
      }),
    removeQuickChatSessionsForTask: (taskId: string) =>
      set((draft) => {
        draft.quickChat = removeQuickChatSessionsForTask(draft.quickChat, taskId);
      }),
    removeQuickChatSession: (sessionId: string) =>
      set((draft) => {
        draft.quickChat = removeQuickChatSession(draft.quickChat, sessionId);
      }),
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
