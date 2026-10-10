import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";

function hasNativeACPSessionToken(session: TaskSession): boolean {
  if (
    typeof session.downstream_acp_session_id === "string" &&
    session.downstream_acp_session_id.trim()
  ) {
    return true;
  }
  const acp = session.metadata?.acp;
  if (!acp || typeof acp !== "object" || Array.isArray(acp)) return false;
  const nativeSessionId = (acp as Record<string, unknown>).session_id;
  return typeof nativeSessionId === "string" && nativeSessionId.trim().length > 0;
}

export function isProviderRestoredResumeEligible(
  state: AppState,
  taskId: string,
  sessionId: string,
): boolean {
  const session = state.taskSessions.items[sessionId];
  if (
    !session ||
    session.task_id !== taskId ||
    session.state !== "FAILED" ||
    session.is_passthrough ||
    !hasNativeACPSessionToken(session)
  ) {
    return false;
  }

  const task = state.kanban.tasks.find((candidate) => candidate.id === taskId);
  if (task?.isFromOffice) return false;
  const quickChatOwnsTaskSession = state.quickChat.sessions.some(
    (candidate) =>
      candidate.kind === "chat" && candidate.sessionId === sessionId && candidate.taskId === taskId,
  );
  if (!task && !quickChatOwnsTaskSession) return false;

  const profileId = session.execution_profile_id || session.agent_profile_id;
  if (!profileId) return false;
  const profile = state.agentProfiles.items.find((candidate) => candidate.id === profileId);
  return !!profile && !profile.cli_passthrough && profile.agent_name.toLowerCase() === "auggie";
}
