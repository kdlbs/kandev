import { useEffect, useState } from "react";

import { createDebugLogger } from "@/lib/debug/log";
import type { TaskSessionState } from "@/lib/types/http";
import { generateUUID } from "@/lib/utils";
import { getWebSocketClient } from "@/lib/ws/connection";

const UNKNOWN_SESSION_INITIAL_RESUBSCRIBE_MS = 1000;
const UNKNOWN_SESSION_MAX_RESUBSCRIBE_MS = 30000;
const debug = createDebugLogger("messages:resubscribe");

type RetryState = {
  sessionId: string | null;
  count: number;
};

export function shouldRetryUnknownSessionSubscription(params: {
  taskSessionId: string | null;
  taskSessionState: TaskSessionState | null;
  connectionStatus: string;
}) {
  return (
    !!params.taskSessionId &&
    params.connectionStatus === "connected" &&
    params.taskSessionState === null
  );
}

export function useUnknownSessionSubscriptionRetry(params: {
  taskSessionId: string | null;
  taskSessionState: TaskSessionState | null;
  connectionStatus: string;
}) {
  const [retryState, setRetryState] = useState<RetryState>({ sessionId: null, count: 0 });
  const shouldRetry = shouldRetryUnknownSessionSubscription(params);
  const sessionId = params.taskSessionId;

  useEffect(() => {
    if (!shouldRetry) return;
    let attempts = 0;
    let timeoutId: number | null = null;
    let cancelled = false;
    const schedule = () => {
      if (cancelled) return;
      timeoutId = window.setTimeout(() => {
        if (cancelled) return;
        const nextAttempts = attempts + 1;
        attempts = nextAttempts;
        setRetryState({ sessionId, count: nextAttempts });
        schedule();
      }, getUnknownSessionRetryDelay(attempts));
    };
    schedule();
    return () => {
      cancelled = true;
      if (timeoutId !== null) window.clearTimeout(timeoutId);
    };
  }, [sessionId, shouldRetry]);

  if (!shouldRetry || retryState.sessionId !== sessionId) return 0;
  return retryState.count;
}

export function getUnknownSessionRetryDelay(attempts: number) {
  return Math.min(
    UNKNOWN_SESSION_INITIAL_RESUBSCRIBE_MS * 2 ** attempts,
    UNKNOWN_SESSION_MAX_RESUBSCRIBE_MS,
  );
}

export function useUnknownSessionSubscriptionRetryEffect(params: {
  taskSessionId: string | null;
  connectionStatus: string;
  retryToken: number;
}) {
  const { taskSessionId, connectionStatus, retryToken } = params;

  useEffect(() => {
    if (!taskSessionId || connectionStatus !== "connected" || retryToken === 0) {
      debug("unknown-session retry: skipped", { taskSessionId, connectionStatus, retryToken });
      return;
    }
    const client = getWebSocketClient();
    if (!client) {
      debug("unknown-session retry: skipped (no ws client)", { sessionId: taskSessionId });
      return;
    }
    // The durable subscription effect owns the client's ref-counted
    // subscribe/unsubscribe lifecycle. This retry only re-sends the subscribe
    // frame to cover the backend race where the first subscribe arrived before
    // the session was fully constructed.
    debug("unknown-session retry: sending session.subscribe", {
      sessionId: taskSessionId,
      retryToken,
    });
    client.send({
      id: generateUUID(),
      type: "request",
      action: "session.subscribe",
      payload: { session_id: taskSessionId },
    });
  }, [taskSessionId, connectionStatus, retryToken]);
}
