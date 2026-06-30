"use client";

import { useEffect } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  type AgentErrorOptions,
  resolvedAgentErrorAcknowledgementStamp,
} from "@/lib/task-agent-error";
import type { TaskSession } from "@/lib/types/http";

type UseAgentErrorAcknowledgementsParams = {
  sessionsById: Record<string, TaskSession>;
  sessionIds: readonly string[];
  messagesBySession: AgentErrorOptions["messagesBySession"];
  dismissedAgentErrors: Record<string, string>;
};

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
