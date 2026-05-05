"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ClarificationAnswer, ClarificationRequestMetadata, Message } from "@/lib/types/http";
import { getBackendConfig } from "@/lib/config";

type SubmitState = "idle" | "submitting" | "ok" | "error";

export type ClarificationGroupApi = {
  pendingId: string | null;
  total: number;
  answeredCount: number;
  answers: Record<string, ClarificationAnswer>;
  submitState: SubmitState;
  recordAnswer: (questionId: string, answer: ClarificationAnswer) => void;
  // Submits every recorded answer in a single batch. An optional `override`
  // map is merged into the current answers right before the POST so callers
  // can safely auto-submit immediately after recording an answer (the React
  // state update is async, so the hook's stored map may not include the
  // freshly recorded answer yet).
  submitCollected: (override?: Record<string, ClarificationAnswer>) => Promise<void>;
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
  // 409 Conflict means a duplicate submit (the user clicked Submit twice in
  // quick succession). Treat it as success — the first submit already won
  // and resolved the bundle on the backend, so the overlay should close.
  if (res.status === 409) return "ok";
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
  if (res.status === 409) return "ok";
  console.error("Clarification skip failed:", res.status, res.statusText);
  return "error";
}

// useClarificationGroup tracks the per-question answers for a multi-question
// clarification bundle. The carousel UI owns navigation; this hook just stores
// the local answer state and exposes:
//   - recordAnswer:    write a single question's answer to local state
//   - submitCollected: POST every recorded answer in one batch (called from
//                      the explicit "Submit answers" button on the last step)
//   - skipAll:         reject the entire bundle.
// Decision A is preserved (per-question commit, batched on the wire) but the
// final submit is no longer implicit — the user clicks "Submit answers" or
// presses ArrowRight on the last step.
export function useClarificationGroup(
  messages: readonly Message[] | null | undefined,
): ClarificationGroupApi {
  const [answers, setAnswers] = useState<Record<string, ClarificationAnswer>>({});
  const answersRef = useRef(answers);
  useEffect(() => {
    answersRef.current = answers;
  }, [answers]);
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

  const recordAnswer = useCallback((questionId: string, answer: ClarificationAnswer) => {
    setAnswers((prev) => ({ ...prev, [questionId]: answer }));
  }, []);

  const submitCollected = useCallback(
    async (override?: Record<string, ClarificationAnswer>) => {
      if (!pendingId) return;
      const current = { ...answersRef.current, ...(override ?? {}) };
      const haveAll = questionIds.every((id) => Boolean(current[id]));
      if (!haveAll) return;
      setSubmitState("submitting");
      const ordered = questionIds
        .map((id) => current[id])
        .filter((a): a is ClarificationAnswer => Boolean(a));
      try {
        setSubmitState(await postClarificationBatch(pendingId, ordered));
      } catch (err) {
        console.error("Clarification submit threw:", err);
        setSubmitState("error");
      }
    },
    [pendingId, questionIds],
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

  return {
    pendingId,
    total,
    answeredCount,
    answers,
    submitState,
    recordAnswer,
    submitCollected,
    skipAll,
  };
}
