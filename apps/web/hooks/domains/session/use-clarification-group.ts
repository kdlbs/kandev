"use client";

/* eslint-disable max-lines -- clarification submission and recovery share one wire contract. */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { ClarificationAnswer, ClarificationRequestMetadata, Message } from "@/lib/types/http";
import { getBackendConfig } from "@/lib/config";
import { useAppStoreApi } from "@/components/state-provider";
import { isPendingClarificationMessage } from "@/lib/utils/pending-clarification";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";

type SubmitState = "idle" | "submitting" | "ok" | "error" | "expired";

// The only stable machine-readable 409 cause the backend sends today
// (internal/clarification/handlers.go's writeResolutionResult, guarded by
// IsNotActiveError). A duplicate submit never produces a 409 -- it resolves
// through the 200 win/loss envelope below (claimed: true/false) -- so any
// A 409 this client does not recognize is treated as an error rather than
// risked as a silent success.
const CLARIFICATION_CONFLICT_NOT_ACTIVE = "not_active";
const CLARIFICATION_RESPONSE_TIMEOUT_MS = 40_000;

// The bundle status the backend can report on a resolved response (R10).
// Upstream's claim cannot produce a cancelled winner, so a loss only ever
// resolves to one of these two (W3a is retired: no "cancelled" union member).
export type ResolvedStatus = "answered" | "rejected";

// Parsed shape of the clarification respond/cancel envelope
// (internal/clarification/handlers.go's writeResolutionResult). Both the
// answer batch and the skip/reject call hit the same endpoint and get the
// same envelope back.
export type ClarificationRespondResult = {
  state: SubmitState;
  // Present only when the server returned a parseable 200 body. Absent on a
  // 409 (legacy backend, no body), malformed 200 body, or a network/non-2xx
  // failure — callers treat an absent `claimed` the same as an older backend
  // that never sent one (W3: keep applying this client's own answers).
  claimed?: boolean;
  status?: ResolvedStatus;
  answers?: ClarificationAnswer[];
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isResolvedStatus(value: unknown): value is ResolvedStatus {
  return value === "answered" || value === "rejected";
}

function isClarificationAnswer(value: unknown): value is ClarificationAnswer {
  if (!isRecord(value) || typeof value.question_id !== "string") return false;
  const selectedOptions = value.selected_options;
  if (
    selectedOptions !== undefined &&
    (!Array.isArray(selectedOptions) ||
      !selectedOptions.every((option) => typeof option === "string"))
  ) {
    return false;
  }
  const customText = value.custom_text;
  return customText === undefined || typeof customText === "string";
}

function parseClarificationAnswers(value: unknown): ClarificationAnswer[] | null | undefined {
  if (value === undefined) return undefined;
  if (!Array.isArray(value) || !value.every(isClarificationAnswer)) return null;
  return value;
}

function parseOptionalClaimed(value: unknown): boolean | null | undefined {
  if (value === undefined) return undefined;
  return typeof value === "boolean" ? value : null;
}

function parseOptionalStatus(value: unknown): ResolvedStatus | null | undefined {
  if (value === undefined) return undefined;
  return isResolvedStatus(value) ? value : null;
}

type ParsedResponseFields = {
  response?: Record<string, unknown>;
  answers?: ClarificationAnswer[];
};

function parseResponseFields(value: unknown): ParsedResponseFields | null {
  if (value === undefined || value === null) return {};
  if (!isRecord(value)) return null;
  const answers = parseClarificationAnswers(value.answers);
  if (answers === null) return null;
  return { response: value, answers };
}

// A 200 is authoritative only when it has the response envelope introduced by
// the clarification resolver. Older backends may omit claimed, but they must
// still identify a successful response explicitly.
function parseClarificationResponseBody(value: unknown): ClarificationRespondResult | null {
  if (!isRecord(value) || value.success !== true) return null;
  const claimed = parseOptionalClaimed(value.claimed);
  const status = parseOptionalStatus(value.status);
  const responseFields = parseResponseFields(value.response);
  if (claimed === null || status === null || responseFields === null) return null;
  if (
    claimed === false &&
    (status === undefined ||
      responseFields.response === undefined ||
      responseFields.answers === undefined)
  ) {
    return null;
  }
  return { state: "ok", claimed, status, ...responseFields };
}

// ClarificationOutcome is the settled-submission report every outcome
// consumer needs: built directly from the same ClarificationRespondResult
// this hook already parses (design-01#Components /
// design-02#Failure-and-recovery), so no host (including the Needs-you
// Inbox) ever derives a second representation of "what happened" from raw
// wire fields.
export type ClarificationOutcome =
  | { kind: "resolved"; claimedByThisCaller: boolean; status?: ResolvedStatus }
  | { kind: "no_longer_active" }
  | { kind: "submission_failed" };

function clarificationOutcome(result: ClarificationRespondResult): ClarificationOutcome {
  if (result.state === "expired") return { kind: "no_longer_active" };
  if (result.state === "error") return { kind: "submission_failed" };
  return {
    kind: "resolved",
    claimedByThisCaller: result.claimed !== false,
    status: result.status,
  };
}

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
  // The most recent settled ClarificationRespondResult this submission still
  // owned (see runClarificationRequest's ownsRequest fence), or null before
  // any submission has settled. Callers that need to distinguish "this
  // caller won" from "another caller won" read this alongside submitState;
  // see ClarificationOutcome / the outcome callback threaded through
  // ClarificationInputOverlay -> ClarificationPanelSection.
  lastResult: ClarificationRespondResult | null;
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

function clarificationBundleStateKey(messages: readonly Message[] | null | undefined): string {
  return JSON.stringify(
    (messages ?? []).map((message) => {
      const metadata = message.metadata as ClarificationRequestMetadata | undefined;
      return [
        message.session_id,
        message.id,
        metadata?.pending_id ?? null,
        metadata?.status ?? null,
      ];
    }),
  );
}

function hasNewerMessageVersion(current: Message, submitted: Message): boolean {
  if (!current.updated_at) return false;
  if (!submitted.updated_at) return true;

  const currentTime = parseTurnTimestamp(current.updated_at);
  const submittedTime = parseTurnTimestamp(submitted.updated_at);
  if (currentTime === null || submittedTime === null) return false;
  return currentTime > submittedTime;
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
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), CLARIFICATION_RESPONSE_TIMEOUT_MS);
  try {
    const res = await fetch(`${apiBaseUrl}/api/v1/clarification/${pendingId}/respond`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      signal: controller.signal,
    });
    if (res.status === 409) return await classifyConflictResult(res);
    if (!res.ok) {
      console.error("Clarification request failed:", res.status, res.statusText);
      return { state: "error" };
    }
    try {
      const parsed = parseClarificationResponseBody(await res.json());
      if (!parsed) {
        console.error("Clarification response body failed envelope validation");
        return { state: "error" };
      }
      return parsed;
    } catch (err) {
      console.error("Clarification response body parse failed:", err);
      return { state: "error" };
    }
  } catch (err) {
    console.error("Clarification request failed:", err);
    return { state: "error" };
  } finally {
    clearTimeout(timeoutId);
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
  setLastResult: (result: ClarificationRespondResult | null) => void;
  onOutcome?: (outcome: ClarificationOutcome) => void;
  inactivePendingIdRef: { current: string | null };
  getLatestMessage: (sessionId: string, messageId: string) => Message | undefined;
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
    setLastResult,
    onOutcome,
    inactivePendingIdRef,
    getLatestMessage,
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
    const hasNewerAuthority =
      result.state === "expired" &&
      hasNewerAuthoritativeMessage(bundle, requestPendingId, getLatestMessage);
    if (ownsRequest() && !hasNewerAuthority) {
      setSubmitState(result.state);
      setLastResult(result);
      onOutcome?.(clarificationOutcome(result));
      if (result.state === "expired") inactivePendingIdRef.current = requestPendingId;
    } else if (ownsRequest() && hasNewerAuthority) {
      setSubmitState("idle");
      setLastResult(null);
    }
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
    } else if (result.state === "expired" && !hasNewerAuthority) {
      safeApplyExpiredStatus({
        bundle,
        pendingId: requestPendingId,
        requestGeneration,
        activePendingIdRef,
        requestGenerationRef,
        getLatestMessage,
        update: updateMessage,
      });
    }
  } catch (err) {
    console.error("Clarification request threw:", err);
    if (ownsRequest()) {
      setSubmitState("error");
      setLastResult({ state: "error" });
      onOutcome?.({ kind: "submission_failed" });
    }
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

type ExpiredStatusReconciliationArgs = {
  bundle: readonly Message[];
  pendingId: string;
  requestGeneration: number;
  activePendingIdRef: { current: string | null };
  requestGenerationRef: { current: number };
  getLatestMessage: (sessionId: string, messageId: string) => Message | undefined;
  update: (message: Message) => void;
};

function applyExpiredStatusToBundle({
  bundle,
  pendingId,
  requestGeneration,
  activePendingIdRef,
  requestGenerationRef,
  getLatestMessage,
  update,
}: ExpiredStatusReconciliationArgs) {
  if (
    activePendingIdRef.current === pendingId &&
    requestGenerationRef.current !== requestGeneration
  )
    return;

  for (const submitted of bundle) {
    const submittedMeta = submitted.metadata as ClarificationRequestMetadata | undefined;
    if (submittedMeta?.pending_id !== pendingId) continue;

    const current = getLatestMessage(submitted.session_id, submitted.id);
    if (!current || current.session_id !== submitted.session_id) continue;
    const currentMeta = current.metadata as ClarificationRequestMetadata | undefined;
    if (currentMeta?.pending_id !== pendingId || !isPendingClarificationMessage(current)) continue;

    update({
      ...current,
      metadata: { ...currentMeta, status: "expired" },
    });
  }
}

function safeApplyExpiredStatus(args: ExpiredStatusReconciliationArgs) {
  if (args.bundle.length === 0) return;
  try {
    applyExpiredStatusToBundle(args);
  } catch (err) {
    console.error("Clarification expired update threw:", err);
  }
}

function hasNewerAuthoritativeMessage(
  bundle: readonly Message[],
  pendingId: string,
  getLatestMessage: (sessionId: string, messageId: string) => Message | undefined,
): boolean {
  return bundle.some((submitted) => {
    const submittedMeta = submitted.metadata as ClarificationRequestMetadata | undefined;
    if (submittedMeta?.pending_id !== pendingId) return false;

    const current = getLatestMessage(submitted.session_id, submitted.id);
    if (!current || current.session_id !== submitted.session_id) return false;
    const currentMeta = current.metadata as ClarificationRequestMetadata | undefined;
    return currentMeta?.pending_id === pendingId && hasNewerMessageVersion(current, submitted);
  });
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
  setLastResult: (result: ClarificationRespondResult | null) => void;
  onOutcome?: (outcome: ClarificationOutcome) => void;
  inactivePendingIdRef: { current: string | null };
  getLatestMessage: (sessionId: string, messageId: string) => Message | undefined;
  updateMessage: (message: Message) => void;
  defaultSkipReason: string;
};

// Factors out the request-plumbing fields shared by submitCollected and
// skipAll's runClarificationRequest calls, keeping useClarificationSubmission
// itself under the file's max-lines-per-function limit.
function baseClarificationRequestArgs(
  requestPendingId: string,
  common: UseClarificationSubmissionArgs,
) {
  return {
    bundle: common.submitBundleRef.current.slice(),
    requestPendingId,
    activePendingIdRef: common.activePendingIdRef,
    requestGenerationRef: common.requestGenerationRef,
    inflightRef: common.inflightRef,
    setSubmitState: common.setSubmitState,
    setLastResult: common.setLastResult,
    onOutcome: common.onOutcome,
    inactivePendingIdRef: common.inactivePendingIdRef,
    getLatestMessage: common.getLatestMessage,
    updateMessage: common.updateMessage,
  };
}

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
    setLastResult,
    onOutcome,
    inactivePendingIdRef,
    getLatestMessage,
    updateMessage,
    defaultSkipReason,
  } = args;
  const lastActionRef = useRef<{ kind: "submit" } | { kind: "skip"; reason: string } | null>(null);

  const submitCollected = useCallback(
    async (override?: Record<string, ClarificationAnswer>) => {
      if (!pendingId) return;
      if (inactivePendingIdRef.current === pendingId) return;
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
        ...baseClarificationRequestArgs(pendingId, args),
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
      setLastResult,
      onOutcome,
      inactivePendingIdRef,
      getLatestMessage,
      updateMessage,
    ],
  );

  const skipAll = useCallback(
    async (reason?: string) => {
      if (!pendingId) return;
      if (inactivePendingIdRef.current === pendingId) return;
      if (inflightRef.current) return;
      const effectiveReason = reason ?? defaultSkipReason;
      lastActionRef.current = { kind: "skip", reason: effectiveReason };
      await runClarificationRequest({
        post: () => postClarificationSkip(pendingId, effectiveReason),
        ownStatus: "rejected",
        ownAnswers: {},
        ...baseClarificationRequestArgs(pendingId, args),
      });
    },
    [
      pendingId,
      submitBundleRef,
      activePendingIdRef,
      requestGenerationRef,
      inflightRef,
      setSubmitState,
      setLastResult,
      onOutcome,
      inactivePendingIdRef,
      getLatestMessage,
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
// eslint-disable-next-line max-lines-per-function -- owns the complete clarification lifecycle and restoration fence.
export function useClarificationGroup(
  messages: readonly Message[] | null | undefined,
  onOutcome?: (outcome: ClarificationOutcome) => void,
): ClarificationGroupApi {
  const { t } = useTranslation();
  const storeApi = useAppStoreApi();
  const [answers, setAnswers] = useState<Record<string, ClarificationAnswer>>({});
  const answersRef = useRef(answers);
  useEffect(() => {
    answersRef.current = answers;
  }, [answers]);
  const [submitState, setSubmitState] = useState<SubmitState>("idle");
  const [lastResult, setLastResult] = useState<ClarificationRespondResult | null>(null);
  // Re-entry guard: multiple submit paths can race (Cmd+Enter inside the
  // custom input fires both the input's onSubmit and onRequestFinalSubmit;
  // a double-click on the Submit button can also race). The hook owns the
  // guarantee that only one POST is in flight at a time.
  const inflightRef = useRef(false);
  const requestGenerationRef = useRef(0);
  const inactivePendingIdRef = useRef<string | null>(null);

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
  const bundleStateKey = useMemo(() => clarificationBundleStateKey(messages), [messages]);

  const getLatestMessage = useCallback(
    (sessionId: string, messageId: string) => {
      const messages = storeApi.getState().messages?.bySession?.[sessionId];
      return messages?.find((message) => message.id === messageId);
    },
    [storeApi],
  );

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
    setLastResult,
    onOutcome,
    inactivePendingIdRef,
    getLatestMessage,
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
      setLastResult(null);
      resetLastAction();
      inflightRef.current = false;
      inactivePendingIdRef.current = null;
    }
  }, [pendingId, resetLastAction, inflightRef]);

  const lastBundleStateKeyRef = useRef(bundleStateKey);
  useEffect(() => {
    const changed = lastBundleStateKeyRef.current !== bundleStateKey;
    lastBundleStateKeyRef.current = bundleStateKey;
    if (!changed || inactivePendingIdRef.current !== pendingId || !pendingId) return;

    const restored = messages?.some((message) => {
      const metadata = message.metadata as ClarificationRequestMetadata | undefined;
      if (metadata?.pending_id !== pendingId || !isPendingClarificationMessage(message))
        return false;
      const current = getLatestMessage(message.session_id, message.id);
      return current ? isPendingClarificationMessage(current) : false;
    });
    if (!restored) return;

    requestGenerationRef.current += 1;
    inactivePendingIdRef.current = null;
    answersRef.current = {};
    setAnswers({});
    setSubmitState("idle");
    setLastResult(null);
    resetLastAction();
    inflightRef.current = false;
  }, [
    answersRef,
    bundleStateKey,
    getLatestMessage,
    inflightRef,
    messages,
    pendingId,
    requestGenerationRef,
    resetLastAction,
  ]);

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
    lastResult,
  };
}
