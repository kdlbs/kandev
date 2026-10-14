"use client";

import { useCallback, useRef, useState } from "react";
import { putCoordinatorPause } from "@/lib/api/domains/coordinator-autonomy-api";
import type { AutonomyInput } from "./use-autonomy";

export type PauseControlState = {
  pending: boolean;
  failed: boolean;
  setPaused: (paused: boolean) => void;
};

/**
 * Pause or Resume one coordinator. The request counts as a read issued at the
 * click: its response is applied only while no later read has been issued, and
 * a failure re-reads so the view shows the latest state.
 */
export function usePauseControl(
  workspaceId: string,
  coordinatorId: string,
  autonomy: AutonomyInput,
): PauseControlState {
  const [pending, setPending] = useState(false);
  const [failed, setFailed] = useState(false);
  const inFlight = useRef(false);
  const { beginWrite, applyWrite, retry } = autonomy;

  const setPaused = useCallback(
    (paused: boolean) => {
      if (inFlight.current || !beginWrite || !applyWrite) return;
      inFlight.current = true;
      setPending(true);
      setFailed(false);
      const token = beginWrite();
      putCoordinatorPause(workspaceId, coordinatorId, paused)
        .then(
          (response) =>
            applyWrite(token, {
              paused: response.paused ?? paused,
              paused_at: response.paused_at ?? null,
              paused_by: response.paused_by ?? null,
            }),
          () => {
            setFailed(true);
            retry();
          },
        )
        .finally(() => {
          inFlight.current = false;
          setPending(false);
        });
    },
    [workspaceId, coordinatorId, beginWrite, applyWrite, retry],
  );
  return { pending, failed, setPaused };
}
