"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { ControlAction } from "@/lib/api/domains/coordinator-api";
import {
  getClassEligibility,
  recordClassReview,
  type ClassEligibility,
} from "@/lib/api/domains/coordinator-automatic-api";
import { useWebSocketClient } from "@/lib/ws/connection";

export type ClassEligibilityStatus = "loading" | "ready" | "error";

/**
 * One class's automatic eligibility, re-read on `coordinator.updated`; the
 * latest read wins. `enabled` false reads nothing.
 */
export function useClassEligibility(
  workspaceId: string,
  coordinatorId: string,
  actionClass: ControlAction,
  enabled: boolean,
) {
  const [eligibility, setEligibility] = useState<ClassEligibility | null>(null);
  const [status, setStatus] = useState<ClassEligibilityStatus>("loading");
  const [reviewFailed, setReviewFailed] = useState(false);
  const sequenceRef = useRef(0);

  const reload = useCallback(() => {
    if (!enabled) return;
    const sequence = ++sequenceRef.current;
    getClassEligibility(workspaceId, coordinatorId, actionClass)
      .then((next) => {
        if (sequence !== sequenceRef.current) return;
        setEligibility(next);
        setStatus("ready");
      })
      .catch(() => {
        if (sequence !== sequenceRef.current) return;
        setStatus("error");
      });
  }, [workspaceId, coordinatorId, actionClass, enabled]);

  useEffect(() => {
    setEligibility(null);
    setStatus("loading");
    reload();
    return () => {
      sequenceRef.current += 1;
    };
  }, [reload]);

  const wsClient = useWebSocketClient();
  useEffect(() => {
    if (!wsClient || !enabled) return;
    return wsClient.on("coordinator.updated", (message) => {
      const payload = message.payload;
      if (payload.workspace_id !== workspaceId || payload.coordinator_id !== coordinatorId) return;
      reload();
    });
  }, [wsClient, enabled, workspaceId, coordinatorId, reload]);

  const markReviewed = useCallback(async () => {
    setReviewFailed(false);
    try {
      await recordClassReview(workspaceId, coordinatorId, actionClass);
      reload();
    } catch {
      setReviewFailed(true);
    }
  }, [workspaceId, coordinatorId, actionClass, reload]);

  return { eligibility, status, reload, markReviewed, reviewFailed };
}
