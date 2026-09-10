import { useEffect, useRef } from "react";
import { getChatDraftText, setChatDraftText } from "@/lib/local-storage";
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
  onAttempted?: () => void;
  onAccepted?: () => void;
  onRejected?: (sessionId: string, prompt: string) => void;
};

/** Sends a Quick Chat launch prompt once admission prerequisites are ready. */
export function useQuickChatInitialPrompt({
  sessionId,
  taskId,
  prompt,
  blocked,
  submit,
  onAttempted,
  onAccepted,
  onRejected,
}: InitialPromptDelivery) {
  const attemptedFor = useRef<string | null>(null);
  const inFlightFor = useRef<string | null>(null);
  const submitRef = useRef(submit);
  const onAttemptedRef = useRef(onAttempted);
  const onAcceptedRef = useRef(onAccepted);
  const onRejectedRef = useRef(onRejected);
  submitRef.current = submit;
  onAttemptedRef.current = onAttempted;
  onAcceptedRef.current = onAccepted;
  onRejectedRef.current = onRejected;

  useEffect(() => {
    if (!prompt || !taskId || blocked) return;
    const attemptKey = `${sessionId}\u0000${taskId}\u0000${prompt}`;
    if (attemptedFor.current === attemptKey || inFlightFor.current === attemptKey) return;
    attemptedFor.current = attemptKey;
    inFlightFor.current = attemptKey;
    const savedForRecovery = !getChatDraftText(sessionId);
    if (savedForRecovery) setChatDraftText(sessionId, prompt);
    const submit = submitRef.current;
    const onAccepted = onAcceptedRef.current;
    const restoreRejectedDraft = () => {
      if (savedForRecovery && getChatDraftText(sessionId) === prompt)
        onRejectedRef.current?.(sessionId, prompt);
    };
    onAttemptedRef.current?.();
    void Promise.resolve()
      .then(() => submit({ message: prompt }))
      .then((accepted) => {
        if (accepted === false) {
          restoreRejectedDraft();
          return;
        }
        if (savedForRecovery && getChatDraftText(sessionId) === prompt)
          setChatDraftText(sessionId, "");
        onAccepted?.();
      })
      .catch(restoreRejectedDraft)
      .finally(() => {
        if (inFlightFor.current === attemptKey) inFlightFor.current = null;
      });
  }, [blocked, prompt, sessionId, taskId]);
}
