"use client";

import { useEffect, useMemo } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  type AgentErrorOptions,
  resolvedAgentErrorAcknowledgementStamp,
} from "@/lib/task-agent-error";
import type { TaskSession } from "@/lib/types/http";
import { isTerminalSessionState } from "@/lib/ws/handlers/agent-session";

type UseAgentErrorAcknowledgementsParams = {
  sessionsById: Record<string, TaskSession>;
  sessionIds: readonly string[];
  messagesBySession: AgentErrorOptions["messagesBySession"];
  dismissedAgentErrors: Record<string, string>;
};

type UseTaskAgentErrorAcknowledgementsParams = Omit<
  UseAgentErrorAcknowledgementsParams,
  "sessionIds"
> & {
  tasks: Array<{ id: string; primarySessionId?: string | null }>;
  sessionsByTaskId: Record<string, TaskSession[] | undefined>;
};

/**
 * Mirrors the sessions `agentErrorMessageForTask` can inspect for visible task
 * rows. Primary sessions are included directly; fallback sessions are included
 * only while non-terminal because terminal sessions will not produce a later
 * agent message that can make their error acknowledgement stale.
 */
export function agentErrorAcknowledgementSessionIds(
  tasks: Array<{ id: string; primarySessionId?: string | null }>,
  sessionsByTaskId: Record<string, TaskSession[] | undefined>,
): string[] {
  const sessionIds = new Set<string>();
  for (const task of tasks) {
    if (task.primarySessionId) sessionIds.add(task.primarySessionId);
    for (const session of sessionsByTaskId[task.id] ?? []) {
      if (!isTerminalSessionState(session.state)) sessionIds.add(session.id);
    }
  }
  return [...sessionIds];
}

/**
 * Builds a stable content key for primary session IDs so derived arrays only
 * change when their contents change. The NUL separator avoids collisions with
 * commas or newlines in IDs, preventing avoidable bulk WS re-subscriptions.
 */
export function stablePrimarySessionIdsKey(
  tasks: Array<{ primarySessionId?: string | null }>,
): string {
  return tasks
    .map((task) => task.primarySessionId)
    .filter((sessionId): sessionId is string => sessionId != null)
    .join("\0");
}

export function usePersistResolvedAgentErrorAcknowledgements({
  sessionsById,
  sessionIds,
  messagesBySession,
  dismissedAgentErrors,
}: UseAgentErrorAcknowledgementsParams) {
  const store = useAppStoreApi();
  const acknowledgeAgentErrors = useAppStore((state) => state.acknowledgeAgentErrors);

  useEffect(() => {
    const acknowledgedAgentErrors = store.getState().acknowledgedAgentErrors;
    const stamps: Record<string, string> = {};
    for (const sessionId of sessionIds) {
      const session = sessionsById[sessionId];
      if (!session) continue;
      const stamp = resolvedAgentErrorAcknowledgementStamp(sessionId, session, {
        messagesBySession,
        dismissedAgentErrors,
        acknowledgedAgentErrors,
      });
      if (stamp) stamps[sessionId] = stamp;
    }
    acknowledgeAgentErrors(stamps);
  }, [
    acknowledgeAgentErrors,
    dismissedAgentErrors,
    messagesBySession,
    sessionIds,
    sessionsById,
    store,
  ]);
}

export function usePersistTaskAgentErrorAcknowledgements({
  tasks,
  sessionsByTaskId,
  sessionsById,
  messagesBySession,
  dismissedAgentErrors,
}: UseTaskAgentErrorAcknowledgementsParams) {
  const sessionIds = useMemo(
    () => agentErrorAcknowledgementSessionIds(tasks, sessionsByTaskId),
    [tasks, sessionsByTaskId],
  );
  usePersistResolvedAgentErrorAcknowledgements({
    sessionsById,
    sessionIds,
    messagesBySession,
    dismissedAgentErrors,
  });
}
