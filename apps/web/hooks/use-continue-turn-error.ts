"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { sendMessageRequest } from "./message-request";
import { deriveSessionInputMode } from "./domains/session/session-input-mode";
import { readLastAgentError } from "@/lib/session-last-agent-error";
import { notifyTurnErrorAcknowledgementFailure } from "@/lib/task-turn-error-feedback";
import type { Message, TaskSession } from "@/lib/types/http";

export type ContinueTurnErrorOutcome = "idle" | "pending" | "accepted" | "failed";

/**
 * Stable client identity for one manual continuation of one observed turn
 * error. Repeated clicks, viewers, remounts, and lost-response reconciliation
 * reuse this exact ID instead of creating another logical prompt.
 */
export function continueTurnErrorMessageID(sessionId: string, stamp: string): string {
  const raw = `continue:${sessionId}:${stamp}`;
  if (new TextEncoder().encode(raw).length <= 128) return raw;
  return `continue:${stableMessageIDHash(raw)}`;
}

function stableMessageIDHash(value: string): string {
  let hash = 0x811c9dc5;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193);
  }
  return `${(hash >>> 0).toString(16).padStart(8, "0")}:${value.length}`;
}

function hasPendingAutomaticRetry(messages: Message[] | undefined) {
  return messages?.some((message) => message.metadata?.retrying === true) ?? false;
}

function hasAcceptedContinuation(messages: Message[] | undefined, messageId: string) {
  return messages?.some((message) => message.id === messageId) ?? false;
}

function canContinueTurnError({
  taskId,
  sessionId,
  stamp,
  messageId,
  session,
  queuedCount,
  accepted,
  retrying,
}: {
  taskId?: string | null;
  sessionId?: string | null;
  stamp?: string | null;
  messageId: string;
  session: TaskSession | undefined;
  queuedCount: number;
  accepted: boolean;
  retrying: boolean;
}) {
  if (!taskId || !sessionId || !stamp || !messageId || !session) return false;
  if (accepted || retrying || continueSessionBlocked(session)) return false;
  if (!isContinueSessionState(session)) return false;
  return deriveSessionInputMode(session, queuedCount) === "direct";
}

function continueSessionBlocked(session: TaskSession): boolean {
  if (session.cancellation_pending || session.pending_action) return true;
  if (readLastAgentError(session.metadata)) return true;
  return (session.session_recovery_blocks?.length ?? 0) > 0;
}

function isContinueSessionState(session: TaskSession): boolean {
  return session.state === "IDLE" || session.state === "WAITING_FOR_INPUT";
}

type PlanModeCapture = { messageId: string; planMode: boolean };

function captureAppliedPlanMode(
  current: PlanModeCapture | null,
  messageId: string,
  session: TaskSession | undefined,
): { planMode: boolean; next: PlanModeCapture } {
  if (current?.messageId === messageId) {
    return { planMode: current.planMode, next: current };
  }
  const planMode = session?.metadata?.plan_mode === true;
  return { planMode, next: { messageId, planMode } };
}

export function useContinueTurnError({
  taskId,
  sessionId,
  stamp,
}: {
  taskId?: string | null;
  sessionId?: string | null;
  stamp?: string | null;
}) {
  const storeApi = useAppStoreApi();
  const [pending, setPending] = useState(false);
  const [outcome, setOutcome] = useState<ContinueTurnErrorOutcome>("idle");
  const inFlight = useRef(false);
  const planModeByMessage = useRef<PlanModeCapture | null>(null);
  const messageId = useMemo(
    () => (sessionId && stamp ? continueTurnErrorMessageID(sessionId, stamp) : ""),
    [sessionId, stamp],
  );
  const session = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId] : undefined,
  );
  const queuedCount = useAppStore((state) =>
    sessionId ? (state.queue.metaBySessionId[sessionId]?.count ?? 0) : 0,
  );
  const retrying = useAppStore((state) =>
    sessionId ? hasPendingAutomaticRetry(state.messages.bySession[sessionId]) : false,
  );
  const accepted = useAppStore((state) =>
    sessionId && messageId
      ? hasAcceptedContinuation(state.messages.bySession[sessionId], messageId)
      : false,
  );

  const available = canContinueTurnError({
    taskId,
    sessionId,
    stamp,
    messageId,
    session,
    queuedCount,
    accepted: accepted || outcome === "accepted",
    retrying,
  });
  const ready = available && !pending;

  const continueTurn = useCallback(async () => {
    if (!ready || inFlight.current || !taskId || !sessionId || !stamp) return;
    inFlight.current = true;
    setPending(true);
    setOutcome("pending");
    try {
      const { planMode, next } = captureAppliedPlanMode(
        planModeByMessage.current,
        messageId,
        storeApi.getState().taskSessions.items[sessionId],
      );
      planModeByMessage.current = next;
      const created = await sendMessageRequest({
        taskId,
        resolvedSessionId: sessionId,
        clientMessageId: messageId,
        // Exact manual content. This is a visible user message, not the
        // hidden automatic continuation instruction.
        // i18n-exempt: exact control prompt sent to the model, never rendered.
        finalMessage: "continue",
        // Omit the model so the backend uses the session's applied
        // configuration. Never consume unsent composer or selector changes.
        modelToSend: undefined,
        planMode,
        observedTurnErrorStamp: stamp,
      });
      notifyTurnErrorAcknowledgementFailure(created?.turn_error_acknowledgement_failed);
      if (created?.id && created.session_id) {
        storeApi.getState().addMessage(created);
      }
      setOutcome("accepted");
    } catch {
      setOutcome("failed");
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }, [messageId, ready, sessionId, stamp, storeApi, taskId]);

  return { available, ready, pending, outcome, messageId, continueTurn };
}
