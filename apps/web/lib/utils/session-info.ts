import type { TaskSession, TaskSessionState } from "@/lib/types/http";

export type SessionInfo = {
  diffStats: { additions: number; deletions: number } | undefined;
  updatedAt: string | undefined;
  sessionState: TaskSessionState | undefined;
};

type GitStatusMap = Record<
  string,
  {
    files?: Record<string, { additions?: number; deletions?: number }>;
    branch_additions?: number;
    branch_deletions?: number;
  }
>;

function computeDiffStats(
  gitStatus: GitStatusMap[string],
): { additions: number; deletions: number } | undefined {
  let additions: number;
  let deletions: number;
  if (gitStatus.branch_additions !== undefined || gitStatus.branch_deletions !== undefined) {
    additions = gitStatus.branch_additions ?? 0;
    deletions = gitStatus.branch_deletions ?? 0;
  } else {
    additions = 0;
    deletions = 0;
    for (const file of Object.values(gitStatus.files ?? {})) {
      additions += file.additions ?? 0;
      deletions += file.deletions ?? 0;
    }
  }
  return additions === 0 && deletions === 0 ? undefined : { additions, deletions };
}

export function getSessionInfoForTask(
  taskId: string,
  sessionsByTaskId: Record<string, TaskSession[]>,
  gitStatusByEnvId: GitStatusMap,
  environmentIdBySessionId?: Record<string, string>,
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
  // Empty string means the session was created from a WS event without timestamps;
  // return undefined so callers fall through to task.updatedAt/createdAt instead.
  const updatedAt = latestSession.updated_at || undefined;
  const sessionState = latestSession.state as TaskSessionState | undefined;
  const envKey = environmentIdBySessionId?.[latestSession.id] ?? latestSession.id;
  const gitStatus = gitStatusByEnvId[envKey];
  if (!gitStatus) return { diffStats: undefined, updatedAt, sessionState };

  const diffStats = computeDiffStats(gitStatus);
  return { diffStats, updatedAt, sessionState };
}
