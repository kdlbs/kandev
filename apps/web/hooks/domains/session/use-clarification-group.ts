"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ClarificationAnswer, ClarificationRequestMetadata, Message } from "@/lib/types/http";
import { getBackendConfig } from "@/lib/config";
import { useAppStoreApi } from "@/components/state-provider";

type SubmitState = "idle" | "submitting" | "ok" | "error";

export type ClarificationGroupApi = {
  pendingId: string | null;
  total: number;
  answeredCount: number;
  answers: Record<string, ClarificationAnswer>;
  submitState: SubmitState;
  recordAnswer: (questionId: string, answer: ClarificationAnswer) => void;
  clearAnswer: (questionId: string) => void;
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

// Mark each bundle message as resolved so the overlay closes regardless of
// whether the backend's WS confirmation event arrives. A long-idle tab can
// leave the WebSocket half-dead (NAT/throttle); the HTTP POST still succeeds
// but the session.message.updated broadcast never lands, which would otherwise
// strand the carousel on "pending" until the user refreshes.
function applyResolvedStatusToBundle(
  bundle: readonly Message[],
  status: "answered" | "rejected",
  answersByQuestionId: Record<string, ClarificationAnswer>,
  update: (message: Message) => void,
) {
  for (const msg of bundle) {
    const meta = (msg.metadata ?? {}) as ClarificationRequestMetadata;
    const questionId = meta.question_id ?? meta.question?.id ?? "";
    const nextMeta: ClarificationRequestMetadata = { ...meta, status };
    const matched = questionId ? answersByQuestionId[questionId] : undefined;
    if (matched) nextMeta.response = matched;
    update({ ...msg, metadata: nextMeta });
  }
}

// Returns a stable callback that flips every message in the current bundle to
// `status` and (for answers) stamps the matching response. Hold a live ref to
// the bundle so the callback always observes the latest messages without
// having to be re-created when the array reference changes.
function useApplyResolvedStatus(messages: readonly Message[] | null | undefined) {
  const storeApi = useAppStoreApi();
  const messagesRef = useRef<readonly Message[] | null | undefined>(messages);
  useEffect(() => {
    messagesRef.current = messages;
  }, [messages]);
  return useCallback(
    (status: "answered" | "rejected", answersByQuestionId: Record<string, ClarificationAnswer>) => {
      const bundle = messagesRef.current;
      if (!bundle || bundle.length === 0) return;
      applyResolvedStatusToBundle(
        bundle,
        status,
        answersByQuestionId,
        storeApi.getState().updateMessage,
      );
    },
    [storeApi],
  );
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
  // Re-entry guard: multiple submit paths can race (Cmd+Enter inside the
  // custom input fires both the input's onSubmit and onRequestFinalSubmit;
  // a double-click on the Submit button can also race). The hook owns the
  // guarantee that only one POST is in flight at a time.
  const inflightRef = useRef(false);
  const applyResolvedStatus = useApplyResolvedStatus(messages);

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

  const clearAnswer = useCallback((questionId: string) => {
    setAnswers((prev) => {
      if (!(questionId in prev)) return prev;
      const next = { ...prev };
      delete next[questionId];
      return next;
    });
  }, []);

  const submitCollected = useCallback(
    async (override?: Record<string, ClarificationAnswer>) => {
      if (!pendingId) return;
      if (inflightRef.current) return;
      const current = { ...answersRef.current, ...(override ?? {}) };
      const haveAll = questionIds.every((id) => Boolean(current[id]));
      if (!haveAll) return;
      inflightRef.current = true;
      setSubmitState("submitting");
      const ordered = questionIds
        .map((id) => current[id])
        .filter((a): a is ClarificationAnswer => Boolean(a));
      try {
        const next = await postClarificationBatch(pendingId, ordered);
        if (next === "ok") applyResolvedStatus("answered", current);
        setSubmitState(next);
      } catch (err) {
        console.error("Clarification submit threw:", err);
        setSubmitState("error");
      } finally {
        inflightRef.current = false;
      }
    },
    [pendingId, questionIds, applyResolvedStatus],
  );

  const skipAll = useCallback(
    async (reason?: string) => {
      if (!pendingId) return;
      if (inflightRef.current) return;
      inflightRef.current = true;
      setSubmitState("submitting");
      try {
        const next = await postClarificationSkip(pendingId, reason ?? "User skipped");
        if (next === "ok") applyResolvedStatus("rejected", {});
        setSubmitState(next);
      } catch (err) {
        console.error("Clarification skip threw:", err);
        setSubmitState("error");
      } finally {
        inflightRef.current = false;
      }
    },
    [pendingId, applyResolvedStatus],
  );

  return {
    pendingId,
    total,
    answeredCount,
    answers,
    submitState,
    recordAnswer,
    clearAnswer,
    submitCollected,
    skipAll,
  };
}
