"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  ACTIVITY_CLASSES,
  getActivitySummary,
  type ActivityClass,
  type ClassSummary,
} from "@/lib/api/domains/coordinator-activity-api";
import { useWebSocketClient } from "@/lib/ws/connection";

export type ActionSummaryStatus = "loading" | "ready" | "error";
export type ActionCounts = Partial<Record<ActivityClass, ClassSummary>>;

const isCount = (value: unknown): value is number =>
  Number.isInteger(value) && (value as number) >= 0;

/** A read is usable only when every class is present with integer counts. */
export function validCounts(classes: ActionCounts | undefined): ActionCounts | null {
  if (!classes) return null;
  for (const action of ACTIVITY_CLASSES) {
    const entry = classes[action];
    if (!entry || !isCount(entry.approved) || !isCount(entry.rejected)) return null;
  }
  return classes;
}

/** The 30-day approved/rejected counts per action, re-read on `coordinator.updated`; the latest read wins. */
export function useActionSummary(workspaceId: string, coordinatorId: string, days = 30) {
  const [counts, setCounts] = useState<ActionCounts | null>(null);
  const [status, setStatus] = useState<ActionSummaryStatus>("loading");
  const sequenceRef = useRef(0);
  const loadedRef = useRef(false);

  const reload = useCallback(() => {
    const sequence = ++sequenceRef.current;
    getActivitySummary(workspaceId, coordinatorId, days)
      .then((summary) => {
        if (sequence !== sequenceRef.current) return;
        const valid = validCounts(summary.classes);
        if (!valid) throw new Error("invalid summary");
        loadedRef.current = true;
        setCounts(valid);
        setStatus("ready");
      })
      .catch(() => {
        if (sequence !== sequenceRef.current || loadedRef.current) return;
        setStatus("error");
      });
  }, [workspaceId, coordinatorId, days]);

  const retry = useCallback(() => {
    setStatus("loading");
    reload();
  }, [reload]);

  useEffect(() => {
    loadedRef.current = false;
    setCounts(null);
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

  return { counts, status, retry };
}
