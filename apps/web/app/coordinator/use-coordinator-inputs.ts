"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useWebSocketClient } from "@/lib/ws/connection";
import {
  listCoordinatorStalls,
  type Proposal,
  type Stall,
} from "@/lib/api/domains/coordinator-api";
import { useProposals } from "@/hooks/domains/coordinator/use-proposals";

/**
 * One screen input's last-known state (docs/specs/coordinator/system-design/
 * needs-you.md#failure-and-recovery). `value` is the last successful
 * response (absent before the first success); `loadedAt` is when it was
 * received; `error` is set while the latest read for this input has failed
 * (the previous `value`/`loadedAt` are kept, not cleared).
 */
export type CoordinatorInputEntry<T> = {
  value: T | undefined;
  loadedAt: number | undefined;
  error: boolean;
};

function initialEntry<T>(): CoordinatorInputEntry<T> {
  return { value: undefined, loadedAt: undefined, error: false };
}

export type UseCoordinatorInputsResult = {
  stalls: CoordinatorInputEntry<Stall[]>;
  proposals: CoordinatorInputEntry<Proposal[]>;
  /** Re-issues only the reads currently in an error state, in parallel. */
  retryFailed: () => void;
};

/**
 * Holds the two coordinator-owned screen inputs (stall records and the
 * viewed coordinator's open proposals) for the Needs you / Queue screens.
 * Never holds tasks — those come from `useAllWorkflowSnapshots` via
 * `workspaceContextRead` (needs-you.md#inputs).
 *
 * Proposals come from the shared `use-proposals.ts` store
 * (docs/specs/coordinator/system-design/proposal-cards.md#client-store), so
 * Needs you, Queue and the toast count read one cache instead of each
 * issuing their own `listProposals` call. Stalls keep their own
 * one-latest-request-wins sequence (Build decision 8): a newer read always
 * wins over an older, still in-flight one, and concurrent re-reads are never
 * deduped or cancelled.
 */
export function useCoordinatorInputs(
  workspaceId: string | null,
  coordinatorId: string | null,
): UseCoordinatorInputsResult {
  const [stalls, setStalls] = useState<CoordinatorInputEntry<Stall[]>>(initialEntry);
  const stallsSeqRef = useRef(0);
  const wsClient = useWebSocketClient();

  const readStalls = useCallback((ws: string) => {
    const seq = ++stallsSeqRef.current;
    listCoordinatorStalls(ws)
      .then((res) => {
        if (stallsSeqRef.current !== seq) return;
        setStalls({ value: res.stalls, loadedAt: Date.now(), error: false });
      })
      .catch(() => {
        if (stallsSeqRef.current !== seq) return;
        setStalls((prev) => ({ ...prev, error: true }));
      });
  }, []);

  useEffect(() => {
    setStalls(initialEntry);
    if (!workspaceId || !coordinatorId) return;
    readStalls(workspaceId);
  }, [workspaceId, coordinatorId, readStalls]);

  useEffect(() => {
    if (!wsClient || !workspaceId || !coordinatorId) return;
    return wsClient.on("coordinator.updated", (message) => {
      const payload = message.payload;
      if (payload.workspace_id !== workspaceId || payload.coordinator_id !== coordinatorId) return;
      readStalls(workspaceId);
    });
  }, [wsClient, workspaceId, coordinatorId, readStalls]);

  const { proposals, retryFailed: retryProposals } = useProposals(workspaceId, coordinatorId);

  const retryFailed = useCallback(() => {
    if (!workspaceId || !coordinatorId) return;
    if (stalls.error) readStalls(workspaceId);
    retryProposals();
  }, [workspaceId, coordinatorId, stalls.error, readStalls, retryProposals]);

  return { stalls, proposals, retryFailed };
}
