import { useCallback } from "react";
import { sendShellInput } from "@/lib/terminal/send-shell-input";

/**
 * Returns a sender for shell keystrokes and escape sequences. Applies the
 * active ctrl/shift modifiers before sending. No-op when no session.
 */
export function useShellKeySender(sessionId: string | null | undefined) {
  return useCallback(
    (data: string) => {
      if (!sessionId) return;
      sendShellInput(sessionId, data);
    },
    [sessionId],
  );
}
