import { useMemo, useCallback } from "react";
import type { DiffComment, DiffCommentUpdate } from "./types";

/** Build a DiffComment object from common parameters */
export function buildDiffComment(params: {
  filePath: string;
  sessionId: string;
  startLine: number;
  endLine: number;
  side: DiffComment["side"];
  text: string;
  codeContent?: string;
}): DiffComment {
  return {
    id: `${params.filePath}-${Date.now()}`,
    source: "diff",
    sessionId: params.sessionId,
    filePath: params.filePath,
    startLine: Math.min(params.startLine, params.endLine),
    endLine: Math.max(params.startLine, params.endLine),
    side: params.side,
    codeContent: params.codeContent ?? "",
    text: params.text,
    createdAt: new Date().toISOString(),
    status: "pending",
  };
}

/** Compute the set of line numbers that have comments */
export function useCommentedLines(comments: DiffComment[]): Set<number> {
  return useMemo(() => {
    const set = new Set<number>();
    for (const c of comments) {
      for (let l = c.startLine; l <= c.endLine; l++) set.add(l);
    }
    return set;
  }, [comments]);
}

/**
 * Shared delete/update callbacks that handle external-vs-internal branching.
 * Used by both Pierre diff-viewer and Monaco diff-viewer.
 */
export function useCommentActions(params: {
  removeComment: (commentId: string) => void;
  updateComment: (commentId: string, updates: DiffCommentUpdate) => void;
  setEditingComment: (id: string | null) => void;
  onCommentDelete?: (commentId: string) => void;
  onCommentUpdate?: (commentId: string, updates: DiffCommentUpdate) => void;
  externalComments?: DiffComment[];
}) {
  const {
    removeComment,
    updateComment,
    setEditingComment,
    onCommentDelete,
    onCommentUpdate,
    externalComments,
  } = params;

  const handleCommentDelete = useCallback(
    (commentId: string) => {
      if (onCommentDelete && externalComments !== undefined) {
        onCommentDelete(commentId);
      } else {
        removeComment(commentId);
      }
    },
    [removeComment, onCommentDelete, externalComments],
  );

  const handleCommentUpdate = useCallback(
    (commentId: string, content: string) => {
      const updates = { text: content };
      if (externalComments !== undefined) {
        if (onCommentUpdate) {
          onCommentUpdate(commentId, updates);
        } else if (isDevelopmentMode()) {
          console.warn(
            "[DiffViewer] `comments` is set without `onCommentUpdate`; edited comments must be handled by the controlled owner.",
          );
        }
      } else {
        updateComment(commentId, updates);
      }
      setEditingComment(null);
    },
    [updateComment, onCommentUpdate, externalComments, setEditingComment],
  );

  return { handleCommentDelete, handleCommentUpdate };
}

function isDevelopmentMode(): boolean {
  const viteEnv = (
    import.meta as unknown as {
      env?: { DEV?: boolean };
    }
  ).env;
  return (
    viteEnv?.DEV === true ||
    (typeof process !== "undefined" && process.env.NODE_ENV !== "production")
  );
}
