import { useEffect, useRef } from "react";
import {
  getChatDraftAttachments,
  getChatDraftText,
  setChatDraftAttachments,
  setChatDraftText,
} from "@/lib/local-storage";
import type {
  ChatSubmitPayload,
  ChatSubmitResult,
} from "@/components/task/chat/chat-input-container";
import type { QuickChatInitialPrompt } from "@/lib/state/slices/ui/types";

type InitialPromptDelivery = {
  sessionId: string;
  taskId: string | null;
  prompt?: QuickChatInitialPrompt;
  blocked: boolean;
  submit: (payload: ChatSubmitPayload) => ChatSubmitResult;
  onAttempted?: () => void;
  onAccepted?: (sessionId: string, prompt: QuickChatInitialPrompt) => void;
  onRejected?: (sessionId: string, prompt: QuickChatInitialPrompt) => void;
};

function toSubmitPayload(prompt: QuickChatInitialPrompt): ChatSubmitPayload {
  return typeof prompt === "string" ? { message: prompt } : prompt;
}

function toStoredAttachments(payload: ChatSubmitPayload) {
  return (
    payload.attachments?.flatMap((attachment) => {
      if (!attachment.attachment_id || !attachment.name) return [];
      return [
        {
          id: attachment.attachment_id,
          attachmentId: attachment.attachment_id,
          mimeType: attachment.mime_type,
          fileName: attachment.name,
          size: attachment.size_bytes ?? 0,
          isImage: attachment.type === "image",
          deliveryMode:
            attachment.delivery_mode ?? (attachment.type === "image" ? "prompt" : "path"),
        },
      ];
    }) ?? []
  );
}

function attachmentDraftMatches(
  sessionId: string,
  expected: ReturnType<typeof toStoredAttachments>,
): boolean {
  const current = getChatDraftAttachments(sessionId);
  return (
    current.length === expected.length &&
    current.every(
      (attachment, index) =>
        attachment.attachmentId === expected[index]?.attachmentId &&
        attachment.fileName === expected[index]?.fileName &&
        attachment.mimeType === expected[index]?.mimeType &&
        attachment.size === expected[index]?.size &&
        attachment.isImage === expected[index]?.isImage &&
        attachment.deliveryMode === expected[index]?.deliveryMode,
    )
  );
}

type MutableValue<T> = { current: T };
type DeliveryGeneration = { identity: string; generation: number };

function scheduleInitialPromptDelivery(args: {
  sessionId: string;
  taskId: string;
  prompt: QuickChatInitialPrompt;
  deliveryGeneration: number;
  deliveryGenerationRef: MutableValue<DeliveryGeneration>;
  attemptedFor: MutableValue<string | null>;
  inFlightFor: MutableValue<string | null>;
  blockedRef: MutableValue<boolean>;
  submit: (payload: ChatSubmitPayload) => ChatSubmitResult;
  onAttempted?: () => void;
  onAccepted?: (sessionId: string, prompt: QuickChatInitialPrompt) => void;
  onRejected?: (sessionId: string, prompt: QuickChatInitialPrompt) => void;
}) {
  const {
    sessionId,
    taskId,
    prompt,
    deliveryGeneration,
    deliveryGenerationRef,
    attemptedFor,
    inFlightFor,
    blockedRef,
    submit,
    onAttempted,
    onAccepted,
    onRejected,
  } = args;
  const payload = toSubmitPayload(prompt);
  const draftAttachments = toStoredAttachments(payload);
  const attemptIdentity =
    payload.clientMessageId ??
    JSON.stringify({
      message: payload.message,
      attachments: payload.attachments?.map(
        (attachment) => attachment.attachment_id ?? attachment.name,
      ),
    });
  const attemptKey = `${sessionId}\u0000${taskId}\u0000${attemptIdentity}`;
  if (attemptedFor.current === attemptKey || inFlightFor.current === attemptKey) return;
  inFlightFor.current = attemptKey;
  const currentDraftText = getChatDraftText(sessionId);
  const currentHasAttachments = getChatDraftAttachments(sessionId).length > 0;
  const hasDraft = currentDraftText !== "" || currentHasAttachments;
  const draftAlreadyMatches =
    currentDraftText === payload.message && attachmentDraftMatches(sessionId, draftAttachments);
  const savedForRecovery = !hasDraft || draftAlreadyMatches;
  if (!hasDraft) {
    setChatDraftText(sessionId, payload.message);
    setChatDraftAttachments(sessionId, draftAttachments);
  }
  const restoreRejectedDraft = () => {
    if (
      deliveryGenerationRef.current.generation === deliveryGeneration &&
      attemptedFor.current === attemptKey &&
      savedForRecovery &&
      getChatDraftText(sessionId) === payload.message &&
      attachmentDraftMatches(sessionId, draftAttachments)
    ) {
      onRejected?.(sessionId, prompt);
    }
  };
  void Promise.resolve()
    .then(() => {
      // Earlier passive effects can start the session queue after this effect
      // was scheduled. Re-read the admission gate before consuming the prompt.
      if (deliveryGenerationRef.current.generation !== deliveryGeneration) return undefined;
      if (blockedRef.current) return undefined;
      attemptedFor.current = attemptKey;
      onAttempted?.();
      return submit(payload);
    })
    .then((accepted) => {
      if (
        deliveryGenerationRef.current.generation !== deliveryGeneration ||
        attemptedFor.current !== attemptKey
      )
        return;
      if (accepted === false) {
        restoreRejectedDraft();
        return;
      }
      if (
        savedForRecovery &&
        getChatDraftText(sessionId) === payload.message &&
        attachmentDraftMatches(sessionId, draftAttachments)
      ) {
        setChatDraftText(sessionId, "");
        setChatDraftAttachments(sessionId, []);
      }
      onAccepted?.(sessionId, prompt);
    })
    .catch(restoreRejectedDraft)
    .finally(() => {
      if (inFlightFor.current === attemptKey) inFlightFor.current = null;
    });
}

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
  const deliveryIdentity = `${sessionId}\u0000${taskId ?? ""}`;
  const deliveryGenerationRef = useRef<DeliveryGeneration>({
    identity: deliveryIdentity,
    generation: 0,
  });
  if (deliveryGenerationRef.current.identity !== deliveryIdentity) {
    deliveryGenerationRef.current = {
      identity: deliveryIdentity,
      generation: deliveryGenerationRef.current.generation + 1,
    };
  }
  const blockedRef = useRef(blocked);
  const submitRef = useRef(submit);
  const onAttemptedRef = useRef(onAttempted);
  const onAcceptedRef = useRef(onAccepted);
  const onRejectedRef = useRef(onRejected);
  blockedRef.current = blocked;
  submitRef.current = submit;
  onAttemptedRef.current = onAttempted;
  onAcceptedRef.current = onAccepted;
  onRejectedRef.current = onRejected;

  useEffect(() => {
    if (!prompt || !taskId || blocked) return;
    scheduleInitialPromptDelivery({
      sessionId,
      taskId,
      prompt,
      deliveryGeneration: deliveryGenerationRef.current.generation,
      deliveryGenerationRef,
      attemptedFor,
      inFlightFor,
      blockedRef,
      submit: submitRef.current,
      onAttempted: onAttemptedRef.current,
      onAccepted: onAcceptedRef.current,
      onRejected: onRejectedRef.current,
    });
  }, [blocked, prompt, sessionId, taskId]);
}
