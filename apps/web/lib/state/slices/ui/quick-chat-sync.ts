import { getStoredQuickChatNames } from "@/lib/local-storage";
import { isQuickChatSetupSessionId } from "./quick-chat-session";
import type { QuickChatSession, QuickChatState, QuickTerminalTab } from "./types";

const QUICK_CHAT_LIFECYCLE_RETENTION_MS = 60 * 60 * 1000;
const SETTLED_LEDGER_MAX_ENTRIES = 500;

/** Applies locally stored tab renames over a server-provided session list. */
export function applyStoredQuickChatNames(sessions: QuickChatSession[]): QuickChatSession[] {
  const storedNames = getStoredQuickChatNames();
  return sessions.map((session) => {
    const localName = storedNames[session.sessionId];
    return { ...session, kind: session.kind ?? "chat", ...(localName ? { name: localName } : {}) };
  });
}

export function pruneStaleSettledLedger(
  bySession: Record<string, string>,
  now = Date.now(),
): Record<string, string> {
  const retained = Object.entries(bySession)
    .filter(([, updatedAt]) => {
      const timestamp = Date.parse(updatedAt);
      return Number.isFinite(timestamp) && now - timestamp <= QUICK_CHAT_LIFECYCLE_RETENTION_MS;
    })
    .sort(([, left], [, right]) => Date.parse(right) - Date.parse(left))
    .slice(0, SETTLED_LEDGER_MAX_ENTRIES);
  return Object.fromEntries(retained);
}

function pruneTombstones(
  tombstones: QuickChatState["tombstonedSessions"],
  now = Date.now(),
): QuickChatState["tombstonedSessions"] {
  return Object.fromEntries(
    Object.entries(tombstones).filter(([, tombstone]) => {
      const timestamp = Date.parse(tombstone.tombstonedAt);
      return Number.isFinite(timestamp) && now - timestamp <= QUICK_CHAT_LIFECYCLE_RETENTION_MS;
    }),
  );
}

function indexSessionOwnership(
  ownership: QuickChatState["sessionOwnership"],
  sessions: QuickChatSession[],
): QuickChatState["sessionOwnership"] {
  const next = { ...ownership };
  for (const session of sessions) {
    if (!isQuickChatSetupSessionId(session.sessionId)) {
      next[session.sessionId] = { workspaceId: session.workspaceId, taskId: session.taskId };
    }
  }
  return next;
}

function withRevision(state: QuickChatState, workspaceId: string): QuickChatState {
  return {
    ...state,
    syncRevisionByWorkspace: {
      ...state.syncRevisionByWorkspace,
      [workspaceId]: (state.syncRevisionByWorkspace[workspaceId] ?? 0) + 1,
    },
  };
}

export function clearMarker(
  byWorkspace: QuickChatState["unseenIdleByWorkspace"],
  workspaceId: string,
  sessionId: string,
): QuickChatState["unseenIdleByWorkspace"] {
  const markers = byWorkspace[workspaceId];
  if (!markers?.[sessionId]) return byWorkspace;
  const { [sessionId]: _removed, ...remaining } = markers;
  if (Object.keys(remaining).length === 0) {
    const { [workspaceId]: _workspace, ...otherWorkspaces } = byWorkspace;
    return otherWorkspaces;
  }
  return { ...byWorkspace, [workspaceId]: remaining };
}

function withLifecyclePruning(
  state: QuickChatState,
  removedIds: Set<string>,
  tombstone: boolean,
): QuickChatState {
  if (removedIds.size === 0) return state;
  let markers = state.unseenIdleByWorkspace;
  const ownership = { ...state.sessionOwnership };
  const tombstones = { ...state.tombstonedSessions };
  const lastSettledAtBySession = { ...state.lastSettledAtBySession };
  const sessions = state.sessions.filter((session) => !removedIds.has(session.sessionId));
  for (const sessionId of removedIds) {
    const session = state.sessions.find((item) => item.sessionId === sessionId);
    const owner = session
      ? { workspaceId: session.workspaceId, taskId: session.taskId }
      : ownership[sessionId];
    if (!owner) continue;
    markers = clearMarker(markers, owner.workspaceId, sessionId);
    delete ownership[sessionId];
    delete lastSettledAtBySession[sessionId];
    if (tombstone && !isQuickChatSetupSessionId(sessionId)) {
      tombstones[sessionId] = {
        workspaceId: owner.workspaceId,
        tombstonedAt: new Date().toISOString(),
      };
    }
  }
  const next = withValidActiveSession(
    {
      ...state,
      unseenIdleByWorkspace: markers,
      lastSettledAtBySession,
      sessionOwnership: ownership,
      tombstonedSessions: pruneTombstones(tombstones),
    },
    sessions,
  );
  return { ...next, lastSettledAtBySession: pruneStaleSettledLedger(next.lastSettledAtBySession) };
}

/** Reconciles one workspace's quick-chat tabs against the server's authoritative list. */
export function reconcileQuickChatSessions(
  state: QuickChatState,
  workspaceId: string,
  serverSessions: QuickChatSession[],
): QuickChatState {
  const named = applyStoredQuickChatNames(serverSessions);
  const serverIds = new Set(named.map((session) => session.sessionId));
  const knownIds = new Set([
    ...state.sessions
      .filter((session) => session.workspaceId === workspaceId)
      .map((session) => session.sessionId),
    ...Object.entries(state.sessionOwnership)
      .filter(([, owner]) => owner.workspaceId === workspaceId)
      .map(([sessionId]) => sessionId),
  ]);
  const omittedIds = new Set(
    [...knownIds].filter(
      (sessionId) => !serverIds.has(sessionId) && !isQuickChatSetupSessionId(sessionId),
    ),
  );
  const clearedTombstones = Object.fromEntries(
    Object.entries(state.tombstonedSessions).filter(
      ([sessionId, tombstone]) =>
        tombstone.workspaceId !== workspaceId || !serverIds.has(sessionId),
    ),
  );
  const withoutOmitted = withLifecyclePruning(
    { ...state, tombstonedSessions: clearedTombstones },
    omittedIds,
    true,
  );
  const otherWorkspaces = withoutOmitted.sessions.filter(
    (session) => session.workspaceId !== workspaceId,
  );
  const setupTabs = withoutOmitted.sessions.filter(
    (session) =>
      session.workspaceId === workspaceId && isQuickChatSetupSessionId(session.sessionId),
  );
  const serverById = new Map(named.map((session) => [session.sessionId, session]));
  const survivors = withoutOmitted.sessions
    .filter((session) => session.workspaceId === workspaceId && serverById.has(session.sessionId))
    .map((session) => ({ ...session, ...serverById.get(session.sessionId) }));
  const survivorIds = new Set(survivors.map((session) => session.sessionId));
  const sessions = [
    ...otherWorkspaces,
    ...survivors,
    ...named.filter((session) => !survivorIds.has(session.sessionId)),
    ...setupTabs,
  ];
  const next = withValidActiveSession(withoutOmitted, sessions);
  return withRevision(
    { ...next, sessionOwnership: indexSessionOwnership(next.sessionOwnership, sessions) },
    workspaceId,
  );
}

/** Merges boot hydration without removing tabs or overwriting newer local fields. */
export function mergeHydratedQuickChatSessions(
  state: QuickChatState,
  sessions: QuickChatSession[],
): QuickChatState {
  const tombstonedSessions = pruneTombstones(state.tombstonedSessions);
  const hydrated = applyStoredQuickChatNames(sessions).filter(
    (session) => !tombstonedSessions[session.sessionId],
  );
  const existingIds = new Set(state.sessions.map((session) => session.sessionId));
  const added = hydrated.filter((session) => !existingIds.has(session.sessionId));
  return {
    ...state,
    sessions: [...state.sessions, ...added],
    sessionOwnership: indexSessionOwnership(state.sessionOwnership, added),
    tombstonedSessions,
  };
}

/** Adds or updates a tab observed on the wire without stealing focus. */
export function upsertQuickChatSession(
  state: QuickChatState,
  session: QuickChatSession,
): QuickChatState {
  if (state.tombstonedSessions[session.sessionId]) {
    // A tombstone expires after its retention window even when no later
    // removal or reconciliation prunes it; otherwise the rejection below
    // would discard this session forever.
    const tombstones = pruneTombstones(state.tombstonedSessions);
    if (tombstones[session.sessionId]) return state;
    state = { ...state, tombstonedSessions: tombstones };
  }
  const [named] = applyStoredQuickChatNames([session]);
  const index = state.sessions.findIndex((item) => item.sessionId === named.sessionId);
  const sessions =
    index === -1
      ? [...state.sessions, named]
      : state.sessions.map((item, itemIndex) =>
          itemIndex === index ? { ...item, ...named } : item,
        );
  return withRevision(
    {
      ...state,
      sessions,
      sessionOwnership: indexSessionOwnership(state.sessionOwnership, [named]),
    },
    named.workspaceId,
  );
}

/** Drops every known quick-chat session backed by a deleted task. */
export function removeQuickChatSessionsForTask(
  state: QuickChatState,
  taskId: string,
): QuickChatState {
  const removedIds = new Set(
    Object.entries(state.sessionOwnership)
      .filter(([, owner]) => owner.taskId === taskId)
      .map(([sessionId]) => sessionId),
  );
  for (const session of state.sessions)
    if (session.taskId === taskId) removedIds.add(session.sessionId);
  const next = withLifecyclePruning(state, removedIds, true);
  const workspaceIds = new Set(
    [...removedIds].map(
      (sessionId) =>
        state.sessionOwnership[sessionId]?.workspaceId ??
        state.sessions.find((session) => session.sessionId === sessionId)?.workspaceId,
    ),
  );
  return [...workspaceIds]
    .filter((workspaceId): workspaceId is string => Boolean(workspaceId))
    .reduce(withRevision, next);
}

/** Removes one server-backed quick-chat session and blocks late lifecycle events. */
export function removeQuickChatSession(state: QuickChatState, sessionId: string): QuickChatState {
  const session = state.sessions.find((item) => item.sessionId === sessionId);
  const owner = session
    ? { workspaceId: session.workspaceId, taskId: session.taskId }
    : state.sessionOwnership[sessionId];
  if (!owner) return state;
  return withRevision(withLifecyclePruning(state, new Set([sessionId]), true), owner.workspaceId);
}

/** Closes a local quick-chat tab without creating a server-event tombstone. */
export function closeQuickChatSession(state: QuickChatState, sessionId: string): QuickChatState {
  return withLifecyclePruning(state, new Set([sessionId]), false);
}

/** Reconciles one workspace's server-owned terminal descriptors. */
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
  const lastTerminalTabIdByWorkspace = Object.fromEntries(
    Object.entries(state.lastTerminalTabIdByWorkspace).filter(([, tabId]) => validIds.has(tabId)),
  );
  const next = { ...state, terminalTabs: nextTabs, lastTerminalTabIdByWorkspace };
  if (next.activeKind !== "terminal") return next;
  const active = nextTabs.find((tab) => tab.tabId === next.activeTerminalTabId);
  if (active)
    return {
      ...next,
      lastTerminalTabIdByWorkspace: {
        ...lastTerminalTabIdByWorkspace,
        [active.workspaceId]: active.tabId,
      },
    };
  const replacement = nextTabs
    .filter((tab) => tab.workspaceId === workspaceId)
    .sort((a, b) => a.sequence - b.sequence)[0];
  if (replacement)
    return {
      ...next,
      activeTerminalTabId: replacement.tabId,
      lastTerminalTabIdByWorkspace: {
        ...lastTerminalTabIdByWorkspace,
        [replacement.workspaceId]: replacement.tabId,
      },
    };
  const conversation = next.sessions.find((session) => session.workspaceId === workspaceId);
  return {
    ...next,
    activeKind: "conversation",
    activeTerminalTabId: null,
    activeSessionId: conversation?.sessionId ?? null,
    isOpen: conversation ? next.isOpen : false,
  };
}

function preserveTerminalSelection(
  state: QuickChatState,
  sessions: QuickChatSession[],
): QuickChatState | null {
  if (state.activeKind !== "terminal") return null;
  const active = state.terminalTabs.find((tab) => tab.tabId === state.activeTerminalTabId);
  if (active) return { ...state, sessions };
  const workspaceId = Object.entries(state.lastTerminalTabIdByWorkspace).find(
    ([, tabId]) => tabId === state.activeTerminalTabId,
  )?.[0];
  const terminal = state.terminalTabs.find(
    (tab) => !workspaceId || tab.workspaceId === workspaceId,
  );
  if (!terminal) {
    const conversation =
      sessions.find((session) => session.workspaceId === workspaceId) ??
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
  if (
    !state.activeSessionId ||
    sessions.some((session) => session.sessionId === state.activeSessionId)
  )
    return { ...state, sessions };
  const workspaceId = state.sessions.find(
    (session) => session.sessionId === state.activeSessionId,
  )?.workspaceId;
  const fallback = sessions.find((session) => session.workspaceId === workspaceId);
  const terminalFallback = state.terminalTabs.find((tab) => tab.workspaceId === workspaceId);
  if (!fallback && terminalFallback)
    return {
      ...state,
      sessions,
      activeKind: "terminal",
      activeTerminalTabId: terminalFallback.tabId,
    };
  return {
    ...state,
    sessions,
    activeSessionId: fallback?.sessionId ?? null,
    activeKind: "conversation",
    activeTerminalTabId: null,
    isOpen: fallback ? state.isOpen : false,
  };
}
