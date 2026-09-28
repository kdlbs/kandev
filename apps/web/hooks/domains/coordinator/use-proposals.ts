"use client";

import { useCallback, useEffect, useRef } from "react";
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
  /** Highest ticket applied (success or not-found) for each id, gating a stale duplicate. */
  appliedSeq: Record<string, number>;
  /** Ticket of the latest applied not-found for each id, present iff that id currently reads as not found. */
  tombstoneSeq: Record<string, number>;
  /** Monotonic ticket counter shared by every read/write path for this coordinator. */
  nextSeq: number;
  pendingLoadedAt: number | undefined;
  pendingError: boolean;
};

const INITIAL_COORDINATOR_PROPOSALS: CoordinatorProposalsState = {
  byId: {},
  appliedSeq: {},
  tombstoneSeq: {},
  nextSeq: 1,
  pendingLoadedAt: undefined,
  pendingError: false,
};

export type ProposalApplyResult = { kind: "success"; proposal: Proposal } | { kind: "not_found" };

/**
 * Applies one ticketed response for one proposal id
 * (docs/specs/coordinator/system-design/proposal-cards.md#client-store
 * "Per-id ticket ordering"). Every path that reads or writes a proposal id -
 * `useProposalById`, `backfillDropped`, `readPending`'s per-row merge, a
 * decision's response, and a `coordinator.updated` re-read - funnels through
 * here with a ticket from the same per-coordinator counter, so exactly one
 * mechanism decides which response wins regardless of arrival order:
 * - A not-found response is applied only if its ticket is the newest
 *   applied-or-issued for that id; an older one is a stale duplicate and is
 *   ignored. Applying one sets a tombstone at its ticket and evicts the row.
 * - A success is only ever shadowed by a tombstone with a *newer* ticket
 *   (there is no `updated_at` to arbitrate a not-found by); otherwise it
 *   always merges via `mergeProposal`, so success-vs-success stays
 *   content-arbitrated and ticket-order-independent. A success that lands
 *   the newest ticket for its id clears an older tombstone.
 */
function applyProposalToCoordinator(
  coordinator: CoordinatorProposalsState,
  id: string,
  seq: number,
  result: ProposalApplyResult,
): CoordinatorProposalsState {
  const applied = coordinator.appliedSeq[id] ?? 0;
  const tombstone = coordinator.tombstoneSeq[id];

  if (result.kind === "not_found") {
    if (seq < applied) return coordinator;
    const byId = { ...coordinator.byId };
    delete byId[id];
    return {
      ...coordinator,
      byId,
      appliedSeq: { ...coordinator.appliedSeq, [id]: seq },
      tombstoneSeq: { ...coordinator.tombstoneSeq, [id]: seq },
    };
  }

  if (tombstone !== undefined && tombstone > seq) return coordinator;
  const byId = { ...coordinator.byId, [id]: mergeProposal(coordinator.byId[id], result.proposal) };
  const tombstoneSeq = coordinator.tombstoneSeq;
  const clearedTombstoneSeq =
    tombstone === undefined
      ? tombstoneSeq
      : Object.fromEntries(Object.entries(tombstoneSeq).filter(([key]) => key !== id));
  return {
    ...coordinator,
    byId,
    appliedSeq: { ...coordinator.appliedSeq, [id]: Math.max(applied, seq) },
    tombstoneSeq: clearedTombstoneSeq,
  };
}

type ProposalsStoreState = {
  byCoordinator: Record<string, CoordinatorProposalsState>;
  takeProposalTicket: (coordinatorId: string) => number;
  applyProposalResult: (
    coordinatorId: string,
    id: string,
    seq: number,
    result: ProposalApplyResult,
  ) => void;
  mergePendingRows: (coordinatorId: string, seq: number, proposals: Proposal[]) => void;
  setPendingSettled: (coordinatorId: string) => void;
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
  takeProposalTicket: (coordinatorId) => {
    let seq = 0;
    set((state) => {
      const coordinator = state.byCoordinator[coordinatorId] ?? INITIAL_COORDINATOR_PROPOSALS;
      seq = coordinator.nextSeq;
      return {
        byCoordinator: {
          ...state.byCoordinator,
          [coordinatorId]: { ...coordinator, nextSeq: coordinator.nextSeq + 1 },
        },
      };
    });
    return seq;
  },
  applyProposalResult: (coordinatorId, id, seq, result) =>
    set((state) => {
      const coordinator = state.byCoordinator[coordinatorId] ?? INITIAL_COORDINATOR_PROPOSALS;
      const updated = applyProposalToCoordinator(coordinator, id, seq, result);
      if (updated === coordinator) return state;
      return { byCoordinator: { ...state.byCoordinator, [coordinatorId]: updated } };
    }),
  mergePendingRows: (coordinatorId, seq, proposals) =>
    set((state) => {
      let coordinator = state.byCoordinator[coordinatorId] ?? INITIAL_COORDINATOR_PROPOSALS;
      for (const incoming of proposals) {
        coordinator = applyProposalToCoordinator(coordinator, incoming.id, seq, {
          kind: "success",
          proposal: incoming,
        });
      }
      return { byCoordinator: { ...state.byCoordinator, [coordinatorId]: coordinator } };
    }),
  setPendingSettled: (coordinatorId) =>
    set((state) => {
      const coordinator = state.byCoordinator[coordinatorId] ?? INITIAL_COORDINATOR_PROPOSALS;
      return {
        byCoordinator: {
          ...state.byCoordinator,
          [coordinatorId]: { ...coordinator, pendingLoadedAt: Date.now(), pendingError: false },
        },
      };
    }),
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
          [coordinatorId]: {
            ...coordinator,
            byId,
            pendingLoadedAt: Date.now(),
            pendingError: false,
          },
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
 * that the cache still holds unsettled, and applies each by-id read through
 * the same per-id ticket rule as every other path
 * (proposal-cards.md#client-store "Per-id ticket ordering" / the by-id
 * backfill paragraph). A failed by-id read leaves the entry as-is; a 404
 * applies a tombstone that no older success can undo.
 */
function backfillDropped(workspaceId: string, coordinatorId: string, freshIds: Set<string>): void {
  const coordinator = useProposalsStore.getState().byCoordinator[coordinatorId];
  if (!coordinator) return;
  for (const [id, cached] of Object.entries(coordinator.byId)) {
    if (freshIds.has(id) || isSettledProposal(cached)) continue;
    const seq = useProposalsStore.getState().takeProposalTicket(coordinatorId);
    getProposal(workspaceId, coordinatorId, id)
      .then((row) =>
        useProposalsStore
          .getState()
          .applyProposalResult(coordinatorId, id, seq, { kind: "success", proposal: row }),
      )
      .catch((error: unknown) => {
        if (error instanceof ApiError && error.status === 404) {
          useProposalsStore
            .getState()
            .applyProposalResult(coordinatorId, id, seq, { kind: "not_found" });
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
  const listSeqRef = useRef(0);
  const lastAppliedListSeqRef = useRef(0);

  // Overlapping reads are not cancelled. Each row's own merge always goes
  // through the store's per-id ticket rule (proposal-cards.md#client-store
  // "Per-id ticket ordering"), so row content is safe regardless of arrival
  // order. `listSeqRef`/`lastAppliedListSeqRef` guard only the list-level
  // `pendingLoadedAt`/`pendingError` bookkeeping, symmetrically for both the
  // success and failure branches, so a stale response of either kind can
  // never clobber a state a newer one already applied.
  const readPending = useCallback((ws: string, coordinator: string) => {
    const listSeq = ++listSeqRef.current;
    const seq = useProposalsStore.getState().takeProposalTicket(coordinator);
    listProposals(ws, coordinator, "pending")
      .then((res) => {
        useProposalsStore.getState().mergePendingRows(coordinator, seq, res.proposals);
        backfillDropped(ws, coordinator, new Set(res.proposals.map((p) => p.id)));
        if (listSeq < lastAppliedListSeqRef.current) return;
        lastAppliedListSeqRef.current = listSeq;
        useProposalsStore.getState().setPendingSettled(coordinator);
      })
      .catch(() => {
        if (listSeq < lastAppliedListSeqRef.current) return;
        lastAppliedListSeqRef.current = listSeq;
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
  const notFound = useProposalsStore((state) =>
    coordinatorId && proposalId
      ? (state.byCoordinator[coordinatorId]?.tombstoneSeq[proposalId] ?? undefined) !== undefined
      : false,
  );

  // Every response is applied through the store's per-id ticket rule
  // (proposal-cards.md#client-store "Per-id ticket ordering"), so an older
  // response - whether a success or a 404 - can never override state a
  // newer one already applied, in either direction. `notFound` is derived
  // straight from the store's tombstone for this id rather than local hook
  // state, so a response for an id this hook has since moved past can never
  // touch it, with no separate id-tracking ref needed.
  const read = useCallback((ws: string, coordinator: string, id: string) => {
    const seq = useProposalsStore.getState().takeProposalTicket(coordinator);
    getProposal(ws, coordinator, id)
      .then((row) =>
        useProposalsStore
          .getState()
          .applyProposalResult(coordinator, id, seq, { kind: "success", proposal: row }),
      )
      .catch((error: unknown) => {
        if (!(error instanceof ApiError) || error.status !== 404) return;
        useProposalsStore
          .getState()
          .applyProposalResult(coordinator, id, seq, { kind: "not_found" });
      });
  }, []);

  useEffect(() => {
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
