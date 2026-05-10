import { useEffect, useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  queueMessage,
  clearQueue,
  getQueueStatus,
  updateQueuedMessage,
  removeQueuedEntry,
  QueueEntryNotFoundError,
} from "@/lib/api/domains/queue-api";
import type { QueuedMessage } from "@/lib/state/slices/session/types";

const EMPTY_ENTRIES: QueuedMessage[] = [];

export type MessageAttachment = {
  type: string;
  data: string;
  mime_type: string;
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
  entriesLength: number;
};

/** Build an action set bound to the supplied session + slice setters. */
function useQueueActions({
  sessionId,
  setQueueEntries,
  removeQueueEntry,
  setQueueLoading,
  metaMax,
  entriesLength,
}: QueueActionsArgs) {
  const refetch = useCallback(
    async (sid: string) => {
      try {
        setQueueLoading(sid, true);
        const status = await getQueueStatus(sid);
        setQueueEntries(sid, status.entries ?? [], { count: status.count, max: status.max });
      } finally {
        setQueueLoading(sid, false);
      }
    },
    [setQueueEntries, setQueueLoading],
  );

  const queue = useCallback(
    async (
      taskId: string,
      content: string,
      model?: string,
      planMode?: boolean,
      attachments?: MessageAttachment[],
    ) => {
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
    try {
      await clearQueue(sessionId);
      setQueueEntries(sessionId, [], { count: 0, max: metaMax ?? entriesLength });
    } finally {
      setQueueLoading(sessionId, false);
    }
  }, [sessionId, setQueueEntries, setQueueLoading, metaMax, entriesLength]);

  const editEntry = useCallback(
    async (entryId: string, content: string, attachments?: MessageAttachment[]) => {
      if (!sessionId) return;
      try {
        await updateQueuedMessage({
          session_id: sessionId,
          entry_id: entryId,
          content,
          attachments,
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
      try {
        await removeQueuedEntry({ session_id: sessionId, entry_id: entryId });
      } catch (err) {
        if (err instanceof QueueEntryNotFoundError) return;
        await refetch(sessionId);
        throw err;
      }
    },
    [sessionId, refetch, removeQueueEntry],
  );

  return { refetch, queue, clearAll, editEntry, removeEntry };
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
  const { refetch, queue, clearAll, editEntry, removeEntry } = useQueueActions({
    sessionId,
    setQueueEntries: state.setQueueEntries,
    removeQueueEntry: state.removeQueueEntry,
    setQueueLoading: state.setQueueLoading,
    metaMax: meta?.max,
    entriesLength: entries.length,
  });

  useEffect(() => {
    if (!sessionId) return;
    void refetch(sessionId).catch((err) => {
      console.error("Failed to fetch queue status:", err);
    });
  }, [sessionId, refetch]);

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
    editEntry,
    removeEntry,
    refetch: refetchBound,
  };
}
