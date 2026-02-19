import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import type { Comment, DiffComment, CommentsSlice, CommentsState } from "./types";
import { isDiffComment } from "./types";
import {
  persistSessionComments,
  loadSessionComments,
  clearPersistedSessionComments,
} from "./persistence";

const defaultState: CommentsState = {
  byId: {},
  bySession: {},
  pendingForChat: [],
  editingCommentId: null,
};

/** Collect all comments for a given session from byId + bySession index. */
function getSessionComments(state: CommentsState, sessionId: string): Comment[] {
  const ids = state.bySession[sessionId];
  if (!ids || ids.length === 0) return [];
  const comments: Comment[] = [];
  for (const id of ids) {
    const comment = state.byId[id];
    if (comment) comments.push(comment);
  }
  return comments;
}

/** Persist all comments for a session to sessionStorage. */
function persistSession(state: CommentsState, sessionId: string): void {
  persistSessionComments(sessionId, getSessionComments(state, sessionId));
}

/** Remove a single comment from state and persist. */
function removeCommentFromState(state: CommentsState, commentId: string): void {
  const comment = state.byId[commentId];
  if (!comment) return;
  const { sessionId } = comment;
  delete state.byId[commentId];
  const sessionIds = state.bySession[sessionId];
  if (sessionIds) {
    const idx = sessionIds.indexOf(commentId);
    if (idx !== -1) sessionIds.splice(idx, 1);
    if (sessionIds.length === 0) delete state.bySession[sessionId];
  }
  state.pendingForChat = state.pendingForChat.filter((id) => id !== commentId);
  if (state.editingCommentId === commentId) state.editingCommentId = null;
  persistSession(state, sessionId);
}

/** Mark a batch of comments as sent: remove from store + persist affected sessions. */
function markSentInState(state: CommentsState, commentIds: string[]): void {
  const idsToRemove = new Set(commentIds);
  const affectedSessions = new Set<string>();
  for (const id of commentIds) {
    const comment = state.byId[id];
    if (comment) affectedSessions.add(comment.sessionId);
    delete state.byId[id];
  }
  for (const sessionId of Object.keys(state.bySession)) {
    state.bySession[sessionId] = state.bySession[sessionId].filter((id) => !idsToRemove.has(id));
    if (state.bySession[sessionId].length === 0) delete state.bySession[sessionId];
  }
  state.pendingForChat = state.pendingForChat.filter((id) => !idsToRemove.has(id));
  if (state.editingCommentId && idsToRemove.has(state.editingCommentId))
    state.editingCommentId = null;
  for (const sessionId of affectedSessions) persistSession(state, sessionId);
}

/** Remove all comments for a session from state + storage. */
function clearSessionInState(state: CommentsState, sessionId: string): void {
  const ids = state.bySession[sessionId];
  if (!ids) return;
  const idSet = new Set(ids);
  for (const id of ids) delete state.byId[id];
  delete state.bySession[sessionId];
  state.pendingForChat = state.pendingForChat.filter((id) => !idSet.has(id));
  clearPersistedSessionComments(sessionId);
}

/** Load comments from sessionStorage into state (no-op if already hydrated). */
function hydrateSessionInState(state: CommentsState, sessionId: string): void {
  if (state.bySession[sessionId] && state.bySession[sessionId].length > 0) return;
  const comments = loadSessionComments(sessionId);
  if (comments.length === 0) return;
  const ids: string[] = [];
  for (const comment of comments) {
    state.byId[comment.id] = comment;
    ids.push(comment.id);
    if (comment.status === "pending" && !state.pendingForChat.includes(comment.id)) {
      state.pendingForChat.push(comment.id);
    }
  }
  state.bySession[sessionId] = ids;
}

export const useCommentsStore = create<CommentsSlice>()(
  immer<CommentsSlice>((set, get) => ({
    ...defaultState,

    addComment: (comment: Comment) =>
      set((state) => {
        state.byId[comment.id] = comment;
        if (!state.bySession[comment.sessionId]) state.bySession[comment.sessionId] = [];
        state.bySession[comment.sessionId].push(comment.id);
        if (comment.status === "pending") state.pendingForChat.push(comment.id);
        persistSession(state, comment.sessionId);
      }),

    updateComment: (commentId: string, updates: Partial<Comment>) =>
      set((state) => {
        const existing = state.byId[commentId];
        if (!existing) return;
        Object.assign(existing, updates);
        persistSession(state, existing.sessionId);
      }),

    removeComment: (commentId: string) => set((state) => removeCommentFromState(state, commentId)),

    addToPending: (commentId: string) =>
      set((state) => {
        if (!state.pendingForChat.includes(commentId)) state.pendingForChat.push(commentId);
      }),

    removeFromPending: (commentId: string) =>
      set((state) => {
        state.pendingForChat = state.pendingForChat.filter((id) => id !== commentId);
      }),

    clearPending: () =>
      set((state) => {
        state.pendingForChat = [];
      }),

    setEditingComment: (commentId: string | null) =>
      set((state) => {
        state.editingCommentId = commentId;
      }),

    markCommentsSent: (commentIds: string[]) => set((state) => markSentInState(state, commentIds)),

    clearSessionComments: (sessionId: string) =>
      set((state) => clearSessionInState(state, sessionId)),

    hydrateSession: (sessionId: string) => set((state) => hydrateSessionInState(state, sessionId)),

    getCommentsForFile: (sessionId: string, filePath: string): DiffComment[] => {
      const state = get();
      const ids = state.bySession[sessionId];
      if (!ids) return [];
      const result: DiffComment[] = [];
      for (const id of ids) {
        const comment = state.byId[id];
        if (comment && isDiffComment(comment) && comment.filePath === filePath)
          result.push(comment);
      }
      return result;
    },

    getPendingComments: (): Comment[] => {
      const state = get();
      const pending: Comment[] = [];
      for (const id of state.pendingForChat) {
        const comment = state.byId[id];
        if (comment) pending.push(comment);
      }
      return pending;
    },
  })),
);
