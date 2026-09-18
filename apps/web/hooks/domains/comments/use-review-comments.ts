import { useMemo } from "react";
import { isReviewComment, useCommentsStore, type ReviewComment } from "@/lib/state/slices/comments";

/** Repository-scoped whole-file feedback alongside legacy line-comment groups. */
export function usePendingReviewCommentsByFile(
  sessionId?: string | null,
): Record<string, ReviewComment[]> {
  const byId = useCommentsStore((state) => state.byId);
  const pending = useCommentsStore((state) => state.pendingForChat);
  return useMemo(() => {
    const groups: Record<string, ReviewComment[]> = {};
    if (!sessionId) return groups;
    for (const id of pending) {
      const comment = byId[id];
      if (!comment || !isReviewComment(comment) || comment.sessionId !== sessionId) continue;
      const key =
        comment.source === "review-file" && comment.repositoryName
          ? JSON.stringify([comment.repositoryName, comment.filePath])
          : comment.filePath;
      (groups[key] ??= []).push(comment);
    }
    return groups;
  }, [byId, pending, sessionId]);
}
