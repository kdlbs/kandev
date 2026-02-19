import type { StateCreator } from "zustand";
import type { TaskSession } from "@/lib/types/http";
import type { SessionSlice, SessionSliceState } from "./types";

/** Ensure message metadata exists for a session, initializing with defaults if needed. */
function ensureMessageMeta(
  metaBySession: SessionSliceState["messages"]["metaBySession"],
  sessionId: string,
) {
  if (!metaBySession[sessionId]) {
    metaBySession[sessionId] = { isLoading: false, hasMore: false, oldestCursor: null };
  }
}

/** Apply partial metadata updates to the session's message metadata. */
function applyMessageMeta(
  metaBySession: SessionSliceState["messages"]["metaBySession"],
  sessionId: string,
  meta: { hasMore?: boolean; oldestCursor?: string | null; isLoading?: boolean },
) {
  ensureMessageMeta(metaBySession, sessionId);
  if (meta.hasMore !== undefined) metaBySession[sessionId].hasMore = meta.hasMore;
  if (meta.isLoading !== undefined) metaBySession[sessionId].isLoading = meta.isLoading;
  if (meta.oldestCursor !== undefined) metaBySession[sessionId].oldestCursor = meta.oldestCursor;
}

/**
 * Merge message fields: only overwrite existing fields with non-undefined incoming values.
 * This handles duplicate events from multiple sources.
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function mergeMessageFields(target: Record<string, unknown>, source: Record<string, any>) {
  for (const key of Object.keys(source)) {
    if (source[key] !== undefined) {
      target[key] = source[key];
    }
  }
}

/** Merge an incoming session update with an existing session, preserving nullable fields. */
function mergeTaskSession(existing: TaskSession, incoming: TaskSession): TaskSession {
  return {
    ...existing,
    ...incoming,
    agent_profile_snapshot: incoming.agent_profile_snapshot ?? existing.agent_profile_snapshot,
    worktree_id: incoming.worktree_id ?? existing.worktree_id,
    worktree_path: incoming.worktree_path ?? existing.worktree_path,
    worktree_branch: incoming.worktree_branch ?? existing.worktree_branch,
    repository_id: incoming.repository_id ?? existing.repository_id,
    base_branch: incoming.base_branch ?? existing.base_branch,
  };
}

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

type ImmerSet = Parameters<typeof createSessionSlice>[0];

function buildMessageActions(set: ImmerSet) {
  return {
    setMessages: (
      sessionId: string,
      messages: Parameters<SessionSlice["setMessages"]>[1],
      meta?: Parameters<SessionSlice["setMessages"]>[2],
    ) =>
      set((draft) => {
        draft.messages.bySession[sessionId] = messages;
        ensureMessageMeta(draft.messages.metaBySession, sessionId);
        if (meta) applyMessageMeta(draft.messages.metaBySession, sessionId, meta);
      }),
    addMessage: (message: Parameters<SessionSlice["addMessage"]>[0]) =>
      set((draft) => {
        const sessionId = message.session_id;
        if (!draft.messages.bySession[sessionId]) draft.messages.bySession[sessionId] = [];
        const existingIndex = draft.messages.bySession[sessionId].findIndex(
          (m) => m.id === message.id,
        );
        if (existingIndex === -1) {
          draft.messages.bySession[sessionId].push(message);
        } else {
          mergeMessageFields(
            draft.messages.bySession[sessionId][existingIndex] as unknown as Record<
              string,
              unknown
            >,
            message as unknown as Record<string, unknown>,
          );
        }
      }),
    updateMessage: (message: Parameters<SessionSlice["updateMessage"]>[0]) =>
      set((draft) => {
        const messages = draft.messages.bySession[message.session_id];
        if (!messages) return;
        const index = messages.findIndex((m) => m.id === message.id);
        if (index === -1) return;
        const merged = { ...messages[index] };
        mergeMessageFields(
          merged as unknown as Record<string, unknown>,
          message as unknown as Record<string, unknown>,
        );
        messages[index] = merged;
      }),
    prependMessages: (
      sessionId: string,
      messages: Parameters<SessionSlice["prependMessages"]>[1],
      meta?: Parameters<SessionSlice["prependMessages"]>[2],
    ) =>
      set((draft) => {
        const existing = draft.messages.bySession[sessionId] || [];
        const existingIds = new Set(existing.map((m) => m.id));
        draft.messages.bySession[sessionId] = [
          ...messages.filter((m) => !existingIds.has(m.id)),
          ...existing,
        ];
        ensureMessageMeta(draft.messages.metaBySession, sessionId);
        draft.messages.metaBySession[sessionId].isLoading = false;
        if (meta) applyMessageMeta(draft.messages.metaBySession, sessionId, meta);
      }),
    setMessagesMetadata: (
      sessionId: string,
      meta: Parameters<SessionSlice["setMessagesMetadata"]>[1],
    ) =>
      set((draft) => {
        applyMessageMeta(draft.messages.metaBySession, sessionId, meta);
      }),
    setMessagesLoading: (sessionId: string, loading: boolean) =>
      set((draft) => {
        applyMessageMeta(draft.messages.metaBySession, sessionId, { isLoading: loading });
      }),
  };
}

function buildTaskPlanActions(set: ImmerSet) {
  return {
    setTaskPlan: (taskId: string, plan: Parameters<SessionSlice["setTaskPlan"]>[1]) =>
      set((draft) => {
        draft.taskPlans.byTaskId[taskId] = plan;
        draft.taskPlans.loadingByTaskId[taskId] = false;
        draft.taskPlans.loadedByTaskId[taskId] = true;
      }),
    setTaskPlanLoading: (taskId: string, loading: boolean) =>
      set((draft) => {
        draft.taskPlans.loadingByTaskId[taskId] = loading;
      }),
    setTaskPlanSaving: (taskId: string, saving: boolean) =>
      set((draft) => {
        draft.taskPlans.savingByTaskId[taskId] = saving;
      }),
    clearTaskPlan: (taskId: string) =>
      set((draft) => {
        delete draft.taskPlans.byTaskId[taskId];
        delete draft.taskPlans.loadingByTaskId[taskId];
        delete draft.taskPlans.loadedByTaskId[taskId];
        delete draft.taskPlans.savingByTaskId[taskId];
      }),
  };
}

export const createSessionSlice: StateCreator<
  SessionSlice,
  [["zustand/immer", never]],
  [],
  SessionSlice
> = (set) => ({
  ...defaultSessionState,
  ...buildMessageActions(set),
  addTurn: (turn) =>
    set((draft) => {
      const sessionId = turn.session_id;
      if (!draft.turns.bySession[sessionId]) draft.turns.bySession[sessionId] = [];
      if (!draft.turns.bySession[sessionId].find((t) => t.id === turn.id)) {
        draft.turns.bySession[sessionId].push(turn);
      }
    }),
  completeTurn: (sessionId, turnId, completedAt) =>
    set((draft) => {
      const turn = draft.turns.bySession[sessionId]?.find((t) => t.id === turnId);
      if (turn) turn.completed_at = completedAt;
    }),
  setActiveTurn: (sessionId, turnId) =>
    set((draft) => {
      draft.turns.activeBySession[sessionId] = turnId;
    }),
  setTaskSession: (session) =>
    set((draft) => {
      const existingSession = draft.taskSessions.items[session.id];
      const mergedSession = existingSession ? mergeTaskSession(existingSession, session) : session;
      draft.taskSessions.items[session.id] = mergedSession;
      const sessionsByTask = draft.taskSessionsByTask.itemsByTaskId[session.task_id];
      if (sessionsByTask) {
        const sessionIndex = sessionsByTask.findIndex((s) => s.id === session.id);
        if (sessionIndex >= 0) sessionsByTask[sessionIndex] = mergedSession;
      }
    }),
  setTaskSessionsForTask: (taskId, sessions) =>
    set((draft) => {
      draft.taskSessionsByTask.itemsByTaskId[taskId] = sessions;
      draft.taskSessionsByTask.loadingByTaskId[taskId] = false;
      draft.taskSessionsByTask.loadedByTaskId[taskId] = true;
      for (const session of sessions) draft.taskSessions.items[session.id] = session;
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
  ...buildTaskPlanActions(set),
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
