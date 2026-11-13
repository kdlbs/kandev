import { useCallback, useEffect, useRef, useState } from "react";
import type { SubmitState } from "@/hooks/domains/session/use-clarification-group";

export function isQuestionReviewAvailable(args: {
  hasImages: boolean;
  lateInteraction: boolean;
  submitState: SubmitState;
}): boolean {
  return (
    args.hasImages &&
    !args.lateInteraction &&
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

  return { available, open, openReview, closeReview, triggerRef, scopeRef };
}

export type QuestionReviewController = ReturnType<typeof useQuestionReview>;
