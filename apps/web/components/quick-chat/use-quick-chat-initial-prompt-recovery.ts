import { useCallback, type RefObject } from "react";
import type { ChatInputContainerHandle } from "@/components/task/chat/chat-input-container";
import type { QuickChatInitialPrompt } from "@/lib/state/slices/ui/types";

export function useQuickChatInitialPromptRecovery(
  sessionId: string,
  chatInputRef: RefObject<ChatInputContainerHandle | null>,
) {
  const restoreRejectedPrompt = useCallback(
    (rejectedSessionId: string, prompt: QuickChatInitialPrompt) => {
      const input = chatInputRef.current;
      if (rejectedSessionId === sessionId && input && !input.getValue()) {
        input.insertText(typeof prompt === "string" ? prompt : prompt.message, 0, 0);
      }
    },
    [chatInputRef, sessionId],
  );

  const clearAcceptedPrompt = useCallback(
    (acceptedSessionId: string, prompt: QuickChatInitialPrompt) => {
      if (acceptedSessionId !== sessionId) return;
      const payload = typeof prompt === "string" ? { message: prompt } : prompt;
      chatInputRef.current?.clearAcceptedPayload?.(payload);
    },
    [chatInputRef, sessionId],
  );

  return { restoreRejectedPrompt, clearAcceptedPrompt };
}
