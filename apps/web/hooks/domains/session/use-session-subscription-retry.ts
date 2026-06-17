import { useEffect, useState } from "react";

import type { TaskSessionState } from "@/lib/types/http";

const UNKNOWN_SESSION_RESUBSCRIBE_MS = 1000;

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
  const [retryToken, setRetryToken] = useState(0);
  const shouldRetry = shouldRetryUnknownSessionSubscription(params);

  useEffect(() => {
    if (!shouldRetry) return;
    const id = window.setInterval(
      () => setRetryToken((value) => value + 1),
      UNKNOWN_SESSION_RESUBSCRIBE_MS,
    );
    return () => window.clearInterval(id);
  }, [shouldRetry]);

  return shouldRetry ? retryToken : 0;
}
