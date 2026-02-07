import type { StateCreator } from 'zustand';
import type { SessionSlice, SessionSliceState } from './types';

export const defaultSessionState: SessionSliceState = {
  messages: { bySession: {}, metaBySession: {} },
  turns: {
    bySession: {},
    activeBySession: {},
  },
  taskSessions: { items: {} },
  taskSessionsByTask: { itemsByTaskId: {}, loadingByTaskId: {}, loadedByTaskId: {} },
  sessionAgentctl: { itemsBySessionId: {} },
  worktrees: { items: {} },
  sessionWorktreesBySessionId: { itemsBySessionId: {} },
  pendingModel: { bySessionId: {} },
  activeModel: { bySessionId: {} },
  taskPlans: { byTaskId: {}, loadingByTaskId: {}, loadedByTaskId: {}, savingByTaskId: {} },
  queue: { bySessionId: {}, isLoading: {} },
};

export const createSessionSlice: StateCreator<
  SessionSlice,
  [['zustand/immer', never]],
  [],
  SessionSlice
> = (set) => ({
  ...defaultSessionState,
  setMessages: (sessionId, messages, meta) =>
    set((draft) => {
      draft.messages.bySession[sessionId] = messages;
      if (!draft.messages.metaBySession[sessionId]) {
        draft.messages.metaBySession[sessionId] = {
          isLoading: false,
          hasMore: false,
          oldestCursor: null,
        };
      }
      if (meta?.hasMore !== undefined) {
        draft.messages.metaBySession[sessionId].hasMore = meta.hasMore;
      }
      if (meta?.oldestCursor !== undefined) {
        draft.messages.metaBySession[sessionId].oldestCursor = meta.oldestCursor;
      }
    }),
  addMessage: (message) =>
    set((draft) => {
      const sessionId = message.session_id;
      if (!draft.messages.bySession[sessionId]) {
        draft.messages.bySession[sessionId] = [];
      }
      const existingIndex = draft.messages.bySession[sessionId].findIndex((m) => m.id === message.id);
      if (existingIndex === -1) {
        // New message - add it
        draft.messages.bySession[sessionId].push(message);
      } else {
        // Message exists - merge, but don't overwrite defined values with undefined
        // This handles duplicate events from multiple sources
        const existing = draft.messages.bySession[sessionId][existingIndex];
        for (const key of Object.keys(message) as Array<keyof typeof message>) {
          if (message[key] !== undefined) {
            (existing as Record<string, unknown>)[key] = message[key];
          }
        }
      }
    }),
  updateMessage: (message) =>
    set((draft) => {
      const sessionId = message.session_id;
      const messages = draft.messages.bySession[sessionId];
      if (messages) {
        const index = messages.findIndex((m) => m.id === message.id);
        if (index !== -1) {
          // Merge update with existing message, but don't overwrite defined values with undefined
          // This handles the case where some event sources don't include turn_id
          const existing = messages[index];
          const merged = { ...existing };
          for (const key of Object.keys(message) as Array<keyof typeof message>) {
            if (message[key] !== undefined) {
              (merged as Record<string, unknown>)[key] = message[key];
            }
          }
          messages[index] = merged;
        }
      }
    }),
  prependMessages: (sessionId, messages, meta) =>
    set((draft) => {
      const existing = draft.messages.bySession[sessionId] || [];
      // Deduplicate - only add messages that don't already exist
      const existingIds = new Set(existing.map((m) => m.id));
      const newMessages = messages.filter((m) => !existingIds.has(m.id));
      draft.messages.bySession[sessionId] = [...newMessages, ...existing];
      if (!draft.messages.metaBySession[sessionId]) {
        draft.messages.metaBySession[sessionId] = {
          isLoading: false,
          hasMore: false,
          oldestCursor: null,
        };
      }
      // Always reset isLoading to false after prepending messages
      draft.messages.metaBySession[sessionId].isLoading = false;
      if (meta?.hasMore !== undefined) {
        draft.messages.metaBySession[sessionId].hasMore = meta.hasMore;
      }
      if (meta?.oldestCursor !== undefined) {
        draft.messages.metaBySession[sessionId].oldestCursor = meta.oldestCursor;
      }
    }),
  setMessagesMetadata: (sessionId, meta) =>
    set((draft) => {
      if (!draft.messages.metaBySession[sessionId]) {
        draft.messages.metaBySession[sessionId] = {
          isLoading: false,
          hasMore: false,
          oldestCursor: null,
        };
      }
      if (meta.hasMore !== undefined) {
        draft.messages.metaBySession[sessionId].hasMore = meta.hasMore;
      }
      if (meta.isLoading !== undefined) {
        draft.messages.metaBySession[sessionId].isLoading = meta.isLoading;
      }
      if (meta.oldestCursor !== undefined) {
        draft.messages.metaBySession[sessionId].oldestCursor = meta.oldestCursor;
      }
    }),
  setMessagesLoading: (sessionId, loading) =>
    set((draft) => {
      if (!draft.messages.metaBySession[sessionId]) {
        draft.messages.metaBySession[sessionId] = {
          isLoading: loading,
          hasMore: false,
          oldestCursor: null,
        };
      } else {
        draft.messages.metaBySession[sessionId].isLoading = loading;
      }
    }),
  addTurn: (turn) =>
    set((draft) => {
      const sessionId = turn.session_id;
      if (!draft.turns.bySession[sessionId]) {
        draft.turns.bySession[sessionId] = [];
      }
      const existing = draft.turns.bySession[sessionId].find((t) => t.id === turn.id);
      if (!existing) {
        draft.turns.bySession[sessionId].push(turn);
      }
    }),
  completeTurn: (sessionId, turnId, completedAt) =>
    set((draft) => {
      const turns = draft.turns.bySession[sessionId];
      if (turns) {
        const turn = turns.find((t) => t.id === turnId);
        if (turn) {
          turn.completed_at = completedAt;
        }
      }
    }),
  setActiveTurn: (sessionId, turnId) =>
    set((draft) => {
      draft.turns.activeBySession[sessionId] = turnId;
    }),
  setTaskSession: (session) =>
    set((draft) => {
      // Merge with existing session data to preserve fields like agent_profile_id, worktree info, etc.
      const existingSession = draft.taskSessions.items[session.id];
      const mergedSession = existingSession
        ? {
            ...existingSession,
            ...session,
            // Preserve fields that may not be included in partial updates from WebSocket events
            agent_profile_snapshot: session.agent_profile_snapshot ?? existingSession.agent_profile_snapshot,
            worktree_id: session.worktree_id ?? existingSession.worktree_id,
            worktree_path: session.worktree_path ?? existingSession.worktree_path,
            worktree_branch: session.worktree_branch ?? existingSession.worktree_branch,
            repository_id: session.repository_id ?? existingSession.repository_id,
            base_branch: session.base_branch ?? existingSession.base_branch,
          }
        : session;

      draft.taskSessions.items[session.id] = mergedSession;

      // Also update taskSessionsByTask to keep both stores in sync
      const taskId = session.task_id;
      const sessionsByTask = draft.taskSessionsByTask.itemsByTaskId[taskId];
      if (sessionsByTask) {
        const sessionIndex = sessionsByTask.findIndex((s) => s.id === session.id);
        if (sessionIndex >= 0) {
          sessionsByTask[sessionIndex] = mergedSession;
        }
      }
    }),
  setTaskSessionsForTask: (taskId, sessions) =>
    set((draft) => {
      // Update taskSessionsByTask with the new sessions list
      draft.taskSessionsByTask.itemsByTaskId[taskId] = sessions;
      draft.taskSessionsByTask.loadingByTaskId[taskId] = false;
      draft.taskSessionsByTask.loadedByTaskId[taskId] = true;

      // Also populate taskSessions.items for individual session lookups
      // When loading from API, we get complete session data, so we can replace entirely
      // This ensures all fields including review_status and workflow_step_id are properly set
      for (const session of sessions) {
        draft.taskSessions.items[session.id] = session;
      }
    }),
  setTaskSessionsLoading: (taskId, loading) =>
    set((draft) => {
      draft.taskSessionsByTask.loadingByTaskId[taskId] = loading;
    }),
  setSessionAgentctlStatus: (sessionId, status) =>
    set((draft) => {
      draft.sessionAgentctl.itemsBySessionId[sessionId] = status;
    }),
  setWorktree: (worktree) =>
    set((draft) => {
      draft.worktrees.items[worktree.id] = worktree;
    }),
  setSessionWorktrees: (sessionId, worktreeIds) =>
    set((draft) => {
      draft.sessionWorktreesBySessionId.itemsBySessionId[sessionId] = worktreeIds;
    }),
  setPendingModel: (sessionId, modelId) =>
    set((draft) => {
      draft.pendingModel.bySessionId[sessionId] = modelId;
    }),
  clearPendingModel: (sessionId) =>
    set((draft) => {
      delete draft.pendingModel.bySessionId[sessionId];
    }),
  setActiveModel: (sessionId, modelId) =>
    set((draft) => {
      draft.activeModel.bySessionId[sessionId] = modelId;
    }),
  setTaskPlan: (taskId, plan) =>
    set((draft) => {
      draft.taskPlans.byTaskId[taskId] = plan;
      draft.taskPlans.loadingByTaskId[taskId] = false;
      draft.taskPlans.loadedByTaskId[taskId] = true;
    }),
  setTaskPlanLoading: (taskId, loading) =>
    set((draft) => {
      draft.taskPlans.loadingByTaskId[taskId] = loading;
    }),
  setTaskPlanSaving: (taskId, saving) =>
    set((draft) => {
      draft.taskPlans.savingByTaskId[taskId] = saving;
    }),
  clearTaskPlan: (taskId) =>
    set((draft) => {
      delete draft.taskPlans.byTaskId[taskId];
      delete draft.taskPlans.loadingByTaskId[taskId];
      delete draft.taskPlans.loadedByTaskId[taskId];
      delete draft.taskPlans.savingByTaskId[taskId];
    }),
  setQueueStatus: (sessionId, status) =>
    set((draft) => {
      draft.queue.bySessionId[sessionId] = status;
    }),
  setQueueLoading: (sessionId, loading) =>
    set((draft) => {
      draft.queue.isLoading[sessionId] = loading;
    }),
  clearQueueStatus: (sessionId) =>
    set((draft) => {
      delete draft.queue.bySessionId[sessionId];
      delete draft.queue.isLoading[sessionId];
    }),
});
