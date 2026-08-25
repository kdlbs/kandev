import type { AppState } from "@/lib/state/store";
import { isSessionWorking } from "@/lib/session-working";
import { isQuickChatSetupSessionId } from "./quick-chat-session";
import { selectQuickChatHasUnseenIdle } from "./quick-chat-unseen-selectors";

export type QuickChatActivityState = "running" | "finished" | null;

/** Returns whether one session has live agent, background, or prepare work. */
export function selectQuickChatSessionIsWorking(state: AppState, sessionId: string): boolean {
  const session = state.taskSessions?.items?.[sessionId];
  return (
    isSessionWorking(session) ||
    state.prepareProgress?.bySessionId?.[sessionId]?.status === "preparing"
  );
}

/** Derives the activity bubble for one workspace's closed Quick Chat entry. */
export function selectQuickChatActivity(
  state: AppState,
  workspaceId: string | null | undefined,
): QuickChatActivityState {
  if (!workspaceId || state.quickChat?.isOpen) return null;

  const hasWorkingConversation = (state.quickChat?.sessions ?? []).some(
    (session) =>
      session.workspaceId === workspaceId &&
      !isQuickChatSetupSessionId(session.sessionId) &&
      selectQuickChatSessionIsWorking(state, session.sessionId),
  );
  if (hasWorkingConversation) return "running";
  return selectQuickChatHasUnseenIdle(state, workspaceId) ? "finished" : null;
}
