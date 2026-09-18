"use client";

import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAppStoreApi } from "@/components/state-provider";
import { MessageSendError } from "@/lib/chat/message-send-error";
import { findPendingClarification } from "@/lib/utils/pending-clarification";
import type { Message } from "@/lib/types/http";
import {
  formatLateClarificationMessage,
  type LateClarificationSnapshot,
} from "@/lib/clarification/late-clarification-message";
import { generateUUID } from "@/lib/utils";
import { useMessageHandler, type MessageAdmissionOutcome } from "./use-message-handler";

export type { LateClarificationSnapshot } from "@/lib/clarification/late-clarification-message";

export type LateClarificationState =
  | { status: "idle"; snapshot: null }
  | { status: "sending"; snapshot: LateClarificationSnapshot }
  | { status: "sent"; snapshot: LateClarificationSnapshot }
  | { status: "queued"; snapshot: LateClarificationSnapshot }
  | { status: "error"; error: unknown; snapshot: LateClarificationSnapshot };

// The source row can unmount while admission is unresolved (for example when
// the user opens another task). Keep the identity for that exact source and
// question bundle so a later retry reconciles the same message instead of
// creating a second one. Successful admission removes the entry.
const pendingLateAdmissionIds = new Map<string, string>();

function lateAdmissionKey(
  taskId: string | null,
  sessionId: string | null,
  snapshot: LateClarificationSnapshot,
): string {
  const messageIdentity = snapshot.messages
    .map((message) => {
      const metadata = message.metadata as
        | { pending_id?: string; question_id?: string }
        | undefined;
      return [metadata?.pending_id ?? null, metadata?.question_id ?? null, message.id];
    })
    .sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b)));
  return JSON.stringify([taskId, sessionId, messageIdentity]);
}

function sourceIds(message: Message | undefined): {
  taskId: string | null;
  sessionId: string | null;
} {
  if (!message) return { taskId: null, sessionId: null };
  return { taskId: message.task_id ?? null, sessionId: message.session_id ?? null };
}

export function useLateClarificationMessage(sourceMessage: Message | undefined) {
  const { t } = useTranslation();
  const storeApi = useAppStoreApi();
  const { taskId, sessionId } = sourceIds(sourceMessage);
  const excludedPendingIdRef = useRef<string | null>(null);
  const [state, setState] = useState<LateClarificationState>({ status: "idle", snapshot: null });

  const getHasPendingClarification = useCallback(() => {
    if (!sessionId) return false;
    const messages = storeApi.getState().messages?.bySession?.[sessionId] ?? [];
    const pending = findPendingClarification(messages);
    if (!pending) return false;
    const pendingId = (pending.metadata as { pending_id?: string } | undefined)?.pending_id;
    return pendingId !== excludedPendingIdRef.current;
  }, [sessionId, storeApi]);

  const { handleSendMessageWithOutcome } = useMessageHandler({
    resolvedSessionId: sessionId,
    taskId,
    sessionModel: null,
    activeModel: null,
    getHasPendingClarification,
  });

  const send = useCallback(
    async (snapshot: LateClarificationSnapshot): Promise<MessageAdmissionOutcome> => {
      const pendingId = (snapshot.messages[0]?.metadata as { pending_id?: string } | undefined)
        ?.pending_id;
      excludedPendingIdRef.current = pendingId ?? null;
      const message = formatLateClarificationMessage(snapshot.messages, snapshot.answers, {
        questionLabel: t("task:lateAnswerQuestionLabel"),
        answerLabel: t("task:lateAnswerAnswerLabel"),
        contextLabel: t("task:lateAnswerContextLabel"),
      });
      const admissionKey = lateAdmissionKey(taskId, sessionId, snapshot);
      const clientMessageId = pendingLateAdmissionIds.get(admissionKey) ?? generateUUID();
      pendingLateAdmissionIds.set(admissionKey, clientMessageId);
      const admissionSnapshot: LateClarificationSnapshot = {
        messages: snapshot.messages,
        answers: snapshot.answers,
      };
      setState({ status: "sending", snapshot: admissionSnapshot });
      try {
        const outcome = await handleSendMessageWithOutcome({ message, clientMessageId });
        if (outcome === false || outcome === undefined) {
          throw new MessageSendError("late-answer-admission-failed", t("task:lateAnswerFailed"));
        }
        pendingLateAdmissionIds.delete(admissionKey);
        setState({ status: outcome, snapshot: admissionSnapshot });
        return outcome;
      } catch (error) {
        setState({ status: "error", error, snapshot: admissionSnapshot });
        throw error;
      }
    },
    [handleSendMessageWithOutcome, sessionId, t, taskId],
  );

  const reset = useCallback(() => setState({ status: "idle", snapshot: null }), []);

  return { state, send, reset, taskId, sessionId };
}
