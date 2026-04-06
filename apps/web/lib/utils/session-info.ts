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
  const updatedAt = latestSession.updated_at;
  const sessionState = latestSession.state as TaskSessionState | undefined;
  const envKey = environmentIdBySessionId?.[latestSession.id] ?? latestSession.id;
  const gitStatus = gitStatusByEnvId[envKey];
  if (!gitStatus) return { diffStats: undefined, updatedAt, sessionState };

  // Prefer branch-level totals (full branch diff vs merge-base) when available.
  // Fall back to summing per-file counts for backwards compat with older agents.
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

  const diffStats = additions === 0 && deletions === 0 ? undefined : { additions, deletions };
  return { diffStats, updatedAt, sessionState };
}
