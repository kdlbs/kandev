import type { QuickChatSessionResponse } from "@/lib/api/domains/workspace-api";
import type { QuickChatSession } from "@/lib/state/slices/ui/types";

/**
 * Maps the quick-chat list response onto store tabs.
 *
 * Shared by boot hydration and the runtime resync. Keeping one mapper matters:
 * when these drifted, boot-loaded tabs silently lost `taskId`, so closing one
 * skipped the backend delete and left an orphaned ephemeral task behind.
 */
export function toQuickChatSessions(sessions: QuickChatSessionResponse[]): QuickChatSession[] {
  return sessions.map((session) => ({
    kind: session.kind,
    sessionId: session.session_id,
    taskId: session.task_id,
    workspaceId: session.workspace_id,
    name: session.name,
    agentProfileId: session.agent_profile_id,
  }));
}
