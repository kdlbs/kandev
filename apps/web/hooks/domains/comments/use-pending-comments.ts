import { useMemo } from "react";
import { useCommentsStore } from "@/lib/state/slices/comments";
import type {
  Comment,
  DiffComment,
  PlanComment,
  PRFeedbackComment,
} from "@/lib/state/slices/comments";
import { isDiffComment, isPlanComment, isPRFeedbackComment } from "@/lib/state/slices/comments";

const EMPTY_COMMENTS: Comment[] = [];
const EMPTY_DIFF_COMMENTS: DiffComment[] = [];
const EMPTY_PLAN_COMMENTS: PlanComment[] = [];
const EMPTY_PR_FEEDBACK_COMMENTS: PRFeedbackComment[] = [];

/**
 * Get all pending comments (any source).
 */
export function usePendingComments(): Comment[] {
  const byId = useCommentsStore((state) => state.byId);
  const pendingForChat = useCommentsStore((state) => state.pendingForChat);

  return useMemo(() => {
    if (pendingForChat.length === 0) return EMPTY_COMMENTS;
    const pending: Comment[] = [];
    for (const id of pendingForChat) {
      const comment = byId[id];
      if (comment) pending.push(comment);
    }
    return pending.length === 0 ? EMPTY_COMMENTS : pending;
  }, [byId, pendingForChat]);
}

/**
 * Get all pending diff comments.
 */
export function usePendingDiffComments(): DiffComment[] {
  const byId = useCommentsStore((state) => state.byId);
  const pendingForChat = useCommentsStore((state) => state.pendingForChat);

  return useMemo(() => {
    if (pendingForChat.length === 0) return EMPTY_DIFF_COMMENTS;
    const pending: DiffComment[] = [];
    for (const id of pendingForChat) {
      const comment = byId[id];
      if (comment && isDiffComment(comment)) pending.push(comment);
    }
    return pending.length === 0 ? EMPTY_DIFF_COMMENTS : pending;
  }, [byId, pendingForChat]);
}

/**
 * Get all pending plan comments.
 * If sessionId is provided, only returns comments belonging to that session.
 */
export function usePendingPlanComments(sessionId?: string | null): PlanComment[] {
  const byId = useCommentsStore((state) => state.byId);
  const pendingForChat = useCommentsStore((state) => state.pendingForChat);

  return useMemo(() => {
    if (pendingForChat.length === 0) return EMPTY_PLAN_COMMENTS;
    const pending: PlanComment[] = [];
    for (const id of pendingForChat) {
      const comment = byId[id];
      if (comment && isPlanComment(comment)) {
        // Filter by sessionId if provided
        if (sessionId && comment.sessionId !== sessionId) continue;
        pending.push(comment);
      }
    }
    return pending.length === 0 ? EMPTY_PLAN_COMMENTS : pending;
  }, [byId, pendingForChat, sessionId]);
}

/**
 * Get all pending PR feedback comments.
 * If sessionId is provided, only returns comments belonging to that session.
 */
export function usePendingPRFeedback(sessionId?: string | null): PRFeedbackComment[] {
  const byId = useCommentsStore((state) => state.byId);
  const pendingForChat = useCommentsStore((state) => state.pendingForChat);

  return useMemo(() => {
    if (pendingForChat.length === 0) return EMPTY_PR_FEEDBACK_COMMENTS;
    const pending: PRFeedbackComment[] = [];
    for (const id of pendingForChat) {
      const comment = byId[id];
      if (comment && isPRFeedbackComment(comment)) {
        // Filter by sessionId if provided
        if (sessionId && comment.sessionId !== sessionId) continue;
        pending.push(comment);
      }
    }
    return pending.length === 0 ? EMPTY_PR_FEEDBACK_COMMENTS : pending;
  }, [byId, pendingForChat, sessionId]);
}
