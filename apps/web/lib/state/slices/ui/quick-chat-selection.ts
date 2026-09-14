import { orderQuickChatTabs } from "./quick-chat-tab-order";
import { isQuickChatSetupSessionId } from "./quick-chat-session";
import type {
  QuickChatSelection,
  QuickChatSelectionByWorkspace,
  QuickChatSelectionOrder,
  QuickChatSession,
  QuickChatSessionKind,
} from "./types";

export function rememberQuickChatSelection(
  selections: QuickChatSelectionByWorkspace,
  workspaceId: string,
  kind: QuickChatSessionKind,
  sessionId: string,
): QuickChatSelectionByWorkspace {
  return {
    ...selections,
    [workspaceId]: {
      ...selections[workspaceId],
      [kind]: sessionId,
    },
  };
}

export function clearRememberedQuickChatSelection(
  selections: QuickChatSelectionByWorkspace,
  workspaceId: string,
  kind: QuickChatSessionKind,
): QuickChatSelectionByWorkspace {
  const current = selections[workspaceId];
  if (!current || !current[kind]) return selections;
  const next: QuickChatSelection = { ...current };
  delete next[kind];
  if (Object.keys(next).length === 0) {
    const { [workspaceId]: _removed, ...remaining } = selections;
    return remaining;
  }
  return { ...selections, [workspaceId]: next };
}

export function clearRememberedQuickChatSession(
  selections: QuickChatSelectionByWorkspace,
  sessions: QuickChatSession[],
  sessionId: string,
): QuickChatSelectionByWorkspace {
  const session = sessions.find((item) => item.sessionId === sessionId);
  if (!session) return selections;
  const kind = session.kind ?? "chat";
  if (selections[session.workspaceId]?.[kind] !== sessionId) return selections;
  return clearRememberedQuickChatSelection(selections, session.workspaceId, kind);
}

export function findRememberedQuickChatSession(
  sessions: QuickChatSession[],
  selections: QuickChatSelectionByWorkspace,
  workspaceId: string,
  kind: QuickChatSessionKind,
): QuickChatSession | undefined {
  const sessionId = selections[workspaceId]?.[kind];
  if (!sessionId || isQuickChatSetupSessionId(sessionId)) return undefined;
  return sessions.find(
    (session) =>
      session.sessionId === sessionId &&
      session.workspaceId === workspaceId &&
      (session.kind ?? "chat") === kind,
  );
}

export function restoreQuickChatSession(
  sessions: QuickChatSession[],
  selections: QuickChatSelectionByWorkspace,
  workspaceId: string,
  kind: QuickChatSessionKind,
  savedOrder: QuickChatSelectionOrder = [],
): string | undefined {
  const remembered = findRememberedQuickChatSession(sessions, selections, workspaceId, kind);
  if (remembered) return remembered.sessionId;

  const matching = sessions.filter(
    (session) => session.workspaceId === workspaceId && (session.kind ?? "chat") === kind,
  );
  return orderQuickChatTabs(matching, [], savedOrder).sessions[0]?.sessionId;
}

export function touchQuickChatSelectionOrder(
  order: QuickChatSelectionOrder,
  workspaceId: string,
): QuickChatSelectionOrder {
  return [workspaceId, ...order.filter((item) => item !== workspaceId)];
}

export function clearSelectionOrderIfEmpty(
  order: QuickChatSelectionOrder,
  selections: QuickChatSelectionByWorkspace,
): QuickChatSelectionOrder {
  return order.filter((workspaceId) => Boolean(selections[workspaceId]));
}
