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
  const attemptedFor = useRef<string | null>(null);
  const inFlightFor = useRef<string | null>(null);
  const submitRef = useRef(submit);
  const onAcceptedRef = useRef(onAccepted);
  submitRef.current = submit;
  onAcceptedRef.current = onAccepted;

  useEffect(() => {
    if (!prompt || !taskId || blocked) return;
    const attemptKey = `${sessionId}\u0000${taskId}\u0000${prompt}`;
    if (attemptedFor.current === attemptKey || inFlightFor.current === attemptKey) return;
    attemptedFor.current = attemptKey;
    inFlightFor.current = attemptKey;
    void Promise.resolve()
      .then(() => submitRef.current({ message: prompt }))
      .then((accepted) => {
        if (accepted !== false) onAcceptedRef.current?.();
      })
      .catch(() => undefined)
      .finally(() => {
        if (inFlightFor.current === attemptKey) inFlightFor.current = null;
      });
  }, [blocked, prompt, sessionId, taskId]);
}
