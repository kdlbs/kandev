import { getStoredQuickChatNames } from "@/lib/local-storage";
import { isQuickChatSetupSessionId } from "./quick-chat-session";
import type { QuickChatSession, QuickChatState, QuickTerminalTab } from "./types";

/**
 * Applies locally stored tab renames over a server-provided session list.
 *
 * Renames live in localStorage, so the backend task title is only a fallback.
 * Boot hydration and runtime resync both go through here so a reload and a
 * WebSocket-driven refresh label the same tab the same way.
 */
export function applyStoredQuickChatNames(sessions: QuickChatSession[]): QuickChatSession[] {
  const storedNames = getStoredQuickChatNames();
  return sessions.map((session) => {
    const localName = storedNames[session.sessionId];
    return {
      ...session,
      kind: session.kind ?? "chat",
      ...(localName ? { name: localName } : {}),
    };
  });
}

/**
 * Reconciles one workspace's quick-chat tabs against the server's list.
 *
 * Quick chats are shared state: they are created and closed from any device,
 * but a client only ever learned about them from its own boot payload. Without
 * this reconcile, two long-lived clients drift — each keeps tabs the other
 * never saw, and keeps tabs whose task the other already deleted.
 *
 * Membership is the server's call; position is not. Tabs already on screen keep
 * their order and new ones are appended, because the server sorts by latest
 * activity and that changes as sessions run — adopting it wholesale would
 * reshuffle the strip under the user's cursor on every reconnect. The activity
 * order still decides the initial restore, when there is nothing on screen yet.
 *
 * Local-only state is preserved: unstarted "New chat" setup tabs (which have no
 * backing task yet) and per-session drafts on tabs that survive.
 */
export function reconcileQuickChatSessions(
  state: QuickChatState,
  workspaceId: string,
  serverSessions: QuickChatSession[],
): QuickChatState {
  const otherWorkspaces = state.sessions.filter((session) => session.workspaceId !== workspaceId);
  const localSetupTabs = state.sessions.filter(
    (session) =>
      session.workspaceId === workspaceId && isQuickChatSetupSessionId(session.sessionId),
  );
  const named = applyStoredQuickChatNames(serverSessions);
  const serverById = new Map(named.map((session) => [session.sessionId, session]));

  // Tabs still on the server, in the order this client already shows them.
  const survivors = state.sessions
    .filter(
      (session) => session.workspaceId === workspaceId && serverById.has(session.sessionId),
      // Client-only fields (e.g. an unsent draft) survive via the spread below.
    )
    .map((session) => ({ ...session, ...serverById.get(session.sessionId) }));
  const survivorIds = new Set(survivors.map((session) => session.sessionId));
  const added = named.filter((session) => !survivorIds.has(session.sessionId));

  return withValidActiveSession(state, [
    ...otherWorkspaces,
    ...survivors,
    ...added,
    ...localSetupTabs,
  ]);
}

/**
 * Reconciles one workspace's server-owned terminal descriptors. A local tab
 * that is still creating its descriptor is retained briefly so a socket
 * reconnect cannot erase it before the POST response lands; every established
 * descriptor follows the server list, including deletions from another client.
 */
export function reconcileQuickTerminalTabs(
  state: QuickChatState,
  workspaceId: string,
  serverTabs: QuickTerminalTab[],
): QuickChatState {
  const otherWorkspaces = state.terminalTabs.filter((tab) => tab.workspaceId !== workspaceId);
  const serverById = new Map(serverTabs.map((tab) => [tab.tabId, tab]));
  const pendingLocal = state.terminalTabs.filter(
    (tab) =>
      tab.workspaceId === workspaceId &&
      !serverById.has(tab.tabId) &&
      tab.status === "connecting" &&
      !tab.sessionId,
  );
  const nextTabs = [
    ...otherWorkspaces,
    ...[...serverTabs].sort((a, b) => a.sequence - b.sequence),
    ...pendingLocal,
  ];
  const validIds = new Set(nextTabs.map((tab) => tab.tabId));
  const lastTerminalTabIdByWorkspace = { ...state.lastTerminalTabIdByWorkspace };
  for (const [workspace, tabId] of Object.entries(lastTerminalTabIdByWorkspace)) {
    if (!validIds.has(tabId)) delete lastTerminalTabIdByWorkspace[workspace];
  }

  const next: QuickChatState = {
    ...state,
    terminalTabs: nextTabs,
    lastTerminalTabIdByWorkspace,
  };
  if (next.activeKind !== "terminal") return next;

  const active = nextTabs.find((tab) => tab.tabId === next.activeTerminalTabId);
  if (active) {
    next.lastTerminalTabIdByWorkspace[active.workspaceId] = active.tabId;
    return next;
  }

  const previousWorkspaceId =
    state.terminalTabs.find((tab) => tab.tabId === state.activeTerminalTabId)?.workspaceId ??
    workspaceId;
  const replacement = nextTabs
    .filter((tab) => tab.workspaceId === previousWorkspaceId)
    .sort((a, b) => a.sequence - b.sequence)[0];
  if (replacement) {
    next.activeTerminalTabId = replacement.tabId;
    next.lastTerminalTabIdByWorkspace[replacement.workspaceId] = replacement.tabId;
    return next;
  }

  const conversation = next.sessions.find((session) => session.workspaceId === previousWorkspaceId);
  next.activeKind = "conversation";
  next.activeTerminalTabId = null;
  next.activeSessionId = conversation?.sessionId ?? null;
  next.isOpen = conversation ? next.isOpen : false;
  return next;
}

/**
 * Adds or updates a single quick-chat tab observed on the wire.
 *
 * Passive by design: it never changes which tab is active or opens the modal,
 * because the event describes something the user did on another device.
 */
export function upsertQuickChatSession(
  state: QuickChatState,
  session: QuickChatSession,
): QuickChatState {
  const [named] = applyStoredQuickChatNames([session]);
  const index = state.sessions.findIndex((item) => item.sessionId === named.sessionId);
  if (index === -1) {
    return { ...state, sessions: [...state.sessions, named] };
  }
  const sessions = [...state.sessions];
  sessions[index] = { ...sessions[index], ...named };
  return { ...state, sessions };
}

/** Drops the tabs backed by a task that no longer exists. */
export function removeQuickChatSessionsForTask(
  state: QuickChatState,
  taskId: string,
): QuickChatState {
  const remaining = state.sessions.filter((session) => session.taskId !== taskId);
  if (remaining.length === state.sessions.length) return state;
  return withValidActiveSession(state, remaining);
}

/**
 * Re-points `activeSessionId` when the tab it named is gone, mirroring what a
 * local tab close does: fall back to another tab in the same workspace, and
 * close the modal only when nothing is left to show.
 */
function preserveTerminalSelection(
  state: QuickChatState,
  sessions: QuickChatSession[],
): QuickChatState | null {
  if (state.activeKind !== "terminal") return null;
  const active = state.terminalTabs.find((tab) => tab.tabId === state.activeTerminalTabId);
  if (active) return { ...state, sessions };

  const previousWorkspaceId =
    Object.entries(state.lastTerminalTabIdByWorkspace).find(
      ([, tabId]) => tabId === state.activeTerminalTabId,
    )?.[0] ??
    state.terminalTabs.find((tab) => tab.tabId === state.activeTerminalTabId)?.workspaceId;
  const terminal = state.terminalTabs.find(
    (tab) => !previousWorkspaceId || tab.workspaceId === previousWorkspaceId,
  );
  if (!terminal) {
    const conversation =
      sessions.find((session) => session.workspaceId === previousWorkspaceId) ??
      sessions.find((session) => session.sessionId === state.activeSessionId) ??
      sessions[0];
    return {
      ...state,
      sessions,
      activeKind: "conversation",
      activeTerminalTabId: null,
      activeSessionId: conversation?.sessionId ?? null,
      isOpen: conversation ? state.isOpen : false,
    };
  }
  return {
    ...state,
    sessions,
    activeTerminalTabId: terminal.tabId,
    lastTerminalTabIdByWorkspace: {
      ...state.lastTerminalTabIdByWorkspace,
      [terminal.workspaceId]: terminal.tabId,
    },
  };
}

function withValidActiveSession(
  state: QuickChatState,
  sessions: QuickChatSession[],
): QuickChatState {
  const terminalSelection = preserveTerminalSelection(state, sessions);
  if (terminalSelection) return terminalSelection;
  const active = state.activeSessionId;
  // No tab was ever selected: leave it unset rather than silently promoting one
  // on a background resync. Same invariant `hydrateUI` guards on.
  if (!active) return { ...state, sessions };
  if (sessions.some((session) => session.sessionId === active)) {
    return { ...state, sessions };
  }
  const previousWorkspaceId =
    state.sessions.find((session) => session.sessionId === active)?.workspaceId ??
    state.terminalTabs.find((tab) => tab.tabId === state.activeTerminalTabId)?.workspaceId;
  const fallback =
    sessions.find((session) => session.workspaceId === previousWorkspaceId) ?? sessions[0];
  const terminalFallback = state.terminalTabs.find(
    (tab) => tab.workspaceId === previousWorkspaceId,
  );
  if (!fallback && terminalFallback) {
    return {
      ...state,
      sessions,
      activeKind: "terminal",
      activeTerminalTabId: terminalFallback.tabId,
      isOpen: state.isOpen,
    };
  }
  return {
    ...state,
    sessions,
    activeSessionId: fallback?.sessionId ?? null,
    activeKind: "conversation",
    activeTerminalTabId: null,
    isOpen: fallback ? state.isOpen : false,
  };
}
