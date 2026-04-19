import { useCallback } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";

/**
 * Returns a sender for shell keystrokes and escape sequences.
 * Posts `shell.input` over the active WS connection; no-op when no session or
 * no client.
 */
export function useShellKeySender(sessionId: string | null | undefined) {
  return useCallback(
    (data: string) => {
      if (!sessionId || !data) return;
      const client = getWebSocketClient();
      if (!client) return;
      client.send({
        type: "request",
        action: "shell.input",
        payload: { session_id: sessionId, data },
      });
    },
    [sessionId],
  );
}
