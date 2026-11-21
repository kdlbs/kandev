"use client";

import { useCallback, useRef, useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";
import { useAppStoreApi } from "@/components/state-provider";
import { MessageSendError } from "@/lib/chat/message-send-error";
import { findPendingClarification } from "@/lib/utils/pending-clarification";
import type { Message } from "@/lib/types/http";
import { fetchTaskSession, listTaskSessionMessages } from "@/lib/api/domains/session-api";
import { isStaleSessionStateEvent } from "@/lib/session-state";
import { captureTaskSessionHydrationEpoch } from "@/lib/state/slices/session/hydration-epochs";
import {
  formatLateClarificationMessage,
  orderedLateClarificationAnswers,
  type LateClarificationSnapshot,
} from "@/lib/clarification/late-clarification-message";
import { generateUUID } from "@/lib/utils";
import { markClarificationInboxLateAnswer } from "@/lib/api/domains/clarification-inbox-api";
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
  boundPendingId?: string | null;
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

function sourceIds(
  message: Message | undefined,
  targetSessionId?: string | null,
): {
  taskId: string | null;
  sessionId: string | null;
} {
  if (!message) return { taskId: null, sessionId: null };
  return {
    taskId: message.task_id ?? null,
    sessionId: targetSessionId || (message.session_id ?? null),
  };
}

function sourceAdmissionKey(
  message: Message | undefined,
  targetSessionId?: string | null,
): string | null {
  const { taskId, sessionId } = sourceIds(message, targetSessionId);
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

function bindLateAdmission(record: LateAdmissionRecord, pendingId: string | null) {
  if (
    record.boundPendingId === pendingId &&
    record.state.status !== "sent" &&
    record.state.status !== "queued"
  )
    return;
  const prefix = pendingId ? `late:${pendingId}:` : "";
  const id = `${prefix}${generateUUID()}`;
  // Long legacy identifiers can still be sent and closed manually.
  record.clientMessageId = new TextEncoder().encode(id).length <= 128 ? id : generateUUID();
  record.boundPendingId = pendingId;
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

function unansweredInboxEnabled(storeApi: StoreApi, workspaceId?: string): boolean {
  const workspaces = storeApi.getState().workspaces;
  const active = workspaces?.items.find((item) => item.id === (workspaceId ?? workspaces.activeId));
  return Boolean(active?.clarification_unanswered_inbox_enabled);
}

// With the workspace's unanswered inbox on, an admitted late answer closes
// exactly the question it answered. Best effort: the answer is already
// delivered, and a question that is not unanswered (still active, expired,
// already closed) is rejected server-side without side effects.
async function recordLateAnswer(
  storeApi: StoreApi,
  enabled: boolean,
  pendingId: string | null,
  admission: { messageId: string; sessionId: string },
) {
  if (!enabled || !pendingId) return;
  try {
    await markClarificationInboxLateAnswer(pendingId, admission);
    storeApi.getState().bumpNeedsYouInboxRefreshTick();
  } catch {
    // The question stays listed as unanswered; the user can still close it.
  }
}

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

  // Admission derives direct-vs-queue from the source session state, and a
  // cached row can lag the backend, so the session is always read fresh. The
  // transcript is only loaded when this client has none for the session.
  const needsMessages = !Object.prototype.hasOwnProperty.call(state.messages.bySession, sessionId);
  const hydrationEpochAtRequestStart = captureTaskSessionHydrationEpoch(state, sessionId);

  const [sessionResponse, messagesResponse] = await Promise.all([
    fetchTaskSession(sessionId, { cache: "no-store" }),
    needsMessages
      ? listTaskSessionMessages(sessionId, { limit: 100, sort: "asc" }, { cache: "no-store" })
      : null,
  ]);

  if (sessionResponse?.session) {
    if (sessionResponse.session.task_id !== taskId) {
      throw new MessageSendError("session-unavailable", unavailableMessage);
    }
    const current = storeApi.getState().taskSessions.items[sessionId];
    if (!isStaleSessionStateEvent(current, sessionResponse.session.updated_at)) {
      storeApi.getState().setTaskSession(sessionResponse.session, hydrationEpochAtRequestStart);
    }
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

function snapshotAttachments(snapshot: LateClarificationSnapshot) {
  return orderedLateClarificationAnswers(snapshot.messages, snapshot.answers).flatMap((answer) =>
    (answer.attachments ?? []).map((attachment) => ({
      type: attachment.type,
      attachment_id: attachment.attachment_id,
      mime_type: attachment.mime_type,
      name: attachment.name,
      size_bytes: attachment.size_bytes,
      delivery_mode: attachment.delivery_mode,
    })),
  );
}

function clearInFlightWhenSettled(
  record: LateAdmissionRecord,
  delivery: Promise<MessageAdmissionOutcome>,
) {
  const clear = () => {
    if (record.inFlight === delivery) record.inFlight = null;
  };
  void delivery.then(clear, clear);
}

/**
 * Sends a late answer as an ordinary message. The target defaults to the
 * question's own session; `targetSessionId` is an explicit alternative the
 * user chose (the task's current session after the asking one ended).
 */
export function useLateClarificationMessage(
  sourceMessage: Message | undefined,
  options?: { targetSessionId?: string | null; workspaceId?: string },
) {
  const { t } = useTranslation();
  const storeApi = useAppStoreApi();
  const workspaceId = options?.workspaceId;
  const { taskId, sessionId } = sourceIds(sourceMessage, options?.targetSessionId);
  const sourceKey = sourceAdmissionKey(sourceMessage, options?.targetSessionId);
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
      bindLateAdmission(record, pendingId);
      const admissionSnapshot: LateClarificationSnapshot = {
        messages: snapshot.messages.slice(),
        answers: snapshot.answers.slice(),
      };
      updateRecordState(admissionKey, { status: "sending", snapshot: admissionSnapshot });
      // Capture workspace enablement before asynchronous admission.
      const closeUnanswered = unansweredInboxEnabled(storeApi, workspaceId);
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
            attachments: snapshotAttachments(snapshot),
          });
          if (outcome === false || outcome === undefined) {
            throw new MessageSendError("late-answer-admission-failed", t("task:lateAnswerFailed"));
          }
          updateRecordState(admissionKey, { status: outcome, snapshot: admissionSnapshot });
          await recordLateAnswer(storeApi, closeUnanswered, pendingId, {
            messageId: record.clientMessageId,
            sessionId,
          });
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
      clearInFlightWhenSettled(record, delivery);
      return delivery;
    },
    [handleSendMessageWithOutcome, sessionId, storeApi, t, taskId, workspaceId],
  );

  const reset = useCallback(() => resetRecord(sourceKey), [sourceKey]);

  return { state, send, reset, taskId, sessionId };
}
