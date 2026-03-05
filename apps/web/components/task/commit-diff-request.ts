"use client";

import type { FileInfo } from "@/lib/state/store";
import { getWebSocketClient } from "@/lib/ws/connection";

type SessionStatusResponse = {
  is_agent_running: boolean;
};

export type CommitDiffResponse = {
  success: boolean;
  files?: Record<string, FileInfo>;
  ready?: boolean;
  reason?: string;
  retry_after_ms?: number;
};

const notReadyProbeCooldownMs = 1000;
const nextProbeBySession = new Map<string, number>();

function notReadyResult(retryAfterMs = 500): CommitDiffResponse {
  return {
    success: false,
    ready: false,
    reason: "agent_starting",
    retry_after_ms: retryAfterMs,
  };
}

export async function requestCommitDiff(params: {
  sessionId: string;
  taskId: string | null;
  commitSha: string;
  agentctlReady: boolean;
}): Promise<CommitDiffResponse | null> {
  const { sessionId, taskId, commitSha, agentctlReady } = params;
  const ws = getWebSocketClient();
  if (!ws) return null;

  if (!agentctlReady) {
    const now = Date.now();
    const nextProbeAt = nextProbeBySession.get(sessionId) ?? 0;
    if (now < nextProbeAt) {
      return notReadyResult();
    }
    if (!taskId) {
      nextProbeBySession.set(sessionId, now + notReadyProbeCooldownMs);
      return notReadyResult();
    }
    try {
      const status = await ws.request<SessionStatusResponse>(
        "task.session.status",
        { task_id: taskId, session_id: sessionId },
        5000,
      );
      if (!status?.is_agent_running) {
        nextProbeBySession.set(sessionId, now + notReadyProbeCooldownMs);
        return notReadyResult();
      }
    } catch {
      nextProbeBySession.set(sessionId, now + notReadyProbeCooldownMs);
      return notReadyResult();
    }
  }

  const response = await ws.request<CommitDiffResponse>(
    "session.commit_diff",
    { session_id: sessionId, commit_sha: commitSha },
    10000,
  );
  if (response?.success) {
    nextProbeBySession.delete(sessionId);
    return response;
  }
  if (response?.ready === false) {
    const retryAfter = response.retry_after_ms ?? notReadyProbeCooldownMs;
    nextProbeBySession.set(sessionId, Date.now() + retryAfter);
  }
  return response;
}
