"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getGoal, type GoalResponse } from "@/lib/api/domains/coordinator-api";
import { useWebSocketClient } from "@/lib/ws/connection";

export type GoalLoadStatus = "loading" | "ready" | "error";

/**
 * The coordinator's goal read (`GET goal`), refetched on `coordinator.updated`.
 * The response of the last request sent wins; a failed refetch keeps what was
 * already loaded, so `error` means there is nothing to show.
 */
export function useGoal(workspaceId: string, coordinatorId: string) {
  const [data, setData] = useState<GoalResponse | null>(null);
  const [status, setStatus] = useState<GoalLoadStatus>("loading");
  const sequenceRef = useRef(0);
  const loadedRef = useRef(false);

  const reload = useCallback(() => {
    const sequence = ++sequenceRef.current;
    getGoal(workspaceId, coordinatorId)
      .then((response) => {
        if (sequence !== sequenceRef.current) return;
        loadedRef.current = true;
        setData(response);
        setStatus("ready");
      })
      .catch(() => {
        if (sequence !== sequenceRef.current || loadedRef.current) return;
        setStatus("error");
      });
  }, [workspaceId, coordinatorId]);

  const retry = useCallback(() => {
    setStatus("loading");
    reload();
  }, [reload]);

  useEffect(() => {
    loadedRef.current = false;
    setData(null);
    setStatus("loading");
    reload();
    return () => {
      sequenceRef.current += 1;
    };
  }, [reload]);

  const wsClient = useWebSocketClient();
  useEffect(() => {
    if (!wsClient) return;
    return wsClient.on("coordinator.updated", (message) => {
      const payload = message.payload;
      if (payload.workspace_id !== workspaceId || payload.coordinator_id !== coordinatorId) return;
      reload();
    });
  }, [wsClient, workspaceId, coordinatorId, reload]);

  return { data, status, reload, retry };
}
