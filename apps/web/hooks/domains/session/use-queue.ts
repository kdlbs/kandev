import { useCallback, useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import {
  queueMessage,
  clearQueue,
  drainQueuedMessage,
  updateQueuedMessage,
  removeQueuedEntry,
  QueueEntryNotFoundError,
} from "@/lib/api/domains/queue-api";
import { queueStatusQueryOptions } from "@/lib/query/query-options";
import type { QueueStatus, QueuedMessage } from "@/lib/state/slices/session/types";

const EMPTY_ENTRIES: QueuedMessage[] = [];
const EMPTY_STATUS: QueueStatus = { entries: EMPTY_ENTRIES, count: 0, max: 0 };

export type MessageAttachment = {
  type: string;
  data: string;
  mime_type: string;
  name?: string;
  delivery_mode?: "prompt" | "path";
};

type QueueActionsArgs = {
  sessionId: string | null;
  setQueueLoading: (sessionId: string, loading: boolean) => void;
  metaMax: number | undefined;
  queryClient: QueryClient;
};

function useDrainNextAction(
  sessionId: string | null,
  setQueueLoading: (sessionId: string, loading: boolean) => void,
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

function setQueueStatus(queryClient: QueryClient, sessionId: string, status: QueueStatus) {
  queryClient.setQueryData(queueStatusQueryOptions(sessionId).queryKey, status);
}

function patchQueueStatus(
  queryClient: QueryClient,
  sessionId: string,
  updater: (current: QueueStatus) => QueueStatus,
) {
  queryClient.setQueryData<QueueStatus>(queueStatusQueryOptions(sessionId).queryKey, (current) =>
    updater(current ?? EMPTY_STATUS),
  );
}

function removeEntryFromStatus(status: QueueStatus, entryId: string): QueueStatus {
  const entries = (status.entries ?? []).filter((entry) => entry.id !== entryId);
  return { entries, count: entries.length, max: status.max };
}

/** Build an action set bound to the supplied session and Query cache. */
function useQueueActions({ sessionId, setQueueLoading, metaMax, queryClient }: QueueActionsArgs) {
  const refetch = useCallback(
    async (sid: string) => {
      try {
        setQueueLoading(sid, true);
        await queryClient.fetchQuery({ ...queueStatusQueryOptions(sid), staleTime: 0 });
      } finally {
        setQueueLoading(sid, false);
      }
    },
    [queryClient, setQueueLoading],
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
      // Reset to a neutral capacity snapshot; the next status_changed event
      // will replace it with the authoritative server value. Using the
      // pre-clear entry count as a fallback for `max` was wrong (it would
      // pretend the cap equals "however many were queued").
      setQueueStatus(queryClient, sessionId, {
        entries: [],
        count: 0,
        max: metaMax ?? 0,
      });
    } finally {
      setQueueLoading(sessionId, false);
    }
  }, [sessionId, setQueueLoading, metaMax, queryClient]);

  const drainNext = useDrainNextAction(sessionId, setQueueLoading, refetch);

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
      patchQueueStatus(queryClient, sessionId, (current) =>
        removeEntryFromStatus(current, entryId),
      );
      try {
        await removeQueuedEntry({ session_id: sessionId, entry_id: entryId });
      } catch (err) {
        if (err instanceof QueueEntryNotFoundError) return;
        await refetch(sessionId);
        throw err;
      }
    },
    [sessionId, queryClient, refetch],
  );

  return { refetch, queue, clearAll, drainNext, editEntry, removeEntry };
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
  const queryClient = useQueryClient();
  const connectionStatus = useAppStore((appState) => appState.connection.status);
  const statusQuery = useQuery({
    ...queueStatusQueryOptions(sessionId ?? ""),
    enabled: Boolean(sessionId && connectionStatus === "connected"),
  });
  const [loadingBySessionId, setLoadingBySessionId] = useState<Record<string, boolean>>({});
  const setQueueLoading = useCallback((sid: string, loading: boolean) => {
    setLoadingBySessionId((current) => {
      if ((current[sid] ?? false) === loading) return current;
      const next = { ...current };
      if (loading) {
        next[sid] = true;
      } else {
        delete next[sid];
      }
      return next;
    });
  }, []);
  const status = statusQuery.data ?? EMPTY_STATUS;
  const entries = status.entries ?? EMPTY_ENTRIES;
  const isLoading = sessionId ? (loadingBySessionId[sessionId] ?? false) : false;
  const { refetch, queue, clearAll, drainNext, editEntry, removeEntry } = useQueueActions({
    sessionId,
    setQueueLoading,
    metaMax: status.max,
    queryClient,
  });

  const previousConnectionStatusRef = useRef(connectionStatus);
  useEffect(() => {
    const previous = previousConnectionStatusRef.current;
    previousConnectionStatusRef.current = connectionStatus;
    if (!sessionId) return;
    if (connectionStatus !== "connected" || previous === "connected") return;
    void refetch(sessionId).catch((err) => {
      console.error("Failed to fetch queue status:", err);
    });
  }, [sessionId, connectionStatus, refetch]);

  useEffect(() => {
    if (!sessionId) return;
    const refetchOnVisible = () => {
      if (document.visibilityState !== "visible") return;
      if (connectionStatus !== "connected") return;
      void refetch(sessionId).catch((err) => {
        console.error("Failed to fetch queue status after visibility change:", err);
      });
    };
    document.addEventListener("visibilitychange", refetchOnVisible);
    return () => document.removeEventListener("visibilitychange", refetchOnVisible);
  }, [sessionId, connectionStatus, refetch]);

  const refetchBound = useCallback(
    () => (sessionId ? refetch(sessionId) : Promise.resolve()),
    [sessionId, refetch],
  );

  return {
    entries,
    count: status.count ?? entries.length,
    max: status.max ?? 0,
    isFull: status.count >= status.max && status.max > 0,
    isLoading: isLoading || statusQuery.isFetching,
    queue,
    clearAll,
    drainNext,
    editEntry,
    removeEntry,
    refetch: refetchBound,
  };
}
