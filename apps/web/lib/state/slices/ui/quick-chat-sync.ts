import { getStoredQuickChatNames } from "@/lib/local-storage";
import { isQuickChatSetupSessionId } from "./quick-chat-session";
import type { QuickChatSession, QuickChatState } from "./types";

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
function withValidActiveSession(
  state: QuickChatState,
  sessions: QuickChatSession[],
): QuickChatState {
  const active = state.activeSessionId;
  // No tab was ever selected: leave it unset rather than silently promoting one
  // on a background resync. Same invariant `hydrateUI` guards on.
  if (!active) return { ...state, sessions };
  if (sessions.some((session) => session.sessionId === active)) {
    return { ...state, sessions };
  }
  const previousWorkspaceId = state.sessions.find(
    (session) => session.sessionId === active,
  )?.workspaceId;
  const fallback =
    sessions.find((session) => session.workspaceId === previousWorkspaceId) ?? sessions[0];
  return {
    ...state,
    sessions,
    activeSessionId: fallback?.sessionId ?? null,
    isOpen: fallback ? state.isOpen : false,
  };
}
