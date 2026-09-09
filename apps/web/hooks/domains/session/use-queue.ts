import { useEffect, useCallback, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import {
  clearQueue,
  updateQueuedMessage,
  removeQueuedEntry,
  mergeQueuedEntry,
  reorderQueuedEntries,
  QueueEntryNotFoundError,
  sendQueuedNow,
  setQueueAutoRun,
  setQueueAutoMerge,
} from "@/lib/api/domains/queue-api";
import type { QueueSessionIdentity } from "@/lib/api/domains/queue-api";
import type {
  QueueMeta,
  QueueOperationToken,
  QueuedMessage,
} from "@/lib/state/slices/session/types";
import type { MessageAttachment } from "./use-queue-types";
export type { MessageAttachment, QueueMessageInput } from "./use-queue-types";
import { useQueueAdmissionAction } from "./use-queue-admission";
import { useQueueRefetch, type QueueRefetch } from "./use-queue-refetch";
import type { EntityReference } from "@/lib/types/entity-reference";

const EMPTY_ENTRIES: QueuedMessage[] = [];

/** Selectors over the queue slice for one session. */
function useQueueState(sessionId: string | null) {
  const entries = useAppStore((state) =>
    sessionId ? (state.queue.bySessionId[sessionId] ?? EMPTY_ENTRIES) : EMPTY_ENTRIES,
  );
  const meta = useAppStore((state) =>
    sessionId ? state.queue.metaBySessionId[sessionId] : undefined,
  );
  const setQueueEntries = useAppStore((state) => state.setQueueEntries);
  const removeQueueEntry = useAppStore((state) => state.removeQueueEntry);
  const activeOperation = useAppStore((state) =>
    sessionId ? state.queue.activeOperationBySessionId[sessionId] : undefined,
  );
  const beginQueueOperation = useAppStore((state) => state.beginQueueOperation);
  const finishQueueOperation = useAppStore((state) => state.finishQueueOperation);
  const cancellationPending = useAppStore(
    (state) =>
      (sessionId ? state.taskSessions.items[sessionId]?.cancellation_pending : false) === true,
  );
  const taskSession = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId] : undefined,
  );
  return {
    entries,
    meta,
    isLoading: activeOperation !== undefined,
    cancellationPending,
    taskSession,
    setQueueEntries,
    removeQueueEntry,
    beginQueueOperation,
    finishQueueOperation,
  };
}

type QueueActionsArgs = {
  identity: QueueSessionIdentity | null;
  entries: QueuedMessage[];
  setQueueEntries: ReturnType<typeof useQueueState>["setQueueEntries"];
  removeQueueEntry: ReturnType<typeof useQueueState>["removeQueueEntry"];
  beginQueueOperation: ReturnType<typeof useQueueState>["beginQueueOperation"];
  finishQueueOperation: ReturnType<typeof useQueueState>["finishQueueOperation"];
  metaMax: number | undefined;
  metaMergeEnabled: boolean | undefined;
  metaAutoRun: boolean | undefined;
};

type BeginQueueOperation = ReturnType<typeof useQueueState>["beginQueueOperation"];
type FinishQueueOperation = ReturnType<typeof useQueueState>["finishQueueOperation"];

async function runQueueMutation(
  identity: QueueSessionIdentity | null,
  beginQueueOperation: BeginQueueOperation,
  finishQueueOperation: FinishQueueOperation,
  refetch: QueueRefetch,
  mutate: (identity: QueueSessionIdentity) => Promise<unknown>,
): Promise<void> {
  if (!identity) return;
  const { session_id: sessionId, session_incarnation_id: incarnationId } = identity;
  const token = beginQueueOperation(sessionId, incarnationId);
  if (!token) return;
  let mutationError: unknown;
  try {
    try {
      await mutate(identity);
    } catch (error) {
      mutationError = error;
    }
    try {
      await refetch(sessionId, token);
    } catch (reconcileError) {
      if (mutationError) throw mutationError;
      throw reconcileError;
    }
    if (mutationError) throw mutationError;
  } finally {
    finishQueueOperation(sessionId, token);
  }
}

function useSendNowAction(
  identity: QueueSessionIdentity | null,
  beginQueueOperation: BeginQueueOperation,
  finishQueueOperation: FinishQueueOperation,
  refetch: QueueRefetch,
) {
  return useCallback(
    (entryId: string) =>
      runQueueMutation(identity, beginQueueOperation, finishQueueOperation, refetch, (current) =>
        sendQueuedNow({ ...current, scope: "entry", entry_id: entryId }),
      ),
    [beginQueueOperation, finishQueueOperation, identity, refetch],
  );
}

type BooleanQueueMutation = (identity: QueueSessionIdentity, enabled: boolean) => Promise<unknown>;

function useBooleanQueueAction(
  identity: QueueSessionIdentity | null,
  beginQueueOperation: BeginQueueOperation,
  finishQueueOperation: FinishQueueOperation,
  refetch: QueueRefetch,
  mutate: BooleanQueueMutation,
) {
  return useCallback(
    (enabled: boolean) =>
      runQueueMutation(identity, beginQueueOperation, finishQueueOperation, refetch, (current) =>
        mutate(current, enabled),
      ),
    [beginQueueOperation, finishQueueOperation, identity, mutate, refetch],
  );
}

/** Build an action set bound to the supplied session + slice setters. */
function useQueueActions({
  identity,
  entries,
  setQueueEntries,
  removeQueueEntry,
  beginQueueOperation,
  finishQueueOperation,
  metaMax,
  metaMergeEnabled,
  metaAutoRun,
}: QueueActionsArgs) {
  const { refetch, invalidate: invalidateRefetch } = useQueueRefetch(
    identity,
    setQueueEntries,
    beginQueueOperation,
    finishQueueOperation,
  );

  const queue = useQueueAdmissionAction(
    identity,
    beginQueueOperation,
    finishQueueOperation,
    refetch,
  );

  const clearAll = useClearAllAction({
    identity,
    setQueueEntries,
    beginQueueOperation,
    finishQueueOperation,
    metaMax,
    metaMergeEnabled,
    metaAutoRun,
    refetch,
    invalidateRefetch,
  });

  const sendEntryNow = useSendNowAction(
    identity,
    beginQueueOperation,
    finishQueueOperation,
    refetch,
  );
  const setAutoRun = useBooleanQueueAction(
    identity,
    beginQueueOperation,
    finishQueueOperation,
    refetch,
    setQueueAutoRun,
  );
  const setAutoMerge = useBooleanQueueAction(
    identity,
    beginQueueOperation,
    finishQueueOperation,
    refetch,
    setQueueAutoMerge,
  );

  const { editEntry, removeEntry, mergeEntry } = useEntryMutations({
    identity,
    removeQueueEntry,
    refetch,
    beginQueueOperation,
    finishQueueOperation,
  });

  const reorderEntries = useReorderEntriesAction({
    identity,
    entries,
    setQueueEntries,
    beginQueueOperation,
    finishQueueOperation,
    metaMax,
    metaMergeEnabled,
    metaAutoRun,
    refetch,
  });

  return {
    refetch,
    queue,
    clearAll,
    sendEntryNow,
    setAutoRun,
    setAutoMerge,
    editEntry,
    removeEntry,
    mergeEntry,
    reorderEntries,
  };
}

type ReorderEntriesArgs = {
  identity: QueueSessionIdentity | null;
  entries: QueuedMessage[];
  setQueueEntries: ReturnType<typeof useQueueState>["setQueueEntries"];
  beginQueueOperation: BeginQueueOperation;
  finishQueueOperation: FinishQueueOperation;
  metaMax: number | undefined;
  metaMergeEnabled: boolean | undefined;
  metaAutoRun: boolean | undefined;
  refetch: QueueRefetch;
};

type ClearAllArgs = {
  identity: QueueSessionIdentity | null;
  setQueueEntries: ReturnType<typeof useQueueState>["setQueueEntries"];
  beginQueueOperation: BeginQueueOperation;
  finishQueueOperation: FinishQueueOperation;
  metaMax: number | undefined;
  metaMergeEnabled: boolean | undefined;
  metaAutoRun: boolean | undefined;
  refetch: QueueRefetch;
  invalidateRefetch: (sid: string) => void;
};

/** Clears every pending entry: optimistic empty state, then the backend's
 * authoritative result. Reconcile errors preserve the mutation error. */
function useClearAllAction({
  identity,
  setQueueEntries,
  beginQueueOperation,
  finishQueueOperation,
  metaMax,
  metaMergeEnabled,
  metaAutoRun,
  refetch,
  invalidateRefetch,
}: ClearAllArgs) {
  return useCallback(async () => {
    if (!identity) return;
    const { session_id: sessionId, session_incarnation_id: incarnationId } = identity;
    const token = beginQueueOperation(sessionId, incarnationId);
    if (!token) return;
    let mutationFailed = false;
    let mutationError: unknown;
    try {
      try {
        await clearQueue(identity);
        invalidateRefetch(sessionId);
        setQueueEntries(sessionId, [], {
          count: 0,
          max: metaMax ?? 0,
          mergeEnabled: metaMergeEnabled ?? true,
          autoRun: metaAutoRun ?? true,
          taskId: identity.task_id,
          sessionIncarnationId: incarnationId,
        });
      } catch (err) {
        mutationFailed = true;
        mutationError = err;
      }
      try {
        await refetch(sessionId, token);
      } catch (reconcileError) {
        if (mutationFailed) throw mutationError;
        throw reconcileError;
      }
      if (mutationFailed) throw mutationError;
    } finally {
      finishQueueOperation(sessionId, token);
    }
  }, [
    beginQueueOperation,
    finishQueueOperation,
    identity,
    setQueueEntries,
    metaMax,
    metaMergeEnabled,
    metaAutoRun,
    refetch,
    invalidateRefetch,
  ]);
}

/** Rewrites the visible pending order, optimistically at first, then
 * reconciles to the backend's authoritative order. A queue_changed drift
 * (drain/remove/merge raced the drag) is refetched and rethrown as
 * QueueReorderError so the panel can swallow it without a toast. */
function useReorderEntriesAction({
  identity,
  entries,
  setQueueEntries,
  beginQueueOperation,
  finishQueueOperation,
  metaMax,
  metaMergeEnabled,
  metaAutoRun,
  refetch,
}: ReorderEntriesArgs) {
  return useCallback(
    async (orderedIds: string[]) => {
      if (!identity) return;
      const { session_id: sessionId, session_incarnation_id: incarnationId } = identity;
      const token = beginQueueOperation(sessionId, incarnationId);
      if (!token) return;
      const byId = new Map(entries.map((entry) => [entry.id, entry]));
      const reordered = orderedIds
        .map((id) => byId.get(id))
        .filter((entry): entry is QueuedMessage => entry !== undefined);
      if (reordered.length === entries.length) {
        setQueueEntries(sessionId, reordered, {
          count: reordered.length,
          max: metaMax ?? 0,
          mergeEnabled: metaMergeEnabled ?? true,
          autoRun: metaAutoRun ?? true,
        });
      }
      let mutationFailed = false;
      let mutationError: unknown;
      try {
        try {
          await reorderQueuedEntries({ ...identity, ordered_ids: orderedIds });
        } catch (err) {
          mutationFailed = true;
          mutationError = err;
        }
        try {
          await refetch(sessionId, token);
        } catch (reconcileError) {
          if (mutationFailed) throw mutationError;
          throw reconcileError;
        }
        if (mutationFailed) throw mutationError;
      } finally {
        finishQueueOperation(sessionId, token);
      }
    },
    [
      beginQueueOperation,
      entries,
      finishQueueOperation,
      identity,
      metaAutoRun,
      metaMax,
      metaMergeEnabled,
      refetch,
      setQueueEntries,
    ],
  );
}

type EntryMutationsArgs = {
  identity: QueueSessionIdentity | null;
  removeQueueEntry: ReturnType<typeof useQueueState>["removeQueueEntry"];
  beginQueueOperation: BeginQueueOperation;
  finishQueueOperation: FinishQueueOperation;
  refetch: QueueRefetch;
};

/** Entry-level mutations (edit / remove / merge) that refetch on success and
 * resync on a drain race (QueueEntryNotFoundError). */
function useEntryMutations({
  identity,
  removeQueueEntry,
  beginQueueOperation,
  finishQueueOperation,
  refetch,
}: EntryMutationsArgs) {
  const run = useCallback(
    async (operation: (token: QueueOperationToken) => Promise<void>) => {
      if (!identity) return;
      const token = beginQueueOperation(identity.session_id, identity.session_incarnation_id);
      if (!token) return;
      try {
        await operation(token);
      } finally {
        finishQueueOperation(identity.session_id, token);
      }
    },
    [beginQueueOperation, finishQueueOperation, identity],
  );
  const editEntry = useCallback(
    async (
      entryId: string,
      content: string,
      attachments?: MessageAttachment[],
      entityReferences: EntityReference[] = [],
    ) =>
      run(async (token) => {
        if (!identity) return;
        try {
          await updateQueuedMessage({
            ...identity,
            entry_id: entryId,
            content,
            attachments,
            entity_references: entityReferences,
          });
          await refetch(identity.session_id, token);
        } catch (err) {
          if (err instanceof QueueEntryNotFoundError) {
            await refetch(identity.session_id, token);
          }
          throw err;
        }
      }),
    [identity, refetch, run],
  );
  const removeEntry = useCallback(
    async (entryId: string) =>
      run(async (token) => {
        if (!identity) return;
        removeQueueEntry(identity.session_id, entryId);
        let mutationFailed = false;
        let mutationError: unknown;
        try {
          await removeQueuedEntry({ ...identity, entry_id: entryId });
        } catch (err) {
          mutationFailed = true;
          mutationError = err;
        }
        try {
          await refetch(identity.session_id, token);
        } catch (reconcileError) {
          if (mutationFailed && !(mutationError instanceof QueueEntryNotFoundError)) {
            throw mutationError;
          }
          throw reconcileError;
        }
        if (mutationFailed && !(mutationError instanceof QueueEntryNotFoundError)) {
          throw mutationError;
        }
      }),
    [identity, refetch, removeQueueEntry, run],
  );
  const mergeEntry = useCallback(
    async (entryId: string, userId?: string) =>
      run(async (token) => {
        if (!identity) return;
        try {
          await mergeQueuedEntry({ ...identity, entry_id: entryId, user_id: userId });
          await refetch(identity.session_id, token);
        } catch (err) {
          if (err instanceof QueueEntryNotFoundError) {
            await refetch(identity.session_id, token);
          }
          throw err;
        }
      }),
    [identity, refetch, run],
  );
  return { editEntry, removeEntry, mergeEntry };
}

function useCurrentQueueIdentity(
  sessionId: string | null,
  taskSession: ReturnType<typeof useQueueState>["taskSession"],
) {
  return useMemo<QueueSessionIdentity | null>(() => {
    if (!sessionId || !taskSession?.task_id || !taskSession.queue_incarnation_id) return null;
    return {
      task_id: taskSession.task_id,
      session_id: sessionId,
      session_incarnation_id: taskSession.queue_incarnation_id,
    };
  }, [sessionId, taskSession?.task_id, taskSession?.queue_incarnation_id]);
}

function useQueueRefresh(
  sessionId: string | null,
  connectionStatus: string,
  refetch: QueueRefetch,
) {
  useEffect(() => {
    if (!sessionId || connectionStatus !== "connected") return;
    void refetch(sessionId).catch((error) => {
      console.error("Failed to fetch queue status:", error);
    });
  }, [sessionId, connectionStatus, refetch]);
  useForegroundRefresh(
    () => {
      if (!sessionId || connectionStatus !== "connected") return;
      return refetch(sessionId).catch((error) => {
        console.error("Failed to fetch queue status after foreground refresh:", error);
      });
    },
    Boolean(sessionId),
    sessionId,
  );
  return useCallback(
    () => (sessionId ? refetch(sessionId) : Promise.resolve()),
    [sessionId, refetch],
  );
}

function queueSummary(meta: QueueMeta | undefined, entries: QueuedMessage[]) {
  return {
    count: meta?.count ?? entries.length,
    max: meta?.max ?? 0,
    isFull: meta ? meta.count >= meta.max && meta.max > 0 : false,
    mergeEnabled: meta?.mergeEnabled ?? true,
    autoRun: meta?.autoRun ?? true,
    autoMerge: meta?.autoMergeEnabled ?? false,
    autoMergeAvailable: meta?.autoMergeAvailable ?? false,
  };
}

/**
 * Reactive view over the per-session message queue plus optimistic mutators.
 *
 * - `entries` — ordered FIFO list (head at index 0) drained one-per-turn
 * - `count` / `max` — capacity snapshot from server
 * - Edit and remove rely on entry-level UUIDs: when a drain wins the race, the
 *   server returns `entry_not_found` and we refetch to resync the local list.
 */
export function useQueue(sessionId: string | null) {
  const state = useQueueState(sessionId);
  const { entries, meta, isLoading, cancellationPending, taskSession } = state;
  const identity = useCurrentQueueIdentity(sessionId, taskSession);
  const connectionStatus = useAppStore((appState) => appState.connection.status);
  const {
    refetch,
    queue,
    clearAll,
    sendEntryNow,
    setAutoMerge,
    setAutoRun,
    editEntry,
    removeEntry,
    mergeEntry,
    reorderEntries,
  } = useQueueActions({
    identity,
    entries,
    setQueueEntries: state.setQueueEntries,
    removeQueueEntry: state.removeQueueEntry,
    beginQueueOperation: state.beginQueueOperation,
    finishQueueOperation: state.finishQueueOperation,
    metaMax: meta?.max,
    metaMergeEnabled: meta?.mergeEnabled,
    metaAutoRun: meta?.autoRun,
  });

  const refetchBound = useQueueRefresh(sessionId, connectionStatus, refetch);

  return {
    entries,
    ...queueSummary(meta, entries),
    isLoading,
    cancellationPending,
    queue,
    clearAll,
    sendEntryNow,
    setAutoRun,
    setAutoMerge,
    editEntry,
    removeEntry,
    mergeEntry,
    reorderEntries,
    refetch: refetchBound,
  };
}
