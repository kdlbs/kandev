import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { LateAnswerState, SubmitState } from "@/hooks/domains/session/use-clarification-group";

/**
 * Review is available while the overlay shows an editable form for an image
 * bundle: a pending bundle or a late-answer form. Resolved, expired, and
 * delivered late answers have no form to review against.
 */
export function isQuestionReviewAvailable(args: {
  hasImages: boolean;
  lateStatus: LateAnswerState;
  submitState: SubmitState;
}): boolean {
  return (
    args.hasImages &&
    args.lateStatus !== "sent" &&
    args.lateStatus !== "queued" &&
    args.submitState !== "ok" &&
    args.submitState !== "expired"
  );
}

/**
 * Owns the open state of the large review view for one clarification overlay.
 * The view is bound to the bundle it was opened for: a replaced bundle or a
 * resolved one closes it, and it never reopens without a new explicit request.
 */
export function useQuestionReview({
  pendingId,
  available,
}: {
  pendingId: string | null;
  available: boolean;
}) {
  const [openFor, setOpenFor] = useState<string | null>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const scopeRef = useRef<HTMLDivElement>(null);
  const open = available && pendingId !== null && openFor === pendingId;

  useEffect(() => {
    if (openFor !== null && (!available || openFor !== pendingId)) setOpenFor(null);
  }, [available, openFor, pendingId]);

  const openReview = useCallback(() => {
    if (available && pendingId) setOpenFor(pendingId);
  }, [available, pendingId]);
  const closeReview = useCallback(() => setOpenFor(null), []);
  const wipeLift = useRoundValue(pendingId);

  return { available, open, openReview, closeReview, triggerRef, scopeRef, wipeLift };
}

/**
 * A view preference that lasts for one question round: it survives closing and
 * reopening the review view, and a new bundle starts without it.
 */
function useRoundValue(pendingId: string | null) {
  const [stored, setStored] = useState<{ round: string; value: number } | null>(null);
  const value = stored && stored.round === pendingId ? stored.value : undefined;
  // A new round drops the old value, so it never returns with an earlier bundle.
  useEffect(() => {
    setStored((current) => (current && current.round !== pendingId ? null : current));
  }, [pendingId]);
  const set = useCallback(
    (next: number) => {
      if (pendingId) setStored({ round: pendingId, value: next });
    },
    [pendingId],
  );
  return useMemo(() => ({ value, set }), [value, set]);
}

export type QuestionReviewController = ReturnType<typeof useQuestionReview>;
