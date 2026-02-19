import { useEffect, useMemo } from "react";
import { useCommentsStore } from "@/lib/state/slices/comments";
import type { DiffComment } from "@/lib/state/slices/comments";
import { isDiffComment } from "@/lib/state/slices/comments";

const EMPTY_COMMENTS: DiffComment[] = [];

/**
 * Get all diff comments for a specific file in a session.
 */
export function useDiffFileComments(sessionId: string, filePath: string): DiffComment[] {
  const byId = useCommentsStore((state) => state.byId);
  const sessionIds = useCommentsStore((state) => state.bySession[sessionId]);
  const hydrateSession = useCommentsStore((state) => state.hydrateSession);

  useEffect(() => {
    if (sessionId) hydrateSession(sessionId);
  }, [sessionId, hydrateSession]);

  return useMemo(() => {
    if (!sessionIds || sessionIds.length === 0) return EMPTY_COMMENTS;
    const result: DiffComment[] = [];
    for (const id of sessionIds) {
      const comment = byId[id];
      if (comment && isDiffComment(comment) && comment.filePath === filePath) {
        result.push(comment);
      }
    }
    return result.length === 0 ? EMPTY_COMMENTS : result;
  }, [byId, sessionIds, filePath]);
}

const EMPTY_BY_FILE: Record<string, DiffComment[]> = {};

/**
 * Get all pending diff comments grouped by file path.
 */
export function usePendingDiffCommentsByFile(): Record<string, DiffComment[]> {
  const byId = useCommentsStore((state) => state.byId);
  const pendingForChat = useCommentsStore((state) => state.pendingForChat);

  return useMemo(() => {
    if (pendingForChat.length === 0) return EMPTY_BY_FILE;
    const byFile: Record<string, DiffComment[]> = {};
    for (const id of pendingForChat) {
      const comment = byId[id];
      if (comment && isDiffComment(comment)) {
        if (!byFile[comment.filePath]) byFile[comment.filePath] = [];
        byFile[comment.filePath].push(comment);
      }
    }
    return Object.keys(byFile).length === 0 ? EMPTY_BY_FILE : byFile;
  }, [byId, pendingForChat]);
}
