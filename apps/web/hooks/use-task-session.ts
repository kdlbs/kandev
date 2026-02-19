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

  const [fetchedSessionId, setFetchedSessionId] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  // Derive the session ID from store first, fall back to fetched value
  const sessionIdFromStore = useMemo(() => {
    return sessionsFromStore?.[0]?.id ?? null;
  }, [sessionsFromStore]);

  const finalSessionId = sessionIdFromStore ?? fetchedSessionId;

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
          setFetchedSessionId(null);
          setIsLoading(false);
        }
        return;
      }

      try {
        setIsLoading(true);
        const response = await client.request<{ sessions: Array<{ id: string }> }>(
          "task.session.list",
          { task_id: taskId },
          10000,
        );
        if (isActive) {
          setFetchedSessionId(response.sessions[0]?.id ?? null);
          setIsLoading(false);
        }
      } catch {
        if (isActive) {
          setFetchedSessionId(null);
          setIsLoading(false);
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
    isLoading: taskId && !sessionIdFromStore ? isLoading : false,
  };
}
