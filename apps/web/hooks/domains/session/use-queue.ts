import { useEffect, useCallback, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import {
  queueMessage,
  clearQueue,
  drainQueuedMessage,
  getQueueStatus,
  updateQueuedMessage,
  removeQueuedEntry,
  mergeQueuedEntry,
  QueueEntryNotFoundError,
} from "@/lib/api/domains/queue-api";
import type { QueuedMessage } from "@/lib/state/slices/session/types";
import type { EntityReference } from "@/lib/types/entity-reference";

const EMPTY_ENTRIES: QueuedMessage[] = [];

export type MessageAttachment = {
  type: string;
  data: string;
  mime_type: string;
  name?: string;
  delivery_mode?: "prompt" | "path";
};

export type QueueMessageInput = {
  taskId: string;
  content: string;
  model?: string;
  planMode?: boolean;
  attachments?: MessageAttachment[];
  entityReferences?: EntityReference[];
};

/** Selectors over the queue slice for one session. */
function useQueueState(sessionId: string | null) {
  const entries = useAppStore((state) =>
    sessionId ? (state.queue.bySessionId[sessionId] ?? EMPTY_ENTRIES) : EMPTY_ENTRIES,
  );
  const meta = useAppStore((state) =>
    sessionId ? state.queue.metaBySessionId[sessionId] : undefined,
  );
  const isLoading = useAppStore((state) =>
    sessionId ? (state.queue.isLoading[sessionId] ?? false) : false,
  );
  const setQueueEntries = useAppStore((state) => state.setQueueEntries);
  const removeQueueEntry = useAppStore((state) => state.removeQueueEntry);
  const setQueueLoading = useAppStore((state) => state.setQueueLoading);
  return { entries, meta, isLoading, setQueueEntries, removeQueueEntry, setQueueLoading };
}

type QueueActionsArgs = {
  sessionId: string | null;
  setQueueEntries: ReturnType<typeof useQueueState>["setQueueEntries"];
  removeQueueEntry: ReturnType<typeof useQueueState>["removeQueueEntry"];
  setQueueLoading: ReturnType<typeof useQueueState>["setQueueLoading"];
  metaMax: number | undefined;
};

function useDrainNextAction(
  sessionId: string | null,
  setQueueLoading: ReturnType<typeof useQueueState>["setQueueLoading"],
  refetch: (sid: string) => Promise<void>,
) {
  return useCallback(async () => {
    if (!sessionId) return;
    setQueueLoading(sessionId, true);
    try {
      await drainQueuedMessage(sessionId);
      await refetch(sessionId);
    } finally {
      setQueueLoading(sessionId, false);
    }
  }, [sessionId, refetch, setQueueLoading]);
}

/** Build an action set bound to the supplied session + slice setters. */
function useQueueActions({
  sessionId,
  setQueueEntries,
  removeQueueEntry,
  setQueueLoading,
  metaMax,
}: QueueActionsArgs) {
  // Monotonic per-session refetch version. Concurrent mutations (merge, edit,
  // remove, drain) all refetch; without a guard an older in-flight
  // getQueueStatus response can land after a newer one and restore a stale
  // snapshot (e.g. the pre-merge row). Discard any response that is no longer
  // the latest issued for its session.
  const refetchVersion = useRef<Record<string, number>>({});
  const refetch = useCallback(
    async (sid: string) => {
      const version = (refetchVersion.current[sid] ?? 0) + 1;
      refetchVersion.current[sid] = version;
      try {
        setQueueLoading(sid, true);
        const status = await getQueueStatus(sid);
        if (refetchVersion.current[sid] !== version) return;
        setQueueEntries(sid, status.entries ?? [], { count: status.count, max: status.max });
      } finally {
        if (refetchVersion.current[sid] === version) {
          setQueueLoading(sid, false);
        }
      }
    },
    [setQueueEntries, setQueueLoading],
  );

  const queue = useCallback(
    async ({
      taskId,
      content,
      model,
      planMode,
      attachments,
      entityReferences,
    }: QueueMessageInput) => {
      if (!sessionId) return;
      setQueueLoading(sessionId, true);
      try {
        await queueMessage({
          session_id: sessionId,
          task_id: taskId,
          content,
          model,
          plan_mode: planMode,
          attachments,
          entity_references: entityReferences,
        });
        await refetch(sessionId);
      } finally {
        setQueueLoading(sessionId, false);
      }
    },
    [sessionId, refetch, setQueueLoading],
  );

  const clearAll = useCallback(async () => {
    if (!sessionId) return;
    setQueueLoading(sessionId, true);
    let mutationFailed = false;
    let mutationError: unknown;
    try {
      try {
        await clearQueue(sessionId);
        // Invalidate any pre-clear status request before publishing the
        // optimistic empty state. The explicit refetch below then owns the
        // newest version and replaces it with the backend's exact result.
        refetchVersion.current[sessionId] = (refetchVersion.current[sessionId] ?? 0) + 1;
        setQueueEntries(sessionId, [], { count: 0, max: metaMax ?? 0 });
      } catch (err) {
        mutationFailed = true;
        mutationError = err;
      }

      try {
        await refetch(sessionId);
      } catch (reconcileError) {
        // Preserve the mutation error when both operations fail; it best
        // explains why the requested action did not complete.
        if (mutationFailed) throw mutationError;
        throw reconcileError;
      }
      if (mutationFailed) throw mutationError;
    } finally {
      setQueueLoading(sessionId, false);
    }
  }, [sessionId, setQueueEntries, setQueueLoading, metaMax, refetch]);

  const drainNext = useDrainNextAction(sessionId, setQueueLoading, refetch);

  const { editEntry, removeEntry, mergeEntry } = useEntryMutations({
    sessionId,
    removeQueueEntry,
    refetch,
  });

  return { refetch, queue, clearAll, drainNext, editEntry, removeEntry, mergeEntry };
}

type EntryMutationsArgs = {
  sessionId: string | null;
  removeQueueEntry: ReturnType<typeof useQueueState>["removeQueueEntry"];
  refetch: (sid: string) => Promise<void>;
};

/** Entry-level mutations (edit / remove / merge) that refetch on success and
 * resync on a drain race (QueueEntryNotFoundError). */
function useEntryMutations({ sessionId, removeQueueEntry, refetch }: EntryMutationsArgs) {
  const editEntry = useCallback(
    async (
      entryId: string,
      content: string,
      attachments?: MessageAttachment[],
      entityReferences: EntityReference[] = [],
    ) => {
      if (!sessionId) return;
      try {
        await updateQueuedMessage({
          session_id: sessionId,
          entry_id: entryId,
          content,
          attachments,
          entity_references: entityReferences,
        });
        await refetch(sessionId);
      } catch (err) {
        if (err instanceof QueueEntryNotFoundError) {
          await refetch(sessionId);
        }
        throw err;
      }
    },
    [sessionId, refetch],
  );

  const removeEntry = useCallback(
    async (entryId: string) => {
      if (!sessionId) return;
      removeQueueEntry(sessionId, entryId);
      let mutationFailed = false;
      let mutationError: unknown;
      try {
        await removeQueuedEntry({ session_id: sessionId, entry_id: entryId });
      } catch (err) {
        mutationFailed = true;
        mutationError = err;
      }

      try {
        await refetch(sessionId);
      } catch (reconcileError) {
        if (mutationFailed && !(mutationError instanceof QueueEntryNotFoundError)) {
          throw mutationError;
        }
        throw reconcileError;
      }

      // A drain/remove race is already reflected by the authoritative status,
      // so it is a successful UI outcome. Other failures restore state above
      // and remain actionable to the caller.
      if (mutationFailed && !(mutationError instanceof QueueEntryNotFoundError)) {
        throw mutationError;
      }
    },
    [sessionId, refetch, removeQueueEntry],
  );

  const mergeEntry = useCallback(
    async (entryId: string, userId?: string) => {
      if (!sessionId) return;
      try {
        await mergeQueuedEntry({ session_id: sessionId, entry_id: entryId, user_id: userId });
        await refetch(sessionId);
      } catch (err) {
        if (err instanceof QueueEntryNotFoundError) {
          await refetch(sessionId);
        }
        throw err;
      }
    },
    [sessionId, refetch],
  );

  return { editEntry, removeEntry, mergeEntry };
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
  const { entries, meta, isLoading } = state;
  const connectionStatus = useAppStore((appState) => appState.connection.status);
  const { refetch, queue, clearAll, drainNext, editEntry, removeEntry, mergeEntry } =
    useQueueActions({
      sessionId,
      setQueueEntries: state.setQueueEntries,
      removeQueueEntry: state.removeQueueEntry,
      setQueueLoading: state.setQueueLoading,
      metaMax: meta?.max,
    });

  useEffect(() => {
    if (!sessionId) return;
    if (connectionStatus !== "connected") return;
    void refetch(sessionId).catch((err) => {
      console.error("Failed to fetch queue status:", err);
    });
  }, [sessionId, connectionStatus, refetch]);

  useForegroundRefresh(
    () => {
      if (!sessionId) return;
      if (connectionStatus !== "connected") return;
      return refetch(sessionId).catch((err) => {
        console.error("Failed to fetch queue status after foreground refresh:", err);
      });
    },
    Boolean(sessionId),
    sessionId,
  );

  const refetchBound = useCallback(
    () => (sessionId ? refetch(sessionId) : Promise.resolve()),
    [sessionId, refetch],
  );

  return {
    entries,
    count: meta?.count ?? entries.length,
    max: meta?.max ?? 0,
    isFull: meta ? meta.count >= meta.max && meta.max > 0 : false,
    isLoading,
    queue,
    clearAll,
    drainNext,
    editEntry,
    removeEntry,
    mergeEntry,
    refetch: refetchBound,
  };
}
