import type { TaskSession, TaskSessionState } from "@/lib/types/http";

export type SessionInfo = {
  diffStats: { additions: number; deletions: number } | undefined;
  updatedAt: string | undefined;
  sessionState: TaskSessionState | undefined;
};

type GitStatusMap = Record<
  string,
  { files?: Record<string, { additions?: number; deletions?: number }> }
>;

export function getSessionInfoForTask(
  taskId: string,
  sessionsByTaskId: Record<string, TaskSession[]>,
  gitStatusBySessionId: GitStatusMap,
): SessionInfo {
  const sessions = sessionsByTaskId[taskId] ?? [];
  if (sessions.length === 0) {
    return { diffStats: undefined, updatedAt: undefined, sessionState: undefined };
  }
  const primarySession = sessions.find((s: TaskSession) => s.is_primary);
  const latestSession = primarySession ?? sessions[0];
  if (!latestSession) {
    return { diffStats: undefined, updatedAt: undefined, sessionState: undefined };
  }
  const updatedAt = latestSession.updated_at;
  const sessionState = latestSession.state as TaskSessionState | undefined;
  const gitStatus = gitStatusBySessionId[latestSession.id];
  if (!gitStatus?.files) return { diffStats: undefined, updatedAt, sessionState };
  let additions = 0;
  let deletions = 0;
  for (const file of Object.values(gitStatus.files)) {
    additions += file.additions ?? 0;
    deletions += file.deletions ?? 0;
  }
  const diffStats = additions === 0 && deletions === 0 ? undefined : { additions, deletions };
  return { diffStats, updatedAt, sessionState };
}
