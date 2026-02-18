import { useMemo } from 'react';
import { create } from 'zustand';
import { persist, createJSONStorage } from 'zustand/middleware';
import { immer } from 'zustand/middleware/immer';
import type {
  DiffCommentsState,
  DiffCommentsSlice,
  DiffComment,
} from '@/lib/diff/types';

const STORAGE_KEY = 'kandev-diff-comments';

// Stable empty references to avoid creating new objects on every render
const EMPTY_COMMENTS: DiffComment[] = [];
const EMPTY_COMMENTS_BY_FILE: Record<string, DiffComment[]> = {};

const defaultState: DiffCommentsState = {
  bySession: {},
  pendingForChat: [],
  editingCommentId: null,
};

/** Find and update a comment by ID across all sessions/files. */
function findAndUpdateComment(
  bySession: DiffCommentsState['bySession'],
  commentId: string,
  updates: Partial<DiffComment>,
): void {
  for (const sessionId of Object.keys(bySession)) {
    for (const filePath of Object.keys(bySession[sessionId])) {
      const comments = bySession[sessionId][filePath];
      const index = comments.findIndex((c) => c.id === commentId);
      if (index !== -1) {
        bySession[sessionId][filePath][index] = { ...comments[index], ...updates };
        return;
      }
    }
  }
}

/** Remove comments by ID set, cleaning up empty file/session entries. */
function removeCommentsByIds(
  bySession: DiffCommentsState['bySession'],
  idsToRemove: Set<string>,
): void {
  for (const sessionId of Object.keys(bySession)) {
    for (const filePath of Object.keys(bySession[sessionId])) {
      bySession[sessionId][filePath] = bySession[sessionId][filePath]
        .filter((comment) => !idsToRemove.has(comment.id));
      if (bySession[sessionId][filePath].length === 0) {
        delete bySession[sessionId][filePath];
      }
    }
    if (Object.keys(bySession[sessionId]).length === 0) {
      delete bySession[sessionId];
    }
  }
}

/** Collect all pending DiffComment objects matching the pendingForChat IDs. */
function collectPendingComments(
  bySession: DiffCommentsState['bySession'],
  pendingForChat: string[],
): DiffComment[] {
  const pending: DiffComment[] = [];
  for (const sessionId of Object.keys(bySession)) {
    for (const filePath of Object.keys(bySession[sessionId])) {
      for (const comment of bySession[sessionId][filePath]) {
        if (pendingForChat.includes(comment.id)) {
          pending.push(comment);
        }
      }
    }
  }
  return pending;
}

/**
 * Standalone Zustand store for diff comments with localStorage persistence.
 * Comments are organized by sessionId and filePath.
 */
export const useDiffCommentsStore = create<DiffCommentsSlice>()(
  persist(
    immer<DiffCommentsSlice>((set, get) => ({
      ...defaultState,

      addComment: (comment: DiffComment) =>
        set((state) => {
          const { sessionId, filePath } = comment;
          if (!state.bySession[sessionId]) {
            state.bySession[sessionId] = {};
          }
          if (!state.bySession[sessionId][filePath]) {
            state.bySession[sessionId][filePath] = [];
          }
          state.bySession[sessionId][filePath].push(comment);

          // Auto-add to pending for chat
          if (comment.status === 'pending') {
            state.pendingForChat.push(comment.id);
          }
        }),

      updateComment: (commentId: string, updates: Partial<DiffComment>) =>
        set((state) => {
          findAndUpdateComment(state.bySession, commentId, updates);
        }),

      removeComment: (sessionId: string, filePath: string, commentId: string) =>
        set((state) => {
          if (!state.bySession[sessionId]?.[filePath]) return;
          state.bySession[sessionId][filePath] = state.bySession[sessionId][
            filePath
          ].filter((c) => c.id !== commentId);

          // Remove from pending if present
          state.pendingForChat = state.pendingForChat.filter(
            (id) => id !== commentId
          );

          // Clear editing if this was the editing comment
          if (state.editingCommentId === commentId) {
            state.editingCommentId = null;
          }
        }),

      addToPending: (commentId: string) =>
        set((state) => {
          if (!state.pendingForChat.includes(commentId)) {
            state.pendingForChat.push(commentId);
          }
        }),

      removeFromPending: (commentId: string) =>
        set((state) => {
          state.pendingForChat = state.pendingForChat.filter(
            (id) => id !== commentId
          );
        }),

      clearPending: () =>
        set((state) => {
          state.pendingForChat = [];
        }),

      setEditingComment: (commentId: string | null) =>
        set((state) => {
          state.editingCommentId = commentId;
        }),

      markCommentsSent: (commentIds: string[]) =>
        set((state) => {
          const idsToRemove = new Set(commentIds);
          removeCommentsByIds(state.bySession, idsToRemove);
          state.pendingForChat = state.pendingForChat.filter(
            (id) => !idsToRemove.has(id)
          );
          if (state.editingCommentId && idsToRemove.has(state.editingCommentId)) {
            state.editingCommentId = null;
          }
        }),

      getCommentsForFile: (sessionId: string, filePath: string): DiffComment[] => {
        const state = get();
        return state.bySession[sessionId]?.[filePath] ?? EMPTY_COMMENTS;
      },

      getPendingComments: (): DiffComment[] => {
        const state = get();
        return collectPendingComments(state.bySession, state.pendingForChat);
      },

      clearSessionComments: (sessionId: string) =>
        set((state) => {
          // Collect comment IDs BEFORE deleting the session
          const sessionCommentIds = new Set<string>();
          if (state.bySession[sessionId]) {
            for (const filePath of Object.keys(state.bySession[sessionId])) {
              for (const comment of state.bySession[sessionId][filePath]) {
                sessionCommentIds.add(comment.id);
              }
            }
          }
          // Now delete the session and filter pending
          delete state.bySession[sessionId];
          state.pendingForChat = state.pendingForChat.filter(
            (id) => !sessionCommentIds.has(id)
          );
        }),
    })),
    {
      name: STORAGE_KEY,
      storage: createJSONStorage(() => localStorage),
      partialize: (state) => ({
        bySession: state.bySession,
        // Don't persist pendingForChat or editingCommentId
      }),
    }
  )
);

/**
 * Hook to get comments for a specific file.
 * Returns stable reference from store (or stable empty array).
 */
export function useFileComments(sessionId: string, filePath: string) {
  const fileComments = useDiffCommentsStore((state) =>
    state.bySession[sessionId]?.[filePath]
  );
  return fileComments ?? EMPTY_COMMENTS;
}

/**
 * Hook to get pending comments for chat.
 * Uses useMemo with stable store subscriptions to prevent infinite re-renders.
 */
export function usePendingComments() {
  const bySession = useDiffCommentsStore((state) => state.bySession);
  const pendingForChat = useDiffCommentsStore((state) => state.pendingForChat);

  return useMemo(() => {
    if (pendingForChat.length === 0) return EMPTY_COMMENTS;
    const pending: DiffComment[] = [];
    for (const sessionId of Object.keys(bySession)) {
      for (const filePath of Object.keys(bySession[sessionId])) {
        for (const comment of bySession[sessionId][filePath]) {
          if (pendingForChat.includes(comment.id)) {
            pending.push(comment);
          }
        }
      }
    }
    return pending;
  }, [bySession, pendingForChat]);
}

function addPendingCommentToFileMap(
  byFile: Record<string, DiffComment[]>,
  comment: DiffComment,
  pendingForChat: string[],
): void {
  if (!pendingForChat.includes(comment.id)) return;
  if (!byFile[comment.filePath]) byFile[comment.filePath] = [];
  byFile[comment.filePath].push(comment);
}

/**
 * Hook to get pending comments grouped by file.
 * Uses useMemo with stable store subscriptions to prevent infinite re-renders.
 */
export function usePendingCommentsByFile() {
  const bySession = useDiffCommentsStore((state) => state.bySession);
  const pendingForChat = useDiffCommentsStore((state) => state.pendingForChat);

  return useMemo(() => {
    if (pendingForChat.length === 0) return EMPTY_COMMENTS_BY_FILE;
    const byFile: Record<string, DiffComment[]> = {};
    for (const sessionId of Object.keys(bySession)) {
      for (const filePath of Object.keys(bySession[sessionId])) {
        for (const comment of bySession[sessionId][filePath]) {
          addPendingCommentToFileMap(byFile, comment, pendingForChat);
        }
      }
    }
    return byFile;
  }, [bySession, pendingForChat]);
}
