import { groupCommentsByFile } from "@/lib/state/slices/comments/group-review";
import { useMemo } from "react";
import { isReviewComment, useCommentsStore, type ReviewComment } from "@/lib/state/slices/comments";

/** Repository-scoped whole-file feedback alongside legacy line-comment groups. */
export function usePendingReviewCommentsByFile(
  sessionId?: string | null,
): Record<string, ReviewComment[]> {
  const byId = useCommentsStore((state) => state.byId);
  const pending = useCommentsStore((state) => state.pendingForChat);
  return useMemo(() => {
    if (!sessionId) return {};
    const comments = pending.flatMap((id) => {
      const comment = byId[id];
      return comment && isReviewComment(comment) && comment.sessionId === sessionId
        ? [comment]
        : [];
    });
    const groups: Record<string, ReviewComment[]> = {};
    for (const group of groupCommentsByFile(comments)) {
      groups[group.key] = group.comments.map((comment) =>
        comment.repositoryName === undefined && group.repositoryName !== undefined
          ? { ...comment, repositoryName: group.repositoryName }
          : comment,
      );
    }
    return groups;
  }, [byId, pending, sessionId]);
}
