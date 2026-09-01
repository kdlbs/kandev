"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { ClarificationAnswer, ClarificationRequestMetadata, Message } from "@/lib/types/http";
import { getBackendConfig } from "@/lib/config";
import { useAppStoreApi } from "@/components/state-provider";

type SubmitState = "idle" | "submitting" | "ok" | "error" | "expired";

// The only stable machine-readable 409 cause the backend sends today
// (internal/clarification/handlers.go's writeResolutionResult, guarded by
// IsNotActiveError). A duplicate submit never produces a 409 -- it resolves
// through the 200 win/loss envelope below (claimed: true/false) -- so any
// A 409 this client does not recognize is treated as an error rather than
// risked as a silent success.
const CLARIFICATION_CONFLICT_NOT_ACTIVE = "not_active";

// The bundle status the backend can report on a resolved response (R10).
// Upstream's claim cannot produce a cancelled winner, so a loss only ever
// resolves to one of these two (W3a is retired: no "cancelled" union member).
type ResolvedStatus = "answered" | "rejected";

// Parsed shape of the clarification respond/cancel envelope
// (internal/clarification/handlers.go's writeResolutionResult). Both the
// answer batch and the skip/reject call hit the same endpoint and get the
// same envelope back.
type ClarificationRespondResult = {
  state: SubmitState;
  // Present only when the server returned a parseable 200 body. Absent on a
  // 409 (legacy backend, no body) or a network/non-2xx failure — callers
  // treat an absent `claimed` the same as an older backend that never sent
  // one (W3: keep applying this client's own answers).
  claimed?: boolean;
  status?: ResolvedStatus;
  answers?: ClarificationAnswer[];
};

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
  // Re-attempts whichever of submitCollected/skipAll was last invoked. Submit
  // retries use the current live answers; skip retries keep the original
  // reason. A no-op before either has been called.
  retry: () => Promise<void>;
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

// classifyConflictResult reads a 409 response's body for a machine-readable
// `code` (added alongside the existing human `error` string). A bodyless 409
// (legacy backend) or an explicit "not_active" code both mean the bundle is
// no longer active. A malformed nonempty body is an error, because a proxy or
// server failure must not be mistaken for an expired clarification. Any other
// code is unrecognized by this client -- fail closed to "error" rather than
// guessing it is still safe to report success.
async function classifyConflictResult(res: Response): Promise<ClarificationRespondResult> {
  const body = await res.text();
  if (!body.trim()) return { state: "expired" };

  let parsed: { code?: string };
  try {
    parsed = JSON.parse(body) as { code?: string };
  } catch {
    console.error("Clarification request failed: malformed 409 body");
    return { state: "error" };
  }
  const code = parsed.code;
  if (code === undefined || code === CLARIFICATION_CONFLICT_NOT_ACTIVE) {
    return { state: "expired" };
  }
  console.error("Clarification request failed: unrecognized 409 code", code);
  return { state: "error" };
}

// postClarification posts the respond body and, on a 200, parses the R10
// envelope so the caller can tell a win from a loss (W3) and read the
// winner's own status/answers off the same response. credentials:
// "include" matches the shared client (lib/api/client.ts) — without it,
// split-origin dev mode drops the session cookie and an auth-enabled backend
// rejects the request before ever reaching the resolver (W1).
async function postClarification(
  pendingId: string,
  body: Record<string, unknown>,
): Promise<ClarificationRespondResult> {
  const { apiBaseUrl } = getBackendConfig();
  let res: Response;
  try {
    res = await fetch(`${apiBaseUrl}/api/v1/clarification/${pendingId}/respond`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  } catch (err) {
    console.error("Clarification request failed:", err);
    return { state: "error" };
  }
  if (res.status === 409) return await classifyConflictResult(res);
  if (!res.ok) {
    console.error("Clarification request failed:", res.status, res.statusText);
    return { state: "error" };
  }
  try {
    const parsed = (await res.json()) as {
      claimed?: boolean;
      status?: ResolvedStatus;
      response?: { answers?: ClarificationAnswer[] } | null;
    };
    return {
      state: "ok",
      claimed: parsed.claimed,
      status: parsed.status,
      answers: parsed.response?.answers,
    };
  } catch (err) {
    console.error("Clarification response body parse failed:", err);
    return { state: "ok" };
  }
}

function postClarificationBatch(
  pendingId: string,
  answers: ClarificationAnswer[],
): Promise<ClarificationRespondResult> {
  return postClarification(pendingId, { answers, rejected: false });
}

function postClarificationSkip(
  pendingId: string,
  reason: string,
): Promise<ClarificationRespondResult> {
  return postClarification(pendingId, { rejected: true, reject_reason: reason });
}

// resolveOptimisticUpdate picks which status/answers the optimistic update
// should write. A win (claimed true, or claimed absent — an older backend
// that predates R10) keeps today's behavior: this client's own submitted
// status and answers. A loss (claimed: false) applies the winner's returned
// status and answers instead (W3), since R2 guarantees no later WS broadcast
// will ever correct an optimistic write of this client's own losing answers.
function resolveOptimisticUpdate(
  result: ClarificationRespondResult,
  ownStatus: ResolvedStatus,
  ownAnswers: Record<string, ClarificationAnswer>,
): { status: ResolvedStatus; answersByQuestionId: Record<string, ClarificationAnswer> } {
  if (result.claimed === false) {
    const answersByQuestionId: Record<string, ClarificationAnswer> = {};
    for (const answer of result.answers ?? []) {
      answersByQuestionId[answer.question_id] = answer;
    }
    return { status: result.status ?? ownStatus, answersByQuestionId };
  }
  return { status: ownStatus, answersByQuestionId: ownAnswers };
}

type RunClarificationRequestArgs = {
  post: () => Promise<ClarificationRespondResult>;
  ownStatus: ResolvedStatus;
  ownAnswers: Record<string, ClarificationAnswer>;
  bundle: readonly Message[];
  // The pendingId this request was submitted for, captured at call time.
  // Compared against activePendingIdRef when the POST resolves so a request
  // that outlives its own bundle (the next clarification streamed in while
  // this one was still in flight) can't paint its outcome onto the bundle
  // that replaced it, or release a mutex a newer bundle's own request now owns.
  requestPendingId: string;
  activePendingIdRef: { current: string | null };
  requestGenerationRef: { current: number };
  inflightRef: { current: boolean };
  setSubmitState: (state: SubmitState) => void;
  updateMessage: (message: Message) => void;
};

// Shared submit/skip plumbing: guard re-entry, POST, then apply the
// optimistic update on success. Extracted so submitCollected and skipAll
// (below) each stay a short wrapper around their own POST + own-answer shape.
async function runClarificationRequest(args: RunClarificationRequestArgs) {
  const {
    post,
    ownStatus,
    ownAnswers,
    bundle,
    requestPendingId,
    activePendingIdRef,
    requestGenerationRef,
    inflightRef,
    setSubmitState,
    updateMessage,
  } = args;
  const requestGeneration = ++requestGenerationRef.current;
  const ownsRequest = () =>
    activePendingIdRef.current === requestPendingId &&
    requestGenerationRef.current === requestGeneration;
  inflightRef.current = true;
  setSubmitState("submitting");
  try {
    const result = await post();
    if (ownsRequest()) setSubmitState(result.state);
    if (result.state === "ok") {
      // Applies against the submit-time bundle snapshot regardless of which
      // bundle is now on screen -- this client's own messages really were
      // resolved and the store must reflect that even after a swap.
      const { status, answersByQuestionId } = resolveOptimisticUpdate(
        result,
        ownStatus,
        ownAnswers,
      );
      safeApplyResolvedStatus(bundle, status, answersByQuestionId, updateMessage);
    }
  } catch (err) {
    console.error("Clarification request threw:", err);
    if (ownsRequest()) setSubmitState("error");
  } finally {
    // Only release the mutex if this exact request still owns it. A bundle
    // swap can return to the same pending ID before this request settles, so
    // the ID alone is not a sufficient ownership fence.
    if (ownsRequest()) inflightRef.current = false;
  }
}

// Mark each bundle message as resolved so the overlay closes regardless of
// whether the backend's WS confirmation event arrives. A long-idle tab can
// leave the WebSocket half-dead (NAT/throttle); the HTTP POST still succeeds
// but the session.message.updated broadcast never lands, which would otherwise
// strand the carousel on "pending" until the user refreshes.
function applyResolvedStatusToBundle(
  bundle: readonly Message[],
  status: ResolvedStatus,
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

// Best-effort optimistic update — isolated from the submit's own try/catch so
// a thrown store action (missing handler, immer freeze, etc.) can't downgrade
// a successful HTTP submit to submitState === "error".
function safeApplyResolvedStatus(
  bundle: readonly Message[],
  status: ResolvedStatus,
  answersByQuestionId: Record<string, ClarificationAnswer>,
  update: (message: Message) => void,
) {
  if (bundle.length === 0) return;
  try {
    applyResolvedStatusToBundle(bundle, status, answersByQuestionId, update);
  } catch (err) {
    console.error("Clarification optimistic update threw:", err);
  }
}

type UseClarificationSubmissionArgs = {
  pendingId: string | null;
  questionIds: string[];
  answersRef: { current: Record<string, ClarificationAnswer> };
  submitBundleRef: { current: readonly Message[] };
  activePendingIdRef: { current: string | null };
  requestGenerationRef: { current: number };
  inflightRef: { current: boolean };
  setAnswers: (answers: Record<string, ClarificationAnswer>) => void;
  setSubmitState: (state: SubmitState) => void;
  updateMessage: (message: Message) => void;
  defaultSkipReason: string;
};

// Submission plumbing shared by useClarificationGroup: submitCollected/skipAll
// each POST through runClarificationRequest, and retry() replays whichever of
// the two was last attempted. Submit retries read the live answer map so edits
// made after a failure are included; skip retries keep their original reason.
function useClarificationSubmission(args: UseClarificationSubmissionArgs) {
  const {
    pendingId,
    questionIds,
    answersRef,
    submitBundleRef,
    activePendingIdRef,
    requestGenerationRef,
    inflightRef,
    setAnswers,
    setSubmitState,
    updateMessage,
    defaultSkipReason,
  } = args;
  const lastActionRef = useRef<{ kind: "submit" } | { kind: "skip"; reason: string } | null>(null);

  const submitCollected = useCallback(
    async (override?: Record<string, ClarificationAnswer>) => {
      if (!pendingId) return;
      if (inflightRef.current) return;
      const current = { ...answersRef.current, ...(override ?? {}) };
      const haveAll = questionIds.every((id) => Boolean(current[id]));
      if (!haveAll) return;
      const ordered = questionIds
        .map((id) => current[id])
        .filter((a): a is ClarificationAnswer => Boolean(a));
      // Keep the override in the live answer map. Single-question auto-submit
      // records it before this call, but storing it here also makes direct
      // callers and Retry use the same current-answer source.
      answersRef.current = current;
      setAnswers(current);
      lastActionRef.current = { kind: "submit" };
      await runClarificationRequest({
        post: () => postClarificationBatch(pendingId, ordered),
        ownStatus: "answered",
        ownAnswers: current,
        bundle: submitBundleRef.current.slice(),
        requestPendingId: pendingId,
        activePendingIdRef,
        requestGenerationRef,
        inflightRef,
        setSubmitState,
        updateMessage,
      });
    },
    [
      pendingId,
      questionIds,
      answersRef,
      submitBundleRef,
      activePendingIdRef,
      requestGenerationRef,
      inflightRef,
      setAnswers,
      setSubmitState,
      updateMessage,
    ],
  );

  const skipAll = useCallback(
    async (reason?: string) => {
      if (!pendingId) return;
      if (inflightRef.current) return;
      const effectiveReason = reason ?? defaultSkipReason;
      lastActionRef.current = { kind: "skip", reason: effectiveReason };
      await runClarificationRequest({
        post: () => postClarificationSkip(pendingId, effectiveReason),
        ownStatus: "rejected",
        ownAnswers: {},
        bundle: submitBundleRef.current.slice(),
        requestPendingId: pendingId,
        activePendingIdRef,
        requestGenerationRef,
        inflightRef,
        setSubmitState,
        updateMessage,
      });
    },
    [
      pendingId,
      submitBundleRef,
      activePendingIdRef,
      requestGenerationRef,
      inflightRef,
      setSubmitState,
      updateMessage,
      defaultSkipReason,
    ],
  );

  const retry = useCallback(async () => {
    const action = lastActionRef.current;
    if (!action) return;
    if (action.kind === "submit") {
      await submitCollected();
    } else {
      await skipAll(action.reason);
    }
  }, [submitCollected, skipAll]);

  const resetLastAction = useCallback(() => {
    lastActionRef.current = null;
  }, []);

  return { submitCollected, skipAll, retry, resetLastAction };
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
  const { t } = useTranslation();
  const storeApi = useAppStoreApi();
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
  const requestGenerationRef = useRef(0);

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
    const next = { ...answersRef.current, [questionId]: answer };
    answersRef.current = next;
    setAnswers(next);
  }, []);

  const clearAnswer = useCallback((questionId: string) => {
    if (!(questionId in answersRef.current)) return;
    const next = { ...answersRef.current };
    delete next[questionId];
    answersRef.current = next;
    setAnswers(next);
  }, []);

  // Snapshot the bundle at submit time so a re-render that swaps `messages`
  // mid-flight (e.g. the next clarification streaming in) can't make the
  // optimistic update target the wrong messages once the await resolves.
  const submitBundleRef = useRef<readonly Message[]>([]);
  submitBundleRef.current = messages ?? [];

  // Tracks which pendingId is currently on screen so a request whose bundle
  // was swapped out while it was still in flight can tell it's no longer
  // current (see runClarificationRequest).
  const activePendingIdRef = useRef<string | null>(pendingId);
  activePendingIdRef.current = pendingId;

  // i18n-exempt: the default reason is POSTed as the clarification answer and
  // reaches the agent verbatim; it is not rendered in the UI.
  const { submitCollected, skipAll, retry, resetLastAction } = useClarificationSubmission({
    pendingId,
    questionIds,
    answersRef,
    submitBundleRef,
    activePendingIdRef,
    requestGenerationRef,
    inflightRef,
    setAnswers,
    setSubmitState,
    updateMessage: storeApi.getState().updateMessage,
    defaultSkipReason: t("task:userSkippedClarification"),
  });

  // A new bundle (different pendingId) replacing a still-pending one must not
  // inherit the previous bundle's answers, submit/retry banner, or replayable
  // action -- bundle-swap-without-unmount is a designed-for path (see
  // useCollapsedForBundle in clarification-panel-section.tsx), so without this
  // a stale "error"/"expired" banner (and Retry's recorded answers) would
  // render against the live bundle and POST to the wrong pendingId. The old
  // bundle's request, if still in flight, no longer owns the mutex once this
  // runs (runClarificationRequest checks activePendingIdRef before touching
  // it), so releasing it here is what lets the new bundle submit immediately
  // instead of waiting out a request that's no longer for anything on screen.
  const lastPendingIdRef = useRef(pendingId);
  useEffect(() => {
    if (pendingId !== lastPendingIdRef.current) {
      lastPendingIdRef.current = pendingId;
      requestGenerationRef.current += 1;
      answersRef.current = {};
      setAnswers({});
      setSubmitState("idle");
      resetLastAction();
      inflightRef.current = false;
    }
  }, [pendingId, resetLastAction, inflightRef]);

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
    retry,
  };
}
