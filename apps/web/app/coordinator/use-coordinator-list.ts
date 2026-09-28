"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { listCoordinators, type Coordinator } from "@/lib/api/domains/coordinator-api";

export type UseCoordinatorListResult = {
  /** The workspace's coordinators, in list order (AC-COORDINATOR-COORDINATORS-003.1). Undefined before the first successful read. */
  coordinators: Coordinator[] | undefined;
  /** Set while the latest read has failed. The previous `coordinators` value, if any, is kept. */
  error: boolean;
  /** Re-reads the list. */
  retry: () => void;
};

/**
 * Reads a workspace's coordinators for the sidebar entries, the coordinator
 * routes' no-coordinator/unknown-coordinator states, and the Needs you/Queue
 * header selector (docs/specs/coordinator/system-design/needs-you.md
 * #routes-and-sidebar, #screens).
 */
export function useCoordinatorList(workspaceId: string | null): UseCoordinatorListResult {
  const [coordinators, setCoordinators] = useState<Coordinator[] | undefined>(undefined);
  const [error, setError] = useState(false);
  const seqRef = useRef(0);

  const read = useCallback((ws: string) => {
    const seq = ++seqRef.current;
    listCoordinators(ws)
      .then((res) => {
        if (seqRef.current !== seq) return;
        setCoordinators(res.coordinators);
        setError(false);
      })
      .catch(() => {
        if (seqRef.current !== seq) return;
        setError(true);
      });
  }, []);

  useEffect(() => {
    setCoordinators(undefined);
    setError(false);
    if (!workspaceId) return;
    read(workspaceId);
  }, [workspaceId, read]);

  const retry = useCallback(() => {
    if (!workspaceId) return;
    read(workspaceId);
  }, [workspaceId, read]);

  return { coordinators, error, retry };
}
