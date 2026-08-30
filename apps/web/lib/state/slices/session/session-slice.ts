/* eslint-disable max-lines -- session state intentionally keeps its coordinated actions together. */
import type { StateCreator } from "zustand";
import { original } from "immer";
import type { Message, TaskSession } from "@/lib/types/http";
import type { SessionSlice, SessionSliceState } from "./types";
import { buildTurnActions, isSettledSessionState, parseTurnTimestamp } from "./turn-actions";
import {
  buildTaskSessionProjectionActions,
  mergePendingActionProjection,
} from "./task-session-projection-actions";
import { reconcileMessages } from "./message-signature";
import {
  migrateEnvKeyedData,
  purgeSessionRuntimeState,
} from "@/lib/state/slices/session-runtime/session-runtime-slice";
import type { SessionRuntimeSliceState } from "@/lib/state/slices/session-runtime/types";
import { prepareResultToSessionState } from "@/lib/state/slices/session-runtime/prepare-result";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import { getPlanLastSeen, setPlanLastSeen } from "@/lib/local-storage";
import {
  getWalkthroughLastSeen,
  setWalkthroughLastSeen,
} from "@/lib/walkthrough-notification-storage";

const debugEnv = createDebugLogger("session:env-mapping");

/** Ensure message metadata exists for a session, initializing with defaults if needed. */
function ensureMessageMeta(
  metaBySession: SessionSliceState["messages"]["metaBySession"],
  sessionId: string,
) {
  if (!metaBySession[sessionId]) {
    metaBySession[sessionId] = {
      isLoading: false,
      isLoadingMore: false,
      historyInitialized: false,
      hasMore: false,
      oldestCursor: null,
    };
  }
}

/** Apply partial metadata updates to the session's message metadata. */
function applyMessageMeta(
  metaBySession: SessionSliceState["messages"]["metaBySession"],
  sessionId: string,
  meta: {
    historyInitialized?: boolean;
    hasMore?: boolean;
    oldestCursor?: string | null;
    isLoading?: boolean;
    isLoadingMore?: boolean;
  },
) {
  ensureMessageMeta(metaBySession, sessionId);
  if (meta.historyInitialized !== undefined) {
    metaBySession[sessionId].historyInitialized = meta.historyInitialized;
  }
  if (meta.hasMore !== undefined) metaBySession[sessionId].hasMore = meta.hasMore;
  if (meta.isLoading !== undefined) metaBySession[sessionId].isLoading = meta.isLoading;
  if (meta.isLoadingMore !== undefined) metaBySession[sessionId].isLoadingMore = meta.isLoadingMore;
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

function mergeMessageAtIndex(messages: Message[], message: Message): void {
  const index = messages.findIndex((candidate) => candidate.id === message.id);
  if (index === -1) return;
  const merged = { ...messages[index] };
  mergeMessageFields(
    merged as unknown as Record<string, unknown>,
    message as unknown as Record<string, unknown>,
  );
  messages[index] = merged;
}

/** Return a new messages array with the message matching `messageId` removed. */
function removeMessageByID(messages: Message[], messageId: string) {
  return messages.filter((message) => message.id !== messageId);
}

/** Eagerly populate session→environment mapping and migrate any data stored under the fallback key.
 *  `draft` must be the combined store state (SessionSlice + SessionRuntimeSlice). */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function syncEnvironmentMapping(draft: any, sessionId: string, environmentId: string | undefined) {
  if (!environmentId) return;
  const previous = draft.environmentIdBySessionId[sessionId];
  if (isDebug()) {
    debugEnv("syncEnvironmentMapping", {
      sessionId,
      environmentId,
      previous: previous ?? null,
      changed: previous !== environmentId,
      fallbackGitStatusFileCount: Object.keys(
        draft.gitStatus?.byEnvironmentId?.[sessionId]?.files ?? {},
      ).length,
      targetGitStatusFileCount: Object.keys(
        draft.gitStatus?.byEnvironmentId?.[environmentId]?.files ?? {},
      ).length,
    });
  }
  draft.environmentIdBySessionId[sessionId] = environmentId;
  migrateEnvKeyedData(draft, sessionId, environmentId);
}

/**
 * Backfill the prepare-progress slice from a session's `metadata.prepare_result`
 * when sessions are loaded from the API (e.g. switching tasks client-side).
 *
 * Without this, prepare progress only ever arrives via SSR hydration or live WS
 * events, so switching to a task whose prepare already completed (common for
 * remote executors) showed an empty "Environment prepared" row until a full
 * page reload re-ran SSR. Only populates when no entry exists yet so we never
 * clobber live WS progress for an in-flight prepare.
 *
 * `draft` must be the combined store state (SessionSlice + SessionRuntimeSlice).
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function syncPrepareProgress(draft: any, session: TaskSession) {
  if (draft.prepareProgress.bySessionId[session.id]) return;
  const prepareState = prepareResultToSessionState(session.id, session.metadata);
  if (prepareState) draft.prepareProgress.bySessionId[session.id] = prepareState;
}

/** Merge the runtime cancellation projection using its process-local revision. */
function mergeCancellationProjection(
  existing: TaskSession,
  incoming: TaskSession,
): Pick<TaskSession, "cancellation_pending" | "cancellation_revision"> {
  const incomingRevision = incoming.cancellation_revision;
  const existingRevision = existing.cancellation_revision;
  const incomingIsCurrent =
    incomingRevision !== undefined &&
    (existingRevision === undefined || incomingRevision >= existingRevision);

  if (incomingIsCurrent) {
    return {
      cancellation_pending: incoming.cancellation_pending ?? existing.cancellation_pending,
      cancellation_revision: incomingRevision,
    };
  }

  if (incomingRevision === undefined && existingRevision === undefined) {
    return {
      cancellation_pending: incoming.cancellation_pending ?? existing.cancellation_pending,
      cancellation_revision: existingRevision,
    };
  }

  return {
    cancellation_pending: existing.cancellation_pending,
    cancellation_revision: existingRevision,
  };
}

/** Merge an incoming session update with an existing session, preserving nullable fields. */
function mergeTaskSession(existing: TaskSession, incoming: TaskSession): TaskSession {
  const cancellation = mergeCancellationProjection(existing, incoming);
  const incomingRouteGeneration = incoming.route_generation;
  const existingRouteGeneration = existing.route_generation;
  const routeIsStale =
    existingRouteGeneration !== undefined &&
    (incomingRouteGeneration === undefined || incomingRouteGeneration < existingRouteGeneration);
  const pendingAction = mergePendingActionProjection(existing, incoming);
  return {
    ...existing,
    ...incoming,
    ...cancellation,
    ...(routeIsStale
      ? {
          execution_profile_id: existing.execution_profile_id,
          route_generation: existing.route_generation,
          route_state: existing.route_state,
          route_reason: existing.route_reason,
          route_error_code: existing.route_error_code,
          route_error_class: existing.route_error_class,
          route_catalogue_version: existing.route_catalogue_version,
          route_retry_ordinal: existing.route_retry_ordinal,
          route_deadline: existing.route_deadline,
          route_pending_outcome: existing.route_pending_outcome,
          downstream_acp_session_id: existing.downstream_acp_session_id,
        }
      : {}),
    ...pendingAction,
    agent_profile_snapshot: incoming.agent_profile_snapshot ?? existing.agent_profile_snapshot,
    worktree_id: incoming.worktree_id ?? existing.worktree_id,
    worktree_path: incoming.worktree_path ?? existing.worktree_path,
    worktree_branch: incoming.worktree_branch ?? existing.worktree_branch,
    workspace_path: incoming.workspace_path ?? existing.workspace_path,
    repository_id: incoming.repository_id ?? existing.repository_id,
    base_branch: incoming.base_branch ?? existing.base_branch,
    task_environment_id: incoming.task_environment_id ?? existing.task_environment_id,
  };
}

// Settled states are defined once in turn-actions (SETTLED_SESSION_STATES /
// isSettledSessionState); this file must use the shared predicate so every
// settled-boundary path — hydration seeding, session updates, and the WS
// turn guard — stays on one definition.
/**
 * Retires stale active-turn markers and advances the settled boundary when a
 * session update reports a settled state: any turn started at/before the
 * boundary can never be active again (delayed WS start, stale hydration, or
 * a force-merged snapshot naming it are all rejected).
 */
function reconcileActiveTurnForIdleSession(draft: SessionSliceState, session: TaskSession): void {
  if (!isSettledSessionState(session.state)) return;
  const turns = draft.turns.bySession[session.id] ?? [];
  // Boundary-independent cleanup: drop a marker pointing at a
  // missing/completed turn.
  const activeTurnId = draft.turns.activeBySession[session.id];
  if (activeTurnId) {
    const activeTurn = turns.find((turn) => turn.id === activeTurnId);
    if (!activeTurn || activeTurn.completed_at) {
      draft.turns.activeBySession[session.id] = null;
    }
  }
  // Advance the settled boundary (monotonic). Every turn STARTED at/before
  // it can never be active again — covering missed-start turns and turns
  // unknown to this client, so delayed WS starts and stale hydrations cannot
  // resurrect them. An unparseable session timestamp leaves the boundary as
  // it was (the cleanup above already ran).
  const boundary = parseTurnTimestamp(session.updated_at);
  if (boundary === null) return;
  const currentBoundary = parseTurnTimestamp(draft.turns.settledBoundaryBySession[session.id]);
  const effective =
    currentBoundary === null || boundary > currentBoundary ? boundary : currentBoundary;
  if (effective === boundary) {
    draft.turns.settledBoundaryBySession[session.id] = session.updated_at;
  }
  // Clear the marker if it points at a turn started at/before the boundary.
  const activeId = draft.turns.activeBySession[session.id];
  if (!activeId) return;
  const active = turns.find((turn) => turn.id === activeId);
  if (!active || active.completed_at) return;
  const startedAt = parseTurnTimestamp(active.started_at);
  if (startedAt !== null && startedAt <= effective) {
    draft.turns.activeBySession[session.id] = null;
  }
}

export const defaultSessionState: SessionSliceState = {
  messages: { bySession: {}, metaBySession: {} },
  turns: {
    bySession: {},
    activeBySession: {},
    loadedBySession: {},
    reconcileEpochBySession: {},
    settledBoundaryBySession: {},
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
  walkthroughs: {
    byTaskId: {},
    activeStepByTaskId: {},
    lastSeenUpdatedAtByTaskId: {},
  },
  queue: { bySessionId: {}, metaBySessionId: {}, isLoading: {} },
};

type ImmerSet = Parameters<typeof createSessionSlice>[0];
type ImmerGet = () => SessionSlice;

/** Create the message store actions (set, add, update, remove, merge, prepend, metadata) backed by the given Immer setter. */
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
        mergeMessageAtIndex(messages, message);
      }),
    updateMessages: (messages: Parameters<SessionSlice["updateMessages"]>[0]) =>
      set((draft) => {
        for (const message of messages) {
          const sessionMessages = draft.messages.bySession[message.session_id];
          if (sessionMessages) mergeMessageAtIndex(sessionMessages, message);
        }
      }),
    removeMessage: (
      sessionId: Parameters<SessionSlice["removeMessage"]>[0],
      messageId: Parameters<SessionSlice["removeMessage"]>[1],
    ) =>
      set((draft) => {
        const messages = draft.messages.bySession[sessionId];
        if (!messages) return;
        draft.messages.bySession[sessionId] = removeMessageByID(messages, messageId);
      }),
    mergeMessages: (
      sessionId: string,
      messages: Parameters<SessionSlice["mergeMessages"]>[1],
      meta?: Parameters<SessionSlice["mergeMessages"]>[2],
    ) =>
      set((draft) => {
        const prevDraft = draft.messages.bySession[sessionId];
        const prev = (prevDraft ? (original(prevDraft) ?? prevDraft) : undefined) as
          | Message[]
          | undefined;
        const reconciled = reconcileMessages(prev, messages);
        // Only replace the array when identity actually changed, so a no-op
        // refetch preserves the array reference and triggers no re-render.
        if (reconciled !== prev) {
          draft.messages.bySession[sessionId] = reconciled;
        }
        ensureMessageMeta(draft.messages.metaBySession, sessionId);
        if (meta) applyMessageMeta(draft.messages.metaBySession, sessionId, meta);
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
        // isLoadingMore is owned by the shared pagination coordinator (raised
        // on the session's first in-flight request, cleared only when the
        // last one settles); a prepend must not clear it mid-flight.
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

/** Create the task-plan store actions (set, loading, saving, clear, seen, revisions, preview, compare) backed by the given Immer setter and getter. */
function buildTaskPlanActions(set: ImmerSet, get: ImmerGet) {
  return {
    setTaskPlan: (taskId: string, plan: Parameters<SessionSlice["setTaskPlan"]>[1]) => {
      const shouldHydrateLastSeen = get().taskPlans.lastSeenUpdatedAtByTaskId[taskId] === undefined;
      const storedLastSeen = shouldHydrateLastSeen ? getPlanLastSeen(taskId) : null;
      set((draft) => {
        draft.taskPlans.byTaskId[taskId] = plan;
        draft.taskPlans.loadingByTaskId[taskId] = false;
        draft.taskPlans.loadedByTaskId[taskId] = true;
        if (shouldHydrateLastSeen && storedLastSeen !== null) {
          draft.taskPlans.lastSeenUpdatedAtByTaskId[taskId] = storedLastSeen;
        }
      });
    },
    setTaskPlanLoading: (taskId: string, loading: boolean) =>
      set((draft) => {
        draft.taskPlans.loadingByTaskId[taskId] = loading;
      }),
    setTaskPlanSaving: (taskId: string, saving: boolean) =>
      set((draft) => {
        draft.taskPlans.savingByTaskId[taskId] = saving;
      }),
    clearTaskPlan: (taskId: string) => {
      setPlanLastSeen(taskId, null);
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
      });
    },
    markTaskPlanSeen: (taskId: string) => {
      const plan = get().taskPlans.byTaskId[taskId];
      const lastSeen = plan?.updated_at ?? "";
      setPlanLastSeen(taskId, lastSeen);
      set((draft) => {
        draft.taskPlans.lastSeenUpdatedAtByTaskId[taskId] = lastSeen;
      });
    },
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

/** Create the walkthrough store actions (set, active step, seen) backed by the given Immer setter and getter. */
function buildWalkthroughActions(set: ImmerSet, get: ImmerGet) {
  return {
    setWalkthrough: (
      taskId: string,
      walkthrough: Parameters<SessionSlice["setWalkthrough"]>[1],
    ) => {
      const shouldHydrateLastSeen =
        get().walkthroughs.lastSeenUpdatedAtByTaskId[taskId] === undefined;
      const storedLastSeen = shouldHydrateLastSeen ? getWalkthroughLastSeen(taskId) : null;
      set((draft) => {
        const previous = draft.walkthroughs.byTaskId[taskId];
        draft.walkthroughs.byTaskId[taskId] = walkthrough;
        // Clamp the active step into the new step range (defaults to 0). A
        // replaced/shorter tour must not leave the pointer past the last step.
        const steps = walkthrough?.steps.length ?? 0;
        const isReplacement = previous?.id !== walkthrough?.id;
        const current = isReplacement ? 0 : (draft.walkthroughs.activeStepByTaskId[taskId] ?? 0);
        draft.walkthroughs.activeStepByTaskId[taskId] =
          steps === 0 ? 0 : Math.min(current, steps - 1);
        if (shouldHydrateLastSeen && storedLastSeen !== null) {
          draft.walkthroughs.lastSeenUpdatedAtByTaskId[taskId] = storedLastSeen;
        }
      });
    },
    setWalkthroughActiveStep: (taskId: string, stepIndex: number) =>
      set((draft) => {
        const steps = draft.walkthroughs.byTaskId[taskId]?.steps.length ?? 0;
        const clamped = steps === 0 ? 0 : Math.max(0, Math.min(stepIndex, steps - 1));
        draft.walkthroughs.activeStepByTaskId[taskId] = clamped;
      }),
    markWalkthroughSeen: (taskId: string) => {
      const wt = get().walkthroughs.byTaskId[taskId];
      const lastSeen = wt?.updated_at ?? "";
      setWalkthroughLastSeen(taskId, lastSeen);
      set((draft) => {
        draft.walkthroughs.lastSeenUpdatedAtByTaskId[taskId] = lastSeen;
      });
    },
  };
}

/** Create the plan preview/compare actions (set preview revision, toggle and clear compare pair) backed by the given Immer setter. */
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

/** Create the task-session store actions (set, read cursor, remove, list, upsert from event, loading) backed by the given Immer setter. */
function buildTaskSessionActions(set: ImmerSet) {
  return {
    setTaskSession: (session: Parameters<SessionSlice["setTaskSession"]>[0]) =>
      set((draft) => {
        const existingSession = draft.taskSessions.items[session.id];
        const mergedSession = existingSession
          ? mergeTaskSession(existingSession, session)
          : session;
        draft.taskSessions.items[session.id] = mergedSession;
        const sessionsByTask = draft.taskSessionsByTask.itemsByTaskId[session.task_id];
        if (sessionsByTask) {
          const sessionIndex = sessionsByTask.findIndex((s) => s.id === session.id);
          if (sessionIndex >= 0) sessionsByTask[sessionIndex] = mergedSession;
        }
        syncEnvironmentMapping(draft, session.id, mergedSession.task_environment_id);
        reconcileActiveTurnForIdleSession(draft, mergedSession);
      }),
    /** Narrowly updates only the session's read cursor (last_read_message_id). */
    updateSessionReadCursor: (sessionId: string, lastReadMessageId: string) =>
      set((draft) => {
        const session = draft.taskSessions.items[sessionId];
        if (!session) return;
        session.last_read_message_id = lastReadMessageId;
        const sessionsByTask = draft.taskSessionsByTask.itemsByTaskId[session.task_id];
        if (sessionsByTask) {
          const match = sessionsByTask.find((s) => s.id === sessionId);
          if (match) match.last_read_message_id = lastReadMessageId;
        }
      }),
    /**
     * Removes a session and all its per-session state: task session rows,
     * messages, turns, reconciliation maps (loaded/epoch/boundary), and the
     * cascaded runtime buffers.
     */
    removeTaskSession: (taskId: string, sessionId: string) =>
      set((draft) => {
        delete draft.taskSessions.items[sessionId];
        const sessionsByTask = draft.taskSessionsByTask.itemsByTaskId[taskId];
        if (sessionsByTask) {
          draft.taskSessionsByTask.itemsByTaskId[taskId] = sessionsByTask.filter(
            (s) => s.id !== sessionId,
          );
        }
        // Drop the conversation history owned by this session.
        delete draft.messages.bySession[sessionId];
        delete draft.messages.metaBySession[sessionId];
        delete draft.turns.bySession[sessionId];
        delete draft.turns.activeBySession[sessionId];
        delete draft.turns.loadedBySession[sessionId];
        delete draft.turns.reconcileEpochBySession[sessionId];
        delete draft.turns.settledBoundaryBySession[sessionId];
        // Cascade into the runtime slice (shell/process/git buffers + per-session
        // maps); this also removes the environmentIdBySessionId mapping.
        purgeSessionRuntimeState(draft as unknown as SessionRuntimeSliceState, sessionId);
      }),
    setTaskSessionsForTask: (
      taskId: string,
      sessions: Parameters<SessionSlice["setTaskSessionsForTask"]>[1],
    ) =>
      set((draft) => {
        const merged = sessions.map((session) => {
          const existing = draft.taskSessions.items[session.id];
          return existing ? mergeTaskSession(existing, session) : session;
        });
        draft.taskSessionsByTask.itemsByTaskId[taskId] = merged;
        draft.taskSessionsByTask.loadingByTaskId[taskId] = false;
        draft.taskSessionsByTask.loadedByTaskId[taskId] = true;
        for (const session of merged) {
          draft.taskSessions.items[session.id] = session;
          syncEnvironmentMapping(draft, session.id, session.task_environment_id);
          syncPrepareProgress(draft, session);
          reconcileActiveTurnForIdleSession(draft, session);
        }
      }),
    upsertTaskSessionFromEvent: (
      taskId: string,
      session: Parameters<SessionSlice["upsertTaskSessionFromEvent"]>[1],
    ) =>
      set((draft) => {
        const existing = draft.taskSessions.items[session.id];
        if (!existing && draft.taskSessionsByTask.loadedByTaskId[taskId]) {
          // State events intentionally carry partial session rows. When one
          // introduces a new session, let useTaskSessions hydrate fields such
          // as repository_id instead of treating the old list as authoritative.
          draft.taskSessionsByTask.loadedByTaskId[taskId] = false;
        }
        const merged = existing ? mergeTaskSession(existing, session) : session;
        draft.taskSessions.items[session.id] = merged;
        const list = draft.taskSessionsByTask.itemsByTaskId[taskId];
        if (list) {
          const idx = list.findIndex((s) => s.id === session.id);
          if (idx >= 0) list[idx] = merged;
          else list.push(merged);
        } else {
          draft.taskSessionsByTask.itemsByTaskId[taskId] = [merged];
        }
        syncEnvironmentMapping(draft, session.id, merged.task_environment_id);
        reconcileActiveTurnForIdleSession(draft, merged);
      }),
    setTaskSessionsLoading: (taskId: string, loading: boolean) =>
      set((draft) => {
        draft.taskSessionsByTask.loadingByTaskId[taskId] = loading;
      }),
  };
}

/** Create the session slice, combining the default state with all session action builders wired to the store's Immer set/get. */
export const createSessionSlice: StateCreator<
  SessionSlice,
  [["zustand/immer", never]],
  [],
  SessionSlice
> = (set, get) => ({
  ...defaultSessionState,
  ...buildMessageActions(set),
  ...buildTurnActions(set),
  ...buildTaskSessionActions(set),
  ...buildTaskSessionProjectionActions(set),
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
  ...buildTaskPlanActions(set, get),
  ...buildWalkthroughActions(set, get),
  setQueueEntries: (sessionId, entries, meta) =>
    set((draft) => {
      draft.queue.bySessionId[sessionId] = entries;
      draft.queue.metaBySessionId[sessionId] = meta;
    }),
  removeQueueEntry: (sessionId, entryId) =>
    set((draft) => {
      const list = draft.queue.bySessionId[sessionId];
      if (!list) return;
      draft.queue.bySessionId[sessionId] = list.filter((entry) => entry.id !== entryId);
      const meta = draft.queue.metaBySessionId[sessionId];
      if (meta) {
        meta.count = draft.queue.bySessionId[sessionId].length;
      }
    }),
  setQueueLoading: (sessionId, loading) =>
    set((draft) => {
      draft.queue.isLoading[sessionId] = loading;
    }),
  clearQueueStatus: (sessionId) =>
    set((draft) => {
      delete draft.queue.bySessionId[sessionId];
      delete draft.queue.metaBySessionId[sessionId];
      delete draft.queue.isLoading[sessionId];
    }),
});
