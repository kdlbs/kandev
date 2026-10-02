"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { listPendingChanges, type PendingChange } from "@/lib/api/domains/coordinator-changes-api";

export type PendingChangesStatus = "loading" | "ready" | "error";

export type PendingChangesInput = {
  status: PendingChangesStatus;
  changes: PendingChange[];
  refetch: () => void;
};

/**
 * The coordinator's changes waiting for a decision. Only the latest issued read
 * may write, so a slow earlier read never overwrites a newer list.
 */
export function usePendingChanges(workspaceId: string, coordinatorId: string): PendingChangesInput {
  const [status, setStatus] = useState<PendingChangesStatus>("loading");
  const [changes, setChanges] = useState<PendingChange[]>([]);
  const sequenceRef = useRef(0);

  const load = useCallback(
    (initial: boolean) => {
      const sequence = ++sequenceRef.current;
      if (initial) setStatus("loading");
      listPendingChanges(workspaceId, coordinatorId)
        .then((result) => {
          if (sequence !== sequenceRef.current) return;
          setChanges(result.changes.filter((change) => change.status === "pending"));
          setStatus("ready");
        })
        .catch(() => {
          if (sequence !== sequenceRef.current) return;
          setStatus("error");
        });
    },
    [workspaceId, coordinatorId],
  );

  useEffect(() => {
    load(true);
    return () => {
      sequenceRef.current += 1;
    };
  }, [load]);

  const refetch = useCallback(() => load(false), [load]);
  const retry = useCallback(() => load(true), [load]);
  return { status, changes, refetch: status === "error" ? retry : refetch };
}
