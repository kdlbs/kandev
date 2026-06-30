"use client";

import { useEffect } from "react";
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
