import type { StateCreator } from "zustand";
import type { TaskSession } from "@/lib/types/http";
import type { SessionSlice, SessionSliceState } from "./types";
import { migrateEnvKeyedData } from "@/lib/state/slices/session-runtime/session-runtime-slice";

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

/** Eagerly populate session→environment mapping and migrate any data stored under the fallback key.
 *  `draft` must be the combined store state (SessionSlice + SessionRuntimeSlice). */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function syncEnvironmentMapping(draft: any, sessionId: string, environmentId: string | undefined) {
  if (!environmentId) return;
  draft.environmentIdBySessionId[sessionId] = environmentId;
  migrateEnvKeyedData(draft, sessionId, environmentId);
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
  taskPlans: {
    byTaskId: {},
    loadingByTaskId: {},
    loadedByTaskId: {},
    savingByTaskId: {},
    revisionsByTaskId: {},
    revisionsLoadingByTaskId: {},
    revisionsLoadedByTaskId: {},
    revisionContentCache: {},
    previewRevisionIdByTaskId: {},
    comparePairByTaskId: {},
    lastSeenUpdatedAtByTaskId: {},
  },
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
        // revisionContentCache is keyed by revisionId, so pick the IDs for this
        // task before deleting the revisions list and drop their cache entries.
        const revs = draft.taskPlans.revisionsByTaskId[taskId];
        if (revs) {
          for (const r of revs) {
            delete draft.taskPlans.revisionContentCache[r.id];
          }
        }
        delete draft.taskPlans.byTaskId[taskId];
        delete draft.taskPlans.loadingByTaskId[taskId];
        delete draft.taskPlans.loadedByTaskId[taskId];
        delete draft.taskPlans.savingByTaskId[taskId];
        delete draft.taskPlans.revisionsByTaskId[taskId];
        delete draft.taskPlans.revisionsLoadingByTaskId[taskId];
        delete draft.taskPlans.revisionsLoadedByTaskId[taskId];
        delete draft.taskPlans.previewRevisionIdByTaskId[taskId];
        delete draft.taskPlans.comparePairByTaskId[taskId];
        delete draft.taskPlans.lastSeenUpdatedAtByTaskId[taskId];
      }),
    markTaskPlanSeen: (taskId: string) =>
      set((draft) => {
        const plan = draft.taskPlans.byTaskId[taskId];
        draft.taskPlans.lastSeenUpdatedAtByTaskId[taskId] = plan?.updated_at ?? "";
      }),
    setPlanRevisions: (
      taskId: string,
      revisions: Parameters<SessionSlice["setPlanRevisions"]>[1],
    ) =>
      set((draft) => {
        draft.taskPlans.revisionsByTaskId[taskId] = [...revisions].sort(
          (a, b) => b.revision_number - a.revision_number,
        );
        draft.taskPlans.revisionsLoadedByTaskId[taskId] = true;
        draft.taskPlans.revisionsLoadingByTaskId[taskId] = false;
      }),
    upsertPlanRevision: (
      taskId: string,
      revision: Parameters<SessionSlice["upsertPlanRevision"]>[1],
    ) =>
      set((draft) => {
        const list = draft.taskPlans.revisionsByTaskId[taskId] ?? [];
        const idx = list.findIndex((r) => r.id === revision.id);
        if (idx === -1) {
          list.unshift(revision);
        } else {
          list[idx] = { ...list[idx], ...revision };
          // Coalesced writes update an existing revision's content on the
          // backend, but the WS payload carries metadata only — drop any
          // cached content so the next preview refetches.
          delete draft.taskPlans.revisionContentCache[revision.id];
        }
        list.sort((a, b) => b.revision_number - a.revision_number);
        draft.taskPlans.revisionsByTaskId[taskId] = list;
      }),
    setPlanRevisionsLoading: (taskId: string, loading: boolean) =>
      set((draft) => {
        draft.taskPlans.revisionsLoadingByTaskId[taskId] = loading;
      }),
    cachePlanRevisionContent: (revisionId: string, content: string) =>
      set((draft) => {
        draft.taskPlans.revisionContentCache[revisionId] = content;
      }),
    ...buildPreviewCompareActions(set),
  };
}

function buildPreviewCompareActions(set: ImmerSet) {
  return {
    setPreviewRevision: (taskId: string, revisionId: string | null) =>
      set((draft) => {
        if (revisionId === null) {
          delete draft.taskPlans.previewRevisionIdByTaskId[taskId];
        } else {
          draft.taskPlans.previewRevisionIdByTaskId[taskId] = revisionId;
        }
      }),
    toggleComparePair: (taskId: string, revisionId: string) =>
      set((draft) => {
        draft.taskPlans.comparePairByTaskId[taskId] = nextPair(
          draft.taskPlans.comparePairByTaskId[taskId] ?? [null, null],
          revisionId,
        );
      }),
    clearComparePair: (taskId: string) =>
      set((draft) => {
        delete draft.taskPlans.comparePairByTaskId[taskId];
      }),
  };
}

/** Compute the next compare-pair after a toggle. Already-selected ids unselect;
 * empty slots fill in order (slot 0 first); a full pair drops slot 0 and shifts
 * slot 1 → 0, putting the new pick in slot 1 (FIFO of length 2). */
function nextPair(
  current: readonly [string | null, string | null],
  revisionId: string,
): [string | null, string | null] {
  if (current[0] === revisionId) return [current[1], null];
  if (current[1] === revisionId) return [current[0], null];
  if (current[0] === null) return [revisionId, current[1]];
  if (current[1] === null) return [current[0], revisionId];
  return [current[1], revisionId];
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
      syncEnvironmentMapping(draft, session.id, mergedSession.task_environment_id);
    }),
  removeTaskSession: (taskId, sessionId) =>
    set((draft) => {
      delete draft.taskSessions.items[sessionId];
      const sessionsByTask = draft.taskSessionsByTask.itemsByTaskId[taskId];
      if (sessionsByTask) {
        draft.taskSessionsByTask.itemsByTaskId[taskId] = sessionsByTask.filter(
          (s) => s.id !== sessionId,
        );
      }
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      delete (draft as any).environmentIdBySessionId[sessionId];
    }),
  setTaskSessionsForTask: (taskId, sessions) =>
    set((draft) => {
      draft.taskSessionsByTask.itemsByTaskId[taskId] = sessions;
      draft.taskSessionsByTask.loadingByTaskId[taskId] = false;
      draft.taskSessionsByTask.loadedByTaskId[taskId] = true;
      for (const session of sessions) {
        draft.taskSessions.items[session.id] = session;
        syncEnvironmentMapping(draft, session.id, session.task_environment_id);
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
