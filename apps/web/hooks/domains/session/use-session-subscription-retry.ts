import { useEffect, useState } from "react";

import type { TaskSessionState } from "@/lib/types/http";
import { generateUUID } from "@/lib/utils";
import { getWebSocketClient } from "@/lib/ws/connection";

const UNKNOWN_SESSION_RESUBSCRIBE_MS = 1000;
const MAX_UNKNOWN_SESSION_RESUBSCRIBE_ATTEMPTS = 15;

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
    const id = window.setInterval(() => {
      attempts += 1;
      setRetryState({ sessionId, count: attempts });
      if (attempts >= MAX_UNKNOWN_SESSION_RESUBSCRIBE_ATTEMPTS) {
        window.clearInterval(id);
      }
    }, UNKNOWN_SESSION_RESUBSCRIBE_MS);
    return () => window.clearInterval(id);
  }, [sessionId, shouldRetry]);

  if (!shouldRetry || retryState.sessionId !== sessionId) return 0;
  return retryState.count;
}

export function useUnknownSessionSubscriptionRetryEffect(params: {
  taskSessionId: string | null;
  connectionStatus: string;
  retryToken: number;
}) {
  const { taskSessionId, connectionStatus, retryToken } = params;

  useEffect(() => {
    if (!taskSessionId || connectionStatus !== "connected" || retryToken === 0) return;
    const client = getWebSocketClient();
    if (!client) return;
    client.send({
      id: generateUUID(),
      type: "request",
      action: "session.subscribe",
      payload: { session_id: taskSessionId },
    });
  }, [taskSessionId, connectionStatus, retryToken]);
}
