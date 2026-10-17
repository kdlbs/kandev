import type { SessionReadOptions } from "@/lib/state/session-read-coordinator";
import { isWebSocketRequestTimeoutError, WebSocketRequestError } from "@/lib/ws/request-error";

// Repository scans can exceed the default five-second WebSocket timeout.
export const SESSION_GIT_READ_TIMEOUT_MS = 30_000;

export function withSessionGitReadRetry<T>(
  fetch: SessionReadOptions<T>["fetch"],
): SessionReadOptions<T>["fetch"] {
  return async (isCurrent) => {
    try {
      return await fetch(isCurrent);
    } catch (error) {
      if (
        isWebSocketRequestTimeoutError(error) ||
        (error instanceof WebSocketRequestError &&
          (error.code === "INTERNAL_ERROR" || error.code === "UNAVAILABLE"))
      ) {
        // The coordinator retains data and owns one retry timer per shared read.
        return { retryAfter: 5000 };
      }
      throw error;
    }
  };
}
