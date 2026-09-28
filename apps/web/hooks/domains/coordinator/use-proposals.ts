"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { create } from "zustand";
import { ApiError } from "@/lib/api/client";
import { getProposal, listProposals, type Proposal } from "@/lib/api/domains/coordinator-api";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import { useWebSocketClient } from "@/lib/ws/connection";

const SETTLED_STATUSES = new Set(["approved", "rejected"]);
const OPEN_STATUSES = new Set(["pending", "approving", "failed"]);

export function isSettledProposal(proposal: Pick<Proposal, "status">): boolean {
  return SETTLED_STATUSES.has(proposal.status);
}

export function isOpenProposal(proposal: Pick<Proposal, "status">): boolean {
  return OPEN_STATUSES.has(proposal.status);
}

function updatedAtNs(proposal: Proposal): bigint {
  return parseStrictRfc3339Timestamp(proposal.updated_at) ?? BigInt(-1);
}

/**
 * Merges one incoming proposal row into the cache by the five rules in order
 * (docs/specs/coordinator/system-design/proposal-cards.md#client-store
 * "Merging one proposal"): no cached entry stores the incoming row; a
 * settled cached row is never replaced by an unsettled incoming one; a
 * later `updated_at` wins; an earlier one keeps the cached row; an equal
 * `updated_at` takes the incoming row only when it is settled and the
 * cached one is not. A row whose `updated_at` fails to parse sorts as older
 * than any parseable one.
 */
export function mergeProposal(cached: Proposal | undefined, incoming: Proposal): Proposal {
  if (!cached) return incoming;
  if (isSettledProposal(cached) && !isSettledProposal(incoming)) return cached;
  const cachedNs = updatedAtNs(cached);
  const incomingNs = updatedAtNs(incoming);
  if (incomingNs > cachedNs) return incoming;
  if (incomingNs < cachedNs) return cached;
  if (isSettledProposal(incoming) && !isSettledProposal(cached)) return incoming;
  return cached;
}

type CoordinatorProposalsState = {
  byId: Record<string, Proposal>;
  pendingLoadedAt: number | undefined;
  pendingError: boolean;
};

const INITIAL_COORDINATOR_PROPOSALS: CoordinatorProposalsState = {
  byId: {},
  pendingLoadedAt: undefined,
  pendingError: false,
};

type ProposalsStoreState = {
  byCoordinator: Record<string, CoordinatorProposalsState>;
  mergeOne: (coordinatorId: string, incoming: Proposal) => void;
  mergePendingList: (coordinatorId: string, proposals: Proposal[]) => void;
  setPendingError: (coordinatorId: string) => void;
  evict: (coordinatorId: string, id: string) => void;
};

/**
 * The proposal cache, keyed by coordinator id then proposal id
 * (docs/specs/coordinator/system-design/proposal-cards.md#client-store).
 * Deliberately not persisted: a reload starts from an empty cache, and
 * navigating between Needs you and Queue for one coordinator keeps its
 * entries because both routes read this same module-level store.
 */
export const useProposalsStore = create<ProposalsStoreState>()((set) => ({
  byCoordinator: {},
  mergeOne: (coordinatorId, incoming) =>
    set((state) => {
      const coordinator = state.byCoordinator[coordinatorId] ?? INITIAL_COORDINATOR_PROPOSALS;
      const merged = mergeProposal(coordinator.byId[incoming.id], incoming);
      return {
        byCoordinator: {
          ...state.byCoordinator,
          [coordinatorId]: { ...coordinator, byId: { ...coordinator.byId, [incoming.id]: merged } },
        },
      };
    }),
  mergePendingList: (coordinatorId, proposals) =>
    set((state) => {
      const coordinator = state.byCoordinator[coordinatorId] ?? INITIAL_COORDINATOR_PROPOSALS;
      const byId = { ...coordinator.byId };
      for (const incoming of proposals) {
        byId[incoming.id] = mergeProposal(byId[incoming.id], incoming);
      }
      return {
        byCoordinator: {
          ...state.byCoordinator,
          [coordinatorId]: { byId, pendingLoadedAt: Date.now(), pendingError: false },
        },
      };
    }),
  setPendingError: (coordinatorId) =>
    set((state) => {
      const coordinator = state.byCoordinator[coordinatorId] ?? INITIAL_COORDINATOR_PROPOSALS;
      return {
        byCoordinator: {
          ...state.byCoordinator,
          [coordinatorId]: { ...coordinator, pendingError: true },
        },
      };
    }),
  evict: (coordinatorId, id) =>
    set((state) => {
      const coordinator = state.byCoordinator[coordinatorId];
      if (!coordinator || !(id in coordinator.byId)) return state;
      const byId = { ...coordinator.byId };
      delete byId[id];
      return {
        byCoordinator: { ...state.byCoordinator, [coordinatorId]: { ...coordinator, byId } },
      };
    }),
}));

export type UseProposalsProposalsEntry = {
  value: Proposal[] | undefined;
  loadedAt: number | undefined;
  error: boolean;
};

export type UseProposalsResult = {
  proposals: UseProposalsProposalsEntry;
  retryFailed: () => void;
};

/**
 * Fetches every id the fresh pending-list response no longer contains but
 * that the cache still holds unsettled, and merges each by-id read the same
 * way (proposal-cards.md#client-store "Merging one proposal" / the by-id
 * backfill paragraph). A failed by-id read leaves the entry as-is; a 404
 * evicts it.
 */
function backfillDropped(workspaceId: string, coordinatorId: string, freshIds: Set<string>): void {
  const coordinator = useProposalsStore.getState().byCoordinator[coordinatorId];
  if (!coordinator) return;
  for (const [id, cached] of Object.entries(coordinator.byId)) {
    if (freshIds.has(id) || isSettledProposal(cached)) continue;
    getProposal(workspaceId, coordinatorId, id)
      .then((row) => useProposalsStore.getState().mergeOne(coordinatorId, row))
      .catch((error: unknown) => {
        if (error instanceof ApiError && error.status === 404) {
          useProposalsStore.getState().evict(coordinatorId, id);
        }
      });
  }
}

/**
 * Drives the `status=pending` read (mount, `retryFailed`, and
 * `coordinator.updated`) and the by-id backfill, and exposes the cached
 * open proposals (`pending`, `approving`, `failed`) as the input Needs you,
 * Queue and the toast count read (proposal-cards.md#client-store).
 */
export function useProposals(
  workspaceId: string | null,
  coordinatorId: string | null,
): UseProposalsResult {
  const store = useProposalsStore((state) =>
    coordinatorId ? state.byCoordinator[coordinatorId] : undefined,
  );
  const seqRef = useRef(0);
  const lastSucceededSeqRef = useRef(0);

  // Overlapping reads are not cancelled: every response is merged and the
  // merge rules decide which row wins (proposal-cards.md#client-store). The
  // sequence only keeps a failure older than a later success from raising
  // the input's error.
  const readPending = useCallback((ws: string, coordinator: string) => {
    const seq = ++seqRef.current;
    listProposals(ws, coordinator, "pending")
      .then((res) => {
        lastSucceededSeqRef.current = Math.max(lastSucceededSeqRef.current, seq);
        useProposalsStore.getState().mergePendingList(coordinator, res.proposals);
        backfillDropped(ws, coordinator, new Set(res.proposals.map((p) => p.id)));
      })
      .catch(() => {
        if (lastSucceededSeqRef.current > seq) return;
        useProposalsStore.getState().setPendingError(coordinator);
      });
  }, []);

  useEffect(() => {
    if (!workspaceId || !coordinatorId) return;
    readPending(workspaceId, coordinatorId);
  }, [workspaceId, coordinatorId, readPending]);

  const wsClient = useWebSocketClient();
  useEffect(() => {
    if (!wsClient || !workspaceId || !coordinatorId) return;
    return wsClient.on("coordinator.updated", (message) => {
      const payload = message.payload;
      if (payload.workspace_id !== workspaceId || payload.coordinator_id !== coordinatorId) return;
      readPending(workspaceId, coordinatorId);
    });
  }, [wsClient, workspaceId, coordinatorId, readPending]);

  const retryFailed = useCallback(() => {
    if (!workspaceId || !coordinatorId) return;
    if (store?.pendingError) readPending(workspaceId, coordinatorId);
  }, [workspaceId, coordinatorId, store?.pendingError, readPending]);

  const open =
    store?.pendingLoadedAt !== undefined
      ? Object.values(store.byId).filter(isOpenProposal)
      : undefined;

  return {
    proposals: {
      value: open,
      loadedAt: store?.pendingLoadedAt,
      error: store?.pendingError ?? false,
    },
    retryFailed,
  };
}

/**
 * Reads one proposal's full row from the cache by id, with no fetch of its
 * own: the Needs-you `ProposalCard` reads its row here because the
 * classification's proposals input is derived from the same store in the
 * same render, so a classified item always has its row
 * (proposal-cards.md#cards "Row source on Needs you").
 */
export function useProposalRow(
  coordinatorId: string | null,
  proposalId: string | null,
): Proposal | undefined {
  return useProposalsStore((state) =>
    coordinatorId && proposalId ? state.byCoordinator[coordinatorId]?.byId[proposalId] : undefined,
  );
}

export type UseProposalByIdResult = {
  proposal: Proposal | undefined;
  notFound: boolean;
};

/**
 * The chat transcript's `ProposalCard` fetches its own `proposal_id` by id
 * on mount and on every `coordinator.updated`, independent of the pending
 * list, so it can show a settled state after a reload with no other
 * proposal ever having been listed (proposal-cards.md#client-store).
 */
export function useProposalById(
  workspaceId: string | null,
  coordinatorId: string | null,
  proposalId: string | null,
): UseProposalByIdResult {
  const proposal = useProposalRow(coordinatorId, proposalId);
  const [notFound, setNotFound] = useState(false);
  const seqRef = useRef(0);
  const lastSucceededSeqRef = useRef(0);

  // Every response is merged whatever order it arrives in; the merge rules
  // decide which row wins (proposal-cards.md#client-store). A 404 older than
  // an already-succeeded read must not evict the row that read just merged.
  const read = useCallback((ws: string, coordinator: string, id: string) => {
    const seq = ++seqRef.current;
    getProposal(ws, coordinator, id)
      .then((row) => {
        lastSucceededSeqRef.current = Math.max(lastSucceededSeqRef.current, seq);
        useProposalsStore.getState().mergeOne(coordinator, row);
      })
      .catch((error: unknown) => {
        if (lastSucceededSeqRef.current > seq) return;
        if (error instanceof ApiError && error.status === 404) {
          useProposalsStore.getState().evict(coordinator, id);
          setNotFound(true);
        }
      });
  }, []);

  useEffect(() => {
    setNotFound(false);
    if (!workspaceId || !coordinatorId || !proposalId) return;
    read(workspaceId, coordinatorId, proposalId);
  }, [workspaceId, coordinatorId, proposalId, read]);

  const wsClient = useWebSocketClient();
  useEffect(() => {
    if (!wsClient || !workspaceId || !coordinatorId || !proposalId) return;
    return wsClient.on("coordinator.updated", (message) => {
      const payload = message.payload;
      if (payload.workspace_id !== workspaceId || payload.coordinator_id !== coordinatorId) return;
      read(workspaceId, coordinatorId, proposalId);
    });
  }, [wsClient, workspaceId, coordinatorId, proposalId, read]);

  return { proposal, notFound };
}
