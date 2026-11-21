"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getTaskChanges, type ChangeView } from "@/lib/api/domains/coordinator-merge-api";

export type TaskChangesStatus = "idle" | "loading" | "ready" | "error";

/**
 * The linked changes of a card, read once `enabled` turns true. `retry` bypasses the
 * backend's 60-second reuse. A failed read keeps the previous values in place.
 */
export function useTaskChanges(
  workspaceId: string,
  coordinatorId: string,
  taskId: string,
  enabled: boolean,
) {
  const [changes, setChanges] = useState<ChangeView[] | null>(null);
  const [status, setStatus] = useState<TaskChangesStatus>("idle");
  const sequenceRef = useRef(0);

  const read = useCallback(
    (retry: boolean) => {
      const sequence = ++sequenceRef.current;
      setStatus("loading");
      getTaskChanges(workspaceId, coordinatorId, taskId, retry)
        .then((next) => {
          if (sequence !== sequenceRef.current) return;
          setChanges(next ?? []);
          setStatus("ready");
        })
        .catch(() => {
          if (sequence !== sequenceRef.current) return;
          setStatus("error");
        });
    },
    [workspaceId, coordinatorId, taskId],
  );

  useEffect(() => {
    if (!enabled) return;
    read(false);
    return () => {
      sequenceRef.current += 1;
    };
  }, [enabled, read]);

  return { changes, status, retry: () => read(true) };
}
