"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { taskSessionsQueryOptions } from "@/lib/query/query-options";

/**
 * Custom hook that centralizes task session fetching logic.
 * First checks the store, then fetches from the backend if needed.
 *
 * @param taskId - The task ID to fetch session for (null if no task selected)
 * @returns Object with sessionId, hasSession flag, and isLoading state
 */
export function useTaskSession(taskId: string | null) {
  const sessionsQuery = useQuery(taskSessionsQueryOptions(taskId ?? ""));
  const sessionsFromStore = useAppStore((state) =>
    taskId ? state.taskSessionsByTask.itemsByTaskId[taskId] : null,
  );

  // Derive the session ID from store first, fall back to fetched value
  const finalSessionId = useMemo(() => {
    const sessions = sessionsQuery.data?.sessions ?? sessionsFromStore;
    if (!sessions || sessions.length === 0) return null;
    const primary = sessions.find((s) => s.is_primary);
    return (primary ?? sessions[0])?.id ?? null;
  }, [sessionsFromStore, sessionsQuery.data?.sessions]);

  return {
    sessionId: taskId ? finalSessionId : null,
    hasSession: !!taskId && !!finalSessionId,
    isLoading: Boolean(taskId && sessionsQuery.isFetching && !finalSessionId),
  };
}
