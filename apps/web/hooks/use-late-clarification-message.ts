"use client";

import { useCallback, useRef, useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";
import { useAppStoreApi } from "@/components/state-provider";
import { MessageSendError } from "@/lib/chat/message-send-error";
import { findPendingClarification } from "@/lib/utils/pending-clarification";
import type { Message } from "@/lib/types/http";
import { fetchTaskSession, listTaskSessionMessages } from "@/lib/api/domains/session-api";
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

type LateAdmissionRecord = {
  clientMessageId: string;
  state: LateClarificationState;
  inFlight: Promise<MessageAdmissionOutcome> | null;
};

const IDLE_LATE_CLARIFICATION_STATE: LateClarificationState = {
  status: "idle",
  snapshot: null,
};
const lateAdmissionRecords = new Map<string, LateAdmissionRecord>();
const lateAdmissionListeners = new Map<string, Set<() => void>>();

function pendingIdFromMessage(message: Message | undefined): string | null {
  const metadata = message?.metadata as { pending_id?: string } | undefined;
  return metadata?.pending_id ?? null;
}

function messageIdentity(messages: readonly Message[]): string {
  return JSON.stringify(messages.map((message) => message.id).sort());
}

function lateAdmissionKey(
  taskId: string | null,
  sessionId: string | null,
  messages: readonly Message[],
): string {
  const pendingId = pendingIdFromMessage(messages[0]);
  return JSON.stringify([taskId, sessionId, pendingId ?? messageIdentity(messages)]);
}

function sourceIds(message: Message | undefined): {
  taskId: string | null;
  sessionId: string | null;
} {
  if (!message) return { taskId: null, sessionId: null };
  return { taskId: message.task_id ?? null, sessionId: message.session_id ?? null };
}

function sourceAdmissionKey(message: Message | undefined): string | null {
  const { taskId, sessionId } = sourceIds(message);
  if (!message || !taskId || !sessionId) return null;
  return lateAdmissionKey(taskId, sessionId, [message]);
}

function recordForKey(key: string): LateAdmissionRecord {
  const existing = lateAdmissionRecords.get(key);
  if (existing) return existing;
  const created: LateAdmissionRecord = {
    clientMessageId: generateUUID(),
    state: IDLE_LATE_CLARIFICATION_STATE,
    inFlight: null,
  };
  lateAdmissionRecords.set(key, created);
  return created;
}

function stateForKey(key: string | null): LateClarificationState {
  return key
    ? (lateAdmissionRecords.get(key)?.state ?? IDLE_LATE_CLARIFICATION_STATE)
    : IDLE_LATE_CLARIFICATION_STATE;
}

function notifyRecord(key: string) {
  for (const listener of lateAdmissionListeners.get(key) ?? []) listener();
}

function updateRecordState(key: string, state: LateClarificationState) {
  const record = recordForKey(key);
  record.state = state;
  notifyRecord(key);
}

function subscribeToRecord(key: string | null, listener: () => void) {
  if (!key) return () => {};
  const listeners = lateAdmissionListeners.get(key) ?? new Set<() => void>();
  listeners.add(listener);
  lateAdmissionListeners.set(key, listeners);
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0) lateAdmissionListeners.delete(key);
  };
}

function resetRecord(key: string | null) {
  if (!key) return;
  const record = lateAdmissionRecords.get(key);
  if (!record) return;
  record.state = IDLE_LATE_CLARIFICATION_STATE;
  notifyRecord(key);
}

type StoreApi = ReturnType<typeof useAppStoreApi>;

async function hydrateSourceSession({
  storeApi,
  taskId,
  sessionId,
  unavailableMessage,
}: {
  storeApi: StoreApi;
  taskId: string;
  sessionId: string;
  unavailableMessage: string;
}) {
  const state = storeApi.getState();
  const cachedSession = state.taskSessions.items[sessionId];
  if (cachedSession && cachedSession.task_id !== taskId) {
    throw new MessageSendError("session-unavailable", unavailableMessage);
  }

  const needsSession = cachedSession === undefined;
  const needsMessages = !Object.prototype.hasOwnProperty.call(state.messages.bySession, sessionId);
  if (!needsSession && !needsMessages) return;

  const [sessionResponse, messagesResponse] = await Promise.all([
    needsSession ? fetchTaskSession(sessionId, { cache: "no-store" }) : null,
    needsMessages
      ? listTaskSessionMessages(sessionId, { limit: 100, sort: "asc" }, { cache: "no-store" })
      : null,
  ]);

  if (sessionResponse?.session) {
    if (sessionResponse.session.task_id !== taskId) {
      throw new MessageSendError("session-unavailable", unavailableMessage);
    }
    storeApi.getState().setTaskSession(sessionResponse.session);
  }
  if (messagesResponse) {
    const messages = messagesResponse.messages ?? [];
    storeApi.getState().mergeMessages(sessionId, messages, {
      historyInitialized: true,
      hasMore: messagesResponse.has_more ?? false,
      oldestCursor: messages[0]?.id ?? null,
    });
  }
}

export function useLateClarificationMessage(sourceMessage: Message | undefined) {
  const { t } = useTranslation();
  const storeApi = useAppStoreApi();
  const { taskId, sessionId } = sourceIds(sourceMessage);
  const sourceKey = sourceAdmissionKey(sourceMessage);
  const excludedPendingIdRef = useRef<string | null>(null);

  const subscribe = useCallback(
    (listener: () => void) => subscribeToRecord(sourceKey, listener),
    [sourceKey],
  );
  const getSnapshot = useCallback(() => stateForKey(sourceKey), [sourceKey]);
  const state = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);

  const getHasPendingClarification = useCallback(() => {
    if (!sessionId) return false;
    const messages = storeApi.getState().messages?.bySession?.[sessionId] ?? [];
    const pending = findPendingClarification(messages);
    if (!pending) return false;
    const pendingId = pendingIdFromMessage(pending);
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
      if (!taskId || !sessionId) {
        throw new MessageSendError("no-active-session", t("task:lateAnswerFailed"));
      }
      const pendingId = pendingIdFromMessage(snapshot.messages[0]);
      excludedPendingIdRef.current = pendingId;
      const message = formatLateClarificationMessage(snapshot.messages, snapshot.answers, {
        questionLabel: t("task:lateAnswerQuestionLabel"),
        answerLabel: t("task:lateAnswerAnswerLabel"),
        contextLabel: t("task:lateAnswerContextLabel"),
      });
      const admissionKey = lateAdmissionKey(taskId, sessionId, snapshot.messages);
      const record = recordForKey(admissionKey);
      if (record.inFlight) return record.inFlight;
      if (record.state.status === "sent" || record.state.status === "queued") {
        record.clientMessageId = generateUUID();
      }
      const admissionSnapshot: LateClarificationSnapshot = {
        messages: snapshot.messages.slice(),
        answers: snapshot.answers.slice(),
      };
      updateRecordState(admissionKey, { status: "sending", snapshot: admissionSnapshot });
      const delivery = (async () => {
        try {
          await hydrateSourceSession({
            storeApi,
            taskId,
            sessionId,
            unavailableMessage: t("task:sessionNotAvailableForInput"),
          });
          const outcome = await handleSendMessageWithOutcome({
            message,
            clientMessageId: record.clientMessageId,
          });
          if (outcome === false || outcome === undefined) {
            throw new MessageSendError("late-answer-admission-failed", t("task:lateAnswerFailed"));
          }
          updateRecordState(admissionKey, { status: outcome, snapshot: admissionSnapshot });
          return outcome;
        } catch (error) {
          updateRecordState(admissionKey, {
            status: "error",
            error,
            snapshot: admissionSnapshot,
          });
          throw error;
        }
      })();
      record.inFlight = delivery;
      void delivery.then(
        () => {
          if (record.inFlight === delivery) record.inFlight = null;
        },
        () => {
          if (record.inFlight === delivery) record.inFlight = null;
        },
      );
      return delivery;
    },
    [handleSendMessageWithOutcome, sessionId, storeApi, t, taskId],
  );

  const reset = useCallback(() => resetRecord(sourceKey), [sourceKey]);

  return { state, send, reset, taskId, sessionId };
}
