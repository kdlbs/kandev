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

// Activity ranking for the sidebar's status indicator. The primary session
// drives diff stats and updatedAt (those reflect the task's "default" view),
// but the sidebar's state badge should reflect whatever session is most
// active right now — otherwise a secondary chat tab running in the
// background leaves the task showing the idle primary's "Turn Finished"
// badge. Lower index = more active.
const SESSION_STATE_PRIORITY: TaskSessionState[] = [
  "RUNNING",
  "STARTING",
  "WAITING_FOR_INPUT",
  "COMPLETED",
  "FAILED",
  "CANCELLED",
  "CREATED",
];

function priority(state: TaskSessionState | undefined): number {
  if (!state) return SESSION_STATE_PRIORITY.length;
  const idx = SESSION_STATE_PRIORITY.indexOf(state);
  return idx === -1 ? SESSION_STATE_PRIORITY.length : idx;
}

function pickMostActiveState(sessions: TaskSession[]): TaskSessionState | undefined {
  let best: TaskSessionState | undefined;
  for (const s of sessions) {
    const candidate = s.state as TaskSessionState | undefined;
    if (priority(candidate) < priority(best)) best = candidate;
  }
  return best;
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
  const sessionState = pickMostActiveState(sessions);
  const envKey = environmentIdBySessionId?.[latestSession.id] ?? latestSession.id;
  const gitStatus = gitStatusByEnvId[envKey];
  if (!gitStatus) return { diffStats: undefined, updatedAt, sessionState };

  const diffStats = computeDiffStats(gitStatus);
  return { diffStats, updatedAt, sessionState };
}
