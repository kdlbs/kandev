"use client";

import { useCallback, useMemo, useState } from "react";
import type { ClarificationAnswer, ClarificationRequestMetadata, Message } from "@/lib/types/http";
import { getBackendConfig } from "@/lib/config";

type SubmitState = "idle" | "submitting" | "ok" | "expired" | "error";

export type ClarificationGroupApi = {
  pendingId: string | null;
  total: number;
  answeredCount: number;
  answers: Record<string, ClarificationAnswer>;
  submitState: SubmitState;
  recordAnswer: (questionId: string, answer: ClarificationAnswer) => Promise<void>;
  skipAll: (reason?: string) => Promise<void>;
};

function questionIdsFromMessages(messages: readonly Message[]): string[] {
  return messages
    .slice()
    .sort((a, b) => {
      const ai = (a.metadata as ClarificationRequestMetadata | undefined)?.question_index ?? 0;
      const bi = (b.metadata as ClarificationRequestMetadata | undefined)?.question_index ?? 0;
      return ai - bi;
    })
    .map((m) => {
      const meta = m.metadata as ClarificationRequestMetadata | undefined;
      return meta?.question_id ?? meta?.question?.id ?? "";
    })
    .filter(Boolean);
}

async function postClarificationBatch(
  pendingId: string,
  answers: ClarificationAnswer[],
): Promise<SubmitState> {
  const { apiBaseUrl } = getBackendConfig();
  const res = await fetch(`${apiBaseUrl}/api/v1/clarification/${pendingId}/respond`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ answers, rejected: false }),
  });
  if (res.ok) return "ok";
  if (res.status === 410) return "expired";
  console.error("Clarification submit failed:", res.status, res.statusText);
  return "error";
}

async function postClarificationSkip(pendingId: string, reason: string): Promise<SubmitState> {
  const { apiBaseUrl } = getBackendConfig();
  const res = await fetch(`${apiBaseUrl}/api/v1/clarification/${pendingId}/respond`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ rejected: true, reject_reason: reason }),
  });
  if (res.ok) return "ok";
  if (res.status === 410) return "expired";
  console.error("Clarification skip failed:", res.status, res.statusText);
  return "error";
}

// useClarificationGroup coordinates per-question answers for a multi-question
// clarification bundle. Decision A (per-question commit, batched on the wire):
// every option click / custom-text Enter writes the answer locally, and once
// the last question is answered the hook posts every answer in a single
// request. The agent unblocks at that point and receives a map keyed by id.
export function useClarificationGroup(
  messages: readonly Message[] | null | undefined,
): ClarificationGroupApi {
  const [answers, setAnswers] = useState<Record<string, ClarificationAnswer>>({});
  const [submitState, setSubmitState] = useState<SubmitState>("idle");

  const pendingId = useMemo(() => {
    if (!messages || messages.length === 0) return null;
    const meta = messages[0].metadata as ClarificationRequestMetadata | undefined;
    return meta?.pending_id ?? null;
  }, [messages]);

  const questionIds = useMemo(
    () => (messages ? questionIdsFromMessages(messages) : []),
    [messages],
  );
  const total = questionIds.length;
  const answeredCount = Object.keys(answers).filter((id) => questionIds.includes(id)).length;

  const recordAnswer = useCallback(
    async (questionId: string, answer: ClarificationAnswer) => {
      if (!pendingId) return;
      const next = { ...answers, [questionId]: answer };
      setAnswers(next);

      const haveAll = questionIds.every((id) => Boolean(next[id]));
      if (!haveAll) return;

      setSubmitState("submitting");
      const ordered = questionIds
        .map((id) => next[id])
        .filter((a): a is ClarificationAnswer => Boolean(a));
      try {
        setSubmitState(await postClarificationBatch(pendingId, ordered));
      } catch (err) {
        console.error("Clarification submit threw:", err);
        setSubmitState("error");
      }
    },
    [answers, pendingId, questionIds],
  );

  const skipAll = useCallback(
    async (reason?: string) => {
      if (!pendingId) return;
      setSubmitState("submitting");
      try {
        setSubmitState(await postClarificationSkip(pendingId, reason ?? "User skipped"));
      } catch (err) {
        console.error("Clarification skip threw:", err);
        setSubmitState("error");
      }
    },
    [pendingId],
  );

  return { pendingId, total, answeredCount, answers, submitState, recordAnswer, skipAll };
}
