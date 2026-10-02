import { useEffect, useRef, useState } from "react";
import { getCoordinator, type Coordinator } from "@/lib/api/domains/coordinator-api";

export type CoordinatorWatches = NonNullable<Coordinator["watches"]>;

type Loaded = { coordinatorId: string; watches: CoordinatorWatches | undefined };

/**
 * The coordinator's watches, for the "Not watched" hint. Re-read when the panel
 * opens, when the route key changes while open and when the coordinator is
 * switched. A response for an earlier request is dropped; while a re-read is in
 * flight or has failed the last loaded value of the same coordinator decides.
 * `onGone` reports a 404 for the coordinator.
 */
export function useCoordinatorWatches(params: {
  workspaceId: string;
  coordinatorId: string;
  routeKey: string;
  enabled: boolean;
  onGone: (coordinatorId: string) => void;
}): { loaded: boolean; watches: CoordinatorWatches | undefined } {
  const { workspaceId, coordinatorId, routeKey, enabled, onGone } = params;
  const [state, setState] = useState<Loaded | null>(null);
  const seqRef = useRef(0);
  const onGoneRef = useRef(onGone);
  onGoneRef.current = onGone;

  useEffect(() => {
    const seq = ++seqRef.current;
    if (!enabled) return;
    void (async () => {
      try {
        const res = await getCoordinator(workspaceId, coordinatorId);
        if (seqRef.current === seq) setState({ coordinatorId, watches: res.watches });
      } catch (error) {
        if (seqRef.current !== seq) return;
        if ((error as { status?: number } | null)?.status === 404) onGoneRef.current(coordinatorId);
      }
    })();
    return () => {
      seqRef.current += 1;
    };
  }, [workspaceId, coordinatorId, routeKey, enabled]);

  const current = state?.coordinatorId === coordinatorId ? state : null;
  return { loaded: current !== null, watches: current?.watches };
}
