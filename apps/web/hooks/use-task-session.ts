"use client";

import { useState, useEffect, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";

/**
 * Custom hook that centralizes task session fetching logic.
 * First checks the store, then fetches from the backend if needed.
 *
 * @param taskId - The task ID to fetch session for (null if no task selected)
 * @returns Object with sessionId, hasSession flag, and isLoading state
 */
export function useTaskSession(taskId: string | null) {
  const sessionsFromStore = useAppStore((state) =>
    taskId ? state.taskSessionsByTask.itemsByTaskId[taskId] : null,
  );

  const [fetched, setFetched] = useState<{
    taskId: string | null;
    sessionId: string | null;
    isLoading: boolean;
  }>({ taskId: null, sessionId: null, isLoading: false });

  // Derive the session ID from store first, fall back to fetched value
  const sessionIdFromStore = useMemo(() => {
    if (!sessionsFromStore || sessionsFromStore.length === 0) return null;
    const primary = sessionsFromStore.find((s) => s.is_primary);
    return (primary ?? sessionsFromStore[0])?.id ?? null;
  }, [sessionsFromStore]);

  const currentFallback = fetched.taskId === taskId ? fetched : null;
  const finalSessionId = sessionIdFromStore ?? currentFallback?.sessionId ?? null;

  useEffect(() => {
    // If we have session in store or no taskId, don't fetch
    if (sessionIdFromStore || !taskId) {
      return;
    }

    // Fetch if not in store and we have a taskId
    let isActive = true;
    const fetchSession = async () => {
      const client = getWebSocketClient();
      if (!client) {
        if (isActive) {
          setFetched({ taskId, sessionId: null, isLoading: false });
        }
        return;
      }

      try {
        setFetched((previous) => ({
          taskId,
          sessionId: previous.taskId === taskId ? previous.sessionId : null,
          isLoading: true,
        }));
        const response = await client.request<{ sessions: Array<{ id: string }> }>(
          "task.session.list",
          { task_id: taskId },
          10000,
        );
        if (isActive) {
          setFetched({ taskId, sessionId: response.sessions[0]?.id ?? null, isLoading: false });
        }
      } catch {
        if (isActive) {
          setFetched({ taskId, sessionId: null, isLoading: false });
        }
      }
    };

    fetchSession();

    return () => {
      isActive = false;
    };
  }, [taskId, sessionIdFromStore]);

  return {
    sessionId: taskId ? finalSessionId : null,
    hasSession: !!taskId && !!finalSessionId,
    isLoading: taskId && !sessionIdFromStore ? (currentFallback?.isLoading ?? false) : false,
  };
}
