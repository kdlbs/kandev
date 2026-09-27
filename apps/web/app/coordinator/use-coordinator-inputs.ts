"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useWebSocketClient } from "@/lib/ws/connection";
import {
  listCoordinatorStalls,
  listProposals,
  type Proposal,
  type Stall,
} from "@/lib/api/domains/coordinator-api";

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
 * viewed coordinator's pending proposals) for the Needs you / Queue screens.
 * Never holds tasks — those come from `useAllWorkflowSnapshots` via
 * `workspaceContextRead` (needs-you.md#inputs).
 *
 * One latest-request-wins sequence per input is shared across mount,
 * `retryFailed`, and `coordinator.updated` (Build decision 8): a newer read
 * for an input always wins over an older, still in-flight one for the same
 * input, and concurrent re-reads are never deduped or cancelled.
 */
export function useCoordinatorInputs(
  workspaceId: string | null,
  coordinatorId: string | null,
): UseCoordinatorInputsResult {
  const [stalls, setStalls] = useState<CoordinatorInputEntry<Stall[]>>(initialEntry);
  const [proposals, setProposals] = useState<CoordinatorInputEntry<Proposal[]>>(initialEntry);
  const stallsSeqRef = useRef(0);
  const proposalsSeqRef = useRef(0);
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

  const readProposals = useCallback((ws: string, coordinator: string) => {
    const seq = ++proposalsSeqRef.current;
    listProposals(ws, coordinator, "pending")
      .then((res) => {
        if (proposalsSeqRef.current !== seq) return;
        setProposals({ value: res.proposals, loadedAt: Date.now(), error: false });
      })
      .catch(() => {
        if (proposalsSeqRef.current !== seq) return;
        setProposals((prev) => ({ ...prev, error: true }));
      });
  }, []);

  useEffect(() => {
    setStalls(initialEntry);
    setProposals(initialEntry);
    if (!workspaceId || !coordinatorId) return;
    readStalls(workspaceId);
    readProposals(workspaceId, coordinatorId);
  }, [workspaceId, coordinatorId, readStalls, readProposals]);

  useEffect(() => {
    if (!wsClient || !workspaceId || !coordinatorId) return;
    return wsClient.on("coordinator.updated", (message) => {
      const payload = message.payload;
      if (payload.workspace_id !== workspaceId || payload.coordinator_id !== coordinatorId) return;
      readStalls(workspaceId);
      readProposals(workspaceId, coordinatorId);
    });
  }, [wsClient, workspaceId, coordinatorId, readStalls, readProposals]);

  const retryFailed = useCallback(() => {
    if (!workspaceId || !coordinatorId) return;
    if (stalls.error) readStalls(workspaceId);
    if (proposals.error) readProposals(workspaceId, coordinatorId);
  }, [workspaceId, coordinatorId, stalls.error, proposals.error, readStalls, readProposals]);

  return { stalls, proposals, retryFailed };
}
