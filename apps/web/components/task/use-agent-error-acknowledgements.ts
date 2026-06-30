"use client";

import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  type AgentErrorOptions,
  resolvedAgentErrorAcknowledgementStamp,
} from "@/lib/task-agent-error";
import type { TaskSession } from "@/lib/types/http";

type UseAgentErrorAcknowledgementsParams = {
  sessionsById: Record<string, TaskSession>;
  messagesBySession: AgentErrorOptions["messagesBySession"];
  dismissedAgentErrors: Record<string, string>;
  acknowledgedAgentErrors: Record<string, string>;
};

export function usePersistResolvedAgentErrorAcknowledgements({
  sessionsById,
  messagesBySession,
  dismissedAgentErrors,
  acknowledgedAgentErrors,
}: UseAgentErrorAcknowledgementsParams) {
  const acknowledgeAgentError = useAppStore((state) => state.acknowledgeAgentError);

  useEffect(() => {
    for (const [sessionId, session] of Object.entries(sessionsById)) {
      const stamp = resolvedAgentErrorAcknowledgementStamp(sessionId, session, {
        messagesBySession,
        dismissedAgentErrors,
        acknowledgedAgentErrors,
      });
      if (stamp) acknowledgeAgentError(sessionId, stamp);
    }
  }, [
    acknowledgeAgentError,
    acknowledgedAgentErrors,
    dismissedAgentErrors,
    messagesBySession,
    sessionsById,
  ]);
}
