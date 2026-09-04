import { useEffect, useRef } from "react";
import type {
  ChatSubmitPayload,
  ChatSubmitResult,
} from "@/components/task/chat/chat-input-container";

type InitialPromptDelivery = {
  sessionId: string;
  taskId: string | null;
  prompt?: string;
  blocked: boolean;
  submit: (payload: ChatSubmitPayload) => ChatSubmitResult;
  onAccepted?: () => void;
};

/** Sends a Quick Chat launch prompt once admission prerequisites are ready. */
export function useQuickChatInitialPrompt({
  sessionId,
  taskId,
  prompt,
  blocked,
  submit,
  onAccepted,
}: InitialPromptDelivery) {
  const acceptedFor = useRef<string | null>(null);
  const inFlightFor = useRef<string | null>(null);

  useEffect(() => {
    if (!prompt || !taskId || blocked || acceptedFor.current === sessionId) return;
    if (inFlightFor.current === sessionId) return;
    inFlightFor.current = sessionId;
    void Promise.resolve(submit({ message: prompt })).then((accepted) => {
      inFlightFor.current = null;
      if (accepted === false) return;
      acceptedFor.current = sessionId;
      onAccepted?.();
    });
  }, [blocked, onAccepted, prompt, sessionId, submit, taskId]);
}
