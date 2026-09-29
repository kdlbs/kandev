/* eslint-disable max-lines -- one file holds the store's cross-path interleaving matrix for every proposal kind */
import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Proposal } from "@/lib/api/domains/coordinator-api";
import { ApiError } from "@/lib/api/client";

const listProposalsMock = vi.fn();
const getProposalMock = vi.fn();
const approveProposalMock = vi.fn();
const rejectProposalMock = vi.fn();

vi.mock("@/lib/api/domains/coordinator-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/coordinator-api")>();
  return {
    ...actual,
    listProposals: (...args: unknown[]) => listProposalsMock(...args),
    getProposal: (...args: unknown[]) => getProposalMock(...args),
    approveProposal: (...args: unknown[]) => approveProposalMock(...args),
    rejectProposal: (...args: unknown[]) => rejectProposalMock(...args),
  };
});

const clients = vi.hoisted(() => ({ active: undefined as unknown }));

vi.mock("@/lib/ws/connection", () => ({
  useWebSocketClient: () => clients.active,
}));

// Import after mocks so the hooks pick up the mocked modules.
import {
  mergeProposal,
  useProposals,
  useProposalById,
  useProposalRow,
  useProposalsStore,
} from "./use-proposals";
import { useProposalDecision } from "./use-proposal-decision";

const WORKSPACE_ID = "workspace-1";
const COORDINATOR_ID = "coordinator-1";
const T0 = "2026-09-27T00:00:00Z";
const T5 = "2026-09-27T00:05:00Z";
const T10 = "2026-09-27T00:10:00Z";

function proposal(overrides: Partial<Proposal> & { id: string }): Proposal {
  return {
    coordinator_id: COORDINATOR_ID,
    workspace_id: WORKSPACE_ID,
    status: "pending",
    spec: {
      title: "t",
      description: "d",
      rationale: "r",
      workflow_id: "wf",
      step_id: "step",
      repository_id: "repo",
      source_task_id: "task",
    },
    final_spec: null,
    claimed_at: null,
    task_id: null,
    error: null,
    reject_reason: null,
    decided_by: null,
    created_at: T0,
    updated_at: T0,
    ...overrides,
  };
}

/** Seeds the store as a real caller would: through a ticket, not a shortcut around it. */
function seedProposal(coordinatorId: string, incoming: Proposal): void {
  const store = useProposalsStore.getState();
  const seq = store.takeProposalTicket(coordinatorId);
  store.applyProposalResult(coordinatorId, incoming.id, seq, {
    kind: "success",
    proposal: incoming,
  });
}

type UpdatedHandler = (message: { payload: Record<string, unknown> }) => void;

function makeWsClient() {
  return {
    on: vi.fn((_type: string, _handler: UpdatedHandler) => vi.fn()),
  };
}

beforeEach(() => {
  listProposalsMock.mockReset();
  getProposalMock.mockReset();
  approveProposalMock.mockReset();
  rejectProposalMock.mockReset();
  clients.active = undefined;
  useProposalsStore.setState({ byCoordinator: {} });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("mergeProposal", () => {
  it("stores the incoming row when nothing is cached", () => {
    const incoming = proposal({ id: "p-1" });
    expect(mergeProposal(undefined, incoming)).toBe(incoming);
  });

  it("keeps a settled cached row when the incoming row is unsettled", () => {
    const cached = proposal({ id: "p-1", status: "approved", updated_at: T0 });
    const incoming = proposal({ id: "p-1", status: "pending", updated_at: T5 });
    expect(mergeProposal(cached, incoming)).toBe(cached);
  });

  it("keeps a settled cached row against a late unsettled approving response", () => {
    const cached = proposal({ id: "p-1", status: "rejected", updated_at: T0 });
    const incoming = proposal({ id: "p-1", status: "approving", updated_at: T10 });
    expect(mergeProposal(cached, incoming)).toBe(cached);
  });

  it("takes the incoming row when its updated_at is later", () => {
    const cached = proposal({ id: "p-1", status: "pending", updated_at: T0 });
    const incoming = proposal({ id: "p-1", status: "approving", updated_at: T5 });
    expect(mergeProposal(cached, incoming)).toBe(incoming);
  });

  it("keeps the cached row when the incoming updated_at is earlier", () => {
    const cached = proposal({ id: "p-1", status: "approving", updated_at: T5 });
    const incoming = proposal({ id: "p-1", status: "pending", updated_at: T0 });
    expect(mergeProposal(cached, incoming)).toBe(cached);
  });

  it("on an equal updated_at, takes the incoming row only when it is settled and the cached one is not", () => {
    const cached = proposal({ id: "p-1", status: "approving", updated_at: T5 });
    const incoming = proposal({ id: "p-1", status: "approved", updated_at: T5 });
    expect(mergeProposal(cached, incoming)).toBe(incoming);
  });

  it("on an equal updated_at, keeps the cached row when neither or both are settled", () => {
    const cached = proposal({ id: "p-1", status: "pending", updated_at: T5 });
    const incoming = proposal({ id: "p-1", status: "approving", updated_at: T5 });
    expect(mergeProposal(cached, incoming)).toBe(cached);
  });
});

describe("useProposals - initial load", () => {
  it("loads the pending list on mount and exposes only-open rows", async () => {
    listProposalsMock.mockResolvedValue({ proposals: [proposal({ id: "p-1" })] });

    const { result } = renderHook(() => useProposals(WORKSPACE_ID, COORDINATOR_ID));

    await waitFor(() => expect(result.current.proposals.value).toBeDefined());
    expect(result.current.proposals.value).toEqual([proposal({ id: "p-1" })]);
    expect(result.current.proposals.error).toBe(false);
    expect(listProposalsMock).toHaveBeenCalledWith(WORKSPACE_ID, COORDINATOR_ID, "pending");
  });

  it("does not fetch when workspaceId or coordinatorId is null", () => {
    renderHook(() => useProposals(null, null));
    expect(listProposalsMock).not.toHaveBeenCalled();
  });
});

describe("useProposals - read failures", () => {
  it("a failed pending read keeps the cache, sets error, and skips backfill", async () => {
    listProposalsMock.mockResolvedValueOnce({
      proposals: [proposal({ id: "p-1", status: "approving" })],
    });
    const wsClient = makeWsClient();
    clients.active = wsClient;

    const { result } = renderHook(() => useProposals(WORKSPACE_ID, COORDINATOR_ID));
    await waitFor(() => expect(result.current.proposals.value).toHaveLength(1));

    const handler = wsClient.on.mock.calls[0]?.[1] as UpdatedHandler;
    listProposalsMock.mockRejectedValueOnce(new Error("network"));

    act(() => {
      handler({
        payload: { workspace_id: WORKSPACE_ID, coordinator_id: COORDINATOR_ID, open_proposals: 1 },
      });
    });

    await waitFor(() => expect(result.current.proposals.error).toBe(true));
    expect(result.current.proposals.value).toEqual([proposal({ id: "p-1", status: "approving" })]);
    expect(getProposalMock).not.toHaveBeenCalled();
  });

  it("retryFailed re-issues the pending read", async () => {
    listProposalsMock.mockRejectedValueOnce(new Error("network"));
    const { result } = renderHook(() => useProposals(WORKSPACE_ID, COORDINATOR_ID));
    await waitFor(() => expect(result.current.proposals.error).toBe(true));

    listProposalsMock.mockResolvedValueOnce({ proposals: [proposal({ id: "p-2" })] });
    act(() => {
      result.current.retryFailed();
    });

    await waitFor(() => expect(result.current.proposals.value).toEqual([proposal({ id: "p-2" })]));
  });
});

describe("useProposals - backfill", () => {
  it("fetches by id every cached id the fresh list no longer contains, when not settled", async () => {
    listProposalsMock.mockResolvedValueOnce({
      proposals: [
        proposal({ id: "p-1", status: "pending" }),
        proposal({ id: "p-2", status: "approved" }),
      ],
    });
    const wsClient = makeWsClient();
    clients.active = wsClient;

    const { result } = renderHook(() => useProposals(WORKSPACE_ID, COORDINATOR_ID));
    // p-2 is already settled (approved) in the seeded cache, so only p-1
    // (pending) is an open row; p-2 is present in the cache but excluded
    // from `value`, which exposes only-open rows.
    await waitFor(() => expect(result.current.proposals.value).toHaveLength(1));
    expect(useProposalsStore.getState().byCoordinator[COORDINATOR_ID]?.byId["p-2"]).toBeDefined();

    const handler = wsClient.on.mock.calls[0]?.[1] as UpdatedHandler;
    // p-1 settles (approved) and drops off the pending list; p-2 (already
    // settled) also drops off but must be skipped by the backfill.
    listProposalsMock.mockResolvedValueOnce({ proposals: [] });
    getProposalMock.mockResolvedValueOnce(proposal({ id: "p-1", status: "approved" }));

    act(() => {
      handler({
        payload: { workspace_id: WORKSPACE_ID, coordinator_id: COORDINATOR_ID, open_proposals: 0 },
      });
    });

    await waitFor(() => expect(getProposalMock).toHaveBeenCalledTimes(1));
    expect(getProposalMock).toHaveBeenCalledWith(WORKSPACE_ID, COORDINATOR_ID, "p-1");
  });

  it("a failed by-id read keeps the entry", async () => {
    listProposalsMock.mockResolvedValueOnce({
      proposals: [proposal({ id: "p-1", status: "pending" })],
    });
    const wsClient = makeWsClient();
    clients.active = wsClient;
    renderHook(() => useProposals(WORKSPACE_ID, COORDINATOR_ID));
    await waitFor(() => expect(listProposalsMock).toHaveBeenCalledTimes(1));

    const handler = wsClient.on.mock.calls[0]?.[1] as UpdatedHandler;
    listProposalsMock.mockResolvedValueOnce({ proposals: [] });
    getProposalMock.mockRejectedValueOnce(new Error("network"));

    act(() => {
      handler({
        payload: { workspace_id: WORKSPACE_ID, coordinator_id: COORDINATOR_ID, open_proposals: 0 },
      });
    });

    await waitFor(() => expect(getProposalMock).toHaveBeenCalledTimes(1));
    expect(useProposalsStore.getState().byCoordinator[COORDINATOR_ID]?.byId["p-1"]).toEqual(
      proposal({ id: "p-1", status: "pending" }),
    );
  });

  it("a by-id 404 evicts the entry", async () => {
    listProposalsMock.mockResolvedValueOnce({
      proposals: [proposal({ id: "p-1", status: "pending" })],
    });
    const wsClient = makeWsClient();
    clients.active = wsClient;
    renderHook(() => useProposals(WORKSPACE_ID, COORDINATOR_ID));
    await waitFor(() => expect(listProposalsMock).toHaveBeenCalledTimes(1));

    const handler = wsClient.on.mock.calls[0]?.[1] as UpdatedHandler;
    listProposalsMock.mockResolvedValueOnce({ proposals: [] });
    getProposalMock.mockRejectedValueOnce(new ApiError("not found", 404, {}));

    act(() => {
      handler({
        payload: { workspace_id: WORKSPACE_ID, coordinator_id: COORDINATOR_ID, open_proposals: 0 },
      });
    });

    await waitFor(() =>
      expect(
        useProposalsStore.getState().byCoordinator[COORDINATOR_ID]?.byId["p-1"],
      ).toBeUndefined(),
    );
  });
});

describe("useProposalRow", () => {
  it("reads the full row from the store by id, independent of the open filter", () => {
    useProposalsStore.setState({
      byCoordinator: {
        [COORDINATOR_ID]: {
          byId: { "p-1": proposal({ id: "p-1", status: "approving" }) },
          appliedSeq: {},
          tombstoneSeq: {},
          nextSeq: 1,
          pendingLoadedAt: 1,
          pendingError: false,
        },
      },
    });
    const { result } = renderHook(() => useProposalRow(COORDINATOR_ID, "p-1"));
    expect(result.current).toEqual(proposal({ id: "p-1", status: "approving" }));
  });

  it("returns undefined for an id not in the store", () => {
    const { result } = renderHook(() => useProposalRow(COORDINATOR_ID, "missing"));
    expect(result.current).toBeUndefined();
  });
});

describe("useProposalById", () => {
  it("fetches its own id on mount, independent of the pending list", async () => {
    getProposalMock.mockResolvedValueOnce(proposal({ id: "p-1", status: "approved" }));

    const { result } = renderHook(() => useProposalById(WORKSPACE_ID, COORDINATOR_ID, "p-1"));

    expect(result.current.proposal).toBeUndefined();
    await waitFor(() => expect(result.current.proposal).toBeDefined());
    expect(result.current.proposal).toEqual(proposal({ id: "p-1", status: "approved" }));
    expect(result.current.notFound).toBe(false);
    expect(listProposalsMock).not.toHaveBeenCalled();
  });

  it("renders not-found after a 404 and does not keep a stale row", async () => {
    getProposalMock.mockRejectedValueOnce(new ApiError("not found", 404, {}));

    const { result } = renderHook(() => useProposalById(WORKSPACE_ID, COORDINATOR_ID, "p-1"));

    await waitFor(() => expect(result.current.notFound).toBe(true));
    expect(result.current.proposal).toBeUndefined();
  });

  it("keeps the last row on a failed read and retries on the next coordinator.updated", async () => {
    getProposalMock.mockResolvedValueOnce(proposal({ id: "p-1", status: "pending" }));
    const wsClient = makeWsClient();
    clients.active = wsClient;

    const { result } = renderHook(() => useProposalById(WORKSPACE_ID, COORDINATOR_ID, "p-1"));
    await waitFor(() => expect(result.current.proposal).toBeDefined());

    const handler = wsClient.on.mock.calls[0]?.[1] as UpdatedHandler;
    getProposalMock.mockRejectedValueOnce(new Error("network"));

    act(() => {
      handler({
        payload: { workspace_id: WORKSPACE_ID, coordinator_id: COORDINATOR_ID, open_proposals: 0 },
      });
    });

    await waitFor(() => expect(getProposalMock).toHaveBeenCalledTimes(2));
    expect(result.current.proposal).toEqual(proposal({ id: "p-1", status: "pending" }));

    getProposalMock.mockResolvedValueOnce(proposal({ id: "p-1", status: "approved" }));
    act(() => {
      handler({
        payload: { workspace_id: WORKSPACE_ID, coordinator_id: COORDINATOR_ID, open_proposals: 0 },
      });
    });
    await waitFor(() =>
      expect(result.current.proposal).toEqual(proposal({ id: "p-1", status: "approved" })),
    );
  });
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const UPDATED_PAYLOAD = {
  payload: { workspace_id: WORKSPACE_ID, coordinator_id: COORDINATOR_ID, open_proposals: 1 },
};

describe("overlapping reads - pending list", () => {
  it("merges an earlier-issued pending read that answers last, letting the merge rules decide", async () => {
    const first = deferred<{ proposals: Proposal[] }>();
    const second = deferred<{ proposals: Proposal[] }>();
    listProposalsMock.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const wsClient = makeWsClient();
    clients.active = wsClient;

    const { result } = renderHook(() => useProposals(WORKSPACE_ID, COORDINATOR_ID));
    const handler = wsClient.on.mock.calls[0]?.[1] as UpdatedHandler;
    act(() => handler(UPDATED_PAYLOAD));

    await act(async () => {
      second.resolve({ proposals: [proposal({ id: "p-1", status: "pending", updated_at: T0 })] });
    });
    await act(async () => {
      first.resolve({ proposals: [proposal({ id: "p-1", status: "approving", updated_at: T5 })] });
    });

    await waitFor(() => expect(result.current.proposals.value?.[0]?.status).toBe("approving"));
  });

  it("does not let an older failed pending read set the error after a newer read succeeded", async () => {
    const first = deferred<{ proposals: Proposal[] }>();
    listProposalsMock
      .mockReturnValueOnce(first.promise)
      .mockResolvedValueOnce({ proposals: [proposal({ id: "p-1" })] });
    const wsClient = makeWsClient();
    clients.active = wsClient;

    const { result } = renderHook(() => useProposals(WORKSPACE_ID, COORDINATOR_ID));
    const handler = wsClient.on.mock.calls[0]?.[1] as UpdatedHandler;
    act(() => handler(UPDATED_PAYLOAD));
    await waitFor(() => expect(result.current.proposals.value).toHaveLength(1));

    await act(async () => {
      first.reject(new Error("network"));
    });
    expect(result.current.proposals.error).toBe(false);
  });
});

describe("overlapping reads - by id", () => {
  it("merges an earlier-issued by-id read that answers last", async () => {
    const first = deferred<Proposal>();
    const second = deferred<Proposal>();
    getProposalMock.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const wsClient = makeWsClient();
    clients.active = wsClient;

    const { result } = renderHook(() => useProposalById(WORKSPACE_ID, COORDINATOR_ID, "p-1"));
    const handler = wsClient.on.mock.calls[0]?.[1] as UpdatedHandler;
    act(() => handler(UPDATED_PAYLOAD));

    await act(async () => {
      second.resolve(proposal({ id: "p-1", status: "pending", updated_at: T0 }));
    });
    await act(async () => {
      first.resolve(proposal({ id: "p-1", status: "approved", updated_at: T5 }));
    });

    await waitFor(() => expect(result.current.proposal?.status).toBe("approved"));
  });

  it("success-then-404: does not let an older 404 by-id read evict a row merged by a newer read", async () => {
    const first = deferred<Proposal>();
    getProposalMock
      .mockReturnValueOnce(first.promise)
      .mockResolvedValueOnce(proposal({ id: "p-1", status: "approved" }));
    const wsClient = makeWsClient();
    clients.active = wsClient;

    const { result } = renderHook(() => useProposalById(WORKSPACE_ID, COORDINATOR_ID, "p-1"));
    const handler = wsClient.on.mock.calls[0]?.[1] as UpdatedHandler;
    act(() => handler(UPDATED_PAYLOAD));
    await waitFor(() => expect(result.current.proposal?.status).toBe("approved"));

    await act(async () => {
      first.reject(new ApiError("not found", 404, {}));
    });

    expect(result.current.notFound).toBe(false);
    expect(result.current.proposal?.status).toBe("approved");
  });

  it("404-then-success: does not let a stale success clear notFound after a newer 404 already set it", async () => {
    const first = deferred<Proposal>();
    getProposalMock
      .mockReturnValueOnce(first.promise)
      .mockRejectedValueOnce(new ApiError("not found", 404, {}));
    const wsClient = makeWsClient();
    clients.active = wsClient;

    const { result } = renderHook(() => useProposalById(WORKSPACE_ID, COORDINATOR_ID, "p-1"));
    const handler = wsClient.on.mock.calls[0]?.[1] as UpdatedHandler;
    act(() => handler(UPDATED_PAYLOAD));
    await waitFor(() => expect(result.current.notFound).toBe(true));

    // The earlier-issued (now stale) request finally resolves after the
    // later-issued request's 404 already decided notFound. Since it is
    // older, it must not flip notFound back - only a fresher read can.
    await act(async () => {
      first.resolve(proposal({ id: "p-1", status: "approved" }));
    });

    expect(result.current.notFound).toBe(true);
  });

  it("CR-101: does not let a stale response for a previous id set notFound for the current id", async () => {
    const firstForP1 = deferred<Proposal>();
    getProposalMock.mockReturnValueOnce(firstForP1.promise);
    const wsClient = makeWsClient();
    clients.active = wsClient;

    const { result, rerender } = renderHook(
      ({ proposalId }: { proposalId: string }) =>
        useProposalById(WORKSPACE_ID, COORDINATOR_ID, proposalId),
      { initialProps: { proposalId: "p-1" } },
    );

    getProposalMock.mockResolvedValueOnce(proposal({ id: "p-2", status: "pending" }));
    rerender({ proposalId: "p-2" });
    await waitFor(() => expect(result.current.proposal?.id).toBe("p-2"));

    // p-1's read, issued before the hook moved on to p-2, finally settles
    // with a 404. It must not touch the notFound flag now driving p-2.
    await act(async () => {
      firstForP1.reject(new ApiError("not found", 404, {}));
    });

    expect(result.current.notFound).toBe(false);
    expect(result.current.proposal?.id).toBe("p-2");
  });
});

describe("proposal store - per-id ticket ordering", () => {
  const PROPOSAL_ID = "p-1";

  type Event =
    | { kind: "success"; updatedAt: string; status?: Proposal["status"] }
    | { kind: "not_found" };

  function apply(seq: number, event: Event): void {
    const result =
      event.kind === "success"
        ? {
            kind: "success" as const,
            proposal: proposal({
              id: PROPOSAL_ID,
              status: event.status ?? "pending",
              updated_at: event.updatedAt,
            }),
          }
        : { kind: "not_found" as const };
    useProposalsStore.getState().applyProposalResult(COORDINATOR_ID, PROPOSAL_ID, seq, result);
  }

  it.each([
    {
      name: "success (issued first) then not_found (issued second): applied in issue order",
      first: { kind: "success", updatedAt: T0 } as Event,
      second: { kind: "not_found" } as Event,
      applyOrder: "issued" as const,
      expectNotFound: true,
    },
    {
      name: "success (issued first) then not_found (issued second): applied in reverse (out-of-order arrival)",
      first: { kind: "success", updatedAt: T0 } as Event,
      second: { kind: "not_found" } as Event,
      applyOrder: "reverse" as const,
      expectNotFound: true,
    },
    {
      name: "not_found (issued first) then success (issued second): applied in issue order",
      first: { kind: "not_found" } as Event,
      second: { kind: "success", updatedAt: T0 } as Event,
      applyOrder: "issued" as const,
      expectNotFound: false,
    },
    {
      name: "not_found (issued first) then success (issued second): applied in reverse (out-of-order arrival)",
      first: { kind: "not_found" } as Event,
      second: { kind: "success", updatedAt: T0 } as Event,
      applyOrder: "reverse" as const,
      expectNotFound: false,
    },
    {
      name: "not_found (issued first) then not_found (issued second): a fresher 404 advances the tombstone",
      first: { kind: "not_found" } as Event,
      second: { kind: "not_found" } as Event,
      applyOrder: "issued" as const,
      expectNotFound: true,
    },
    {
      name: "not_found (issued first) then not_found (issued second): a stale duplicate 404 applied after is a no-op",
      first: { kind: "not_found" } as Event,
      second: { kind: "not_found" } as Event,
      applyOrder: "reverse" as const,
      expectNotFound: true,
    },
  ])("$name", ({ first, second, applyOrder, expectNotFound }) => {
    const firstSeq = useProposalsStore.getState().takeProposalTicket(COORDINATOR_ID);
    const secondSeq = useProposalsStore.getState().takeProposalTicket(COORDINATOR_ID);

    if (applyOrder === "issued") {
      apply(firstSeq, first);
      apply(secondSeq, second);
    } else {
      apply(secondSeq, second);
      apply(firstSeq, first);
    }

    const coordinator = useProposalsStore.getState().byCoordinator[COORDINATOR_ID];
    expect(coordinator?.tombstoneSeq[PROPOSAL_ID] !== undefined).toBe(expectNotFound);
    expect(coordinator?.byId[PROPOSAL_ID] === undefined).toBe(expectNotFound);
  });

  it("success vs success: content arbitration by updated_at wins regardless of issue or apply order", () => {
    const earlyIssuedLateContent: Event = { kind: "success", updatedAt: T10, status: "approving" };
    const lateIssuedEarlyContent: Event = { kind: "success", updatedAt: T5, status: "pending" };

    const earlySeq = useProposalsStore.getState().takeProposalTicket(COORDINATOR_ID);
    const lateSeq = useProposalsStore.getState().takeProposalTicket(COORDINATOR_ID);

    // The later-issued ticket resolves first, then the earlier-issued
    // ticket resolves last - but it carries the later `updated_at`, so the
    // merge rules must still let it win (mergeProposal is ticket-independent).
    apply(lateSeq, lateIssuedEarlyContent);
    apply(earlySeq, earlyIssuedLateContent);

    const coordinator = useProposalsStore.getState().byCoordinator[COORDINATOR_ID];
    expect(coordinator?.byId[PROPOSAL_ID]?.status).toBe("approving");
  });
});

describe("cross-path interleaving", () => {
  const PROPOSAL_ID = "p-1";

  it("backfill vs by-id: a stale by-id 404 issued before a fresher backfill success cannot evict its row", async () => {
    seedProposal(COORDINATOR_ID, proposal({ id: PROPOSAL_ID, status: "pending" }));

    const byIdRead = deferred<Proposal>();
    const backfillRead = deferred<Proposal>();
    getProposalMock.mockReturnValueOnce(byIdRead.promise).mockReturnValueOnce(backfillRead.promise);
    listProposalsMock.mockResolvedValueOnce({ proposals: [] });

    renderHook(() => useProposalById(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID));
    renderHook(() => useProposals(WORKSPACE_ID, COORDINATOR_ID));

    // The by-id hook's own mount read takes the first ticket; the pending
    // list resolves empty, and the backfill it triggers for the
    // still-cached, still-open p-1 takes the second (fresher) ticket.
    await waitFor(() => expect(getProposalMock).toHaveBeenCalledTimes(2));

    // The fresher (backfill) ticket resolves first with a settled row...
    await act(async () => {
      backfillRead.resolve(proposal({ id: PROPOSAL_ID, status: "approving", updated_at: T5 }));
    });
    // ...then the stale (by-id) ticket resolves last with a 404. Being
    // older than the response already applied, it must not evict the row.
    await act(async () => {
      byIdRead.reject(new ApiError("not found", 404, {}));
    });

    expect(
      useProposalsStore.getState().byCoordinator[COORDINATOR_ID]?.byId[PROPOSAL_ID]?.status,
    ).toBe("approving");
  });

  it("decision vs re-read: a stale re-read success issued before a fresher decision 404 cannot clear notFound", async () => {
    const reject = deferred<Proposal>();
    rejectProposalMock.mockReturnValueOnce(reject.promise);
    const wsClient = makeWsClient();
    clients.active = wsClient;

    getProposalMock.mockResolvedValueOnce(proposal({ id: PROPOSAL_ID, status: "pending" }));
    const { result: byIdResult } = renderHook(() =>
      useProposalById(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID),
    );
    await waitFor(() => expect(byIdResult.current.proposal).toBeDefined());

    const { result: decisionResult } = renderHook(() =>
      useProposalDecision(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID),
    );

    let rejectPromise!: ReturnType<typeof decisionResult.current.reject>;
    act(() => {
      rejectPromise = decisionResult.current.reject();
    });

    // A re-read (coordinator.updated) is dispatched next, taking the next
    // ticket - fresher than the decision's own eventual ticket, since the
    // decision only takes its ticket lazily once its response resolves.
    const reRead = deferred<Proposal>();
    getProposalMock.mockReturnValueOnce(reRead.promise);
    const handler = wsClient.on.mock.calls[0]?.[1] as UpdatedHandler;
    act(() => handler(UPDATED_PAYLOAD));
    await waitFor(() => expect(getProposalMock).toHaveBeenCalledTimes(2));

    // The decision's 404 resolves first, taking a ticket fresher than the
    // re-read already in flight, and sets the tombstone.
    await act(async () => {
      reject.reject(new ApiError("gone", 404, {}));
      await rejectPromise;
    });
    expect(byIdResult.current.notFound).toBe(true);

    // The re-read - issued before the decision's ticket but resolving after
    // it - finally succeeds. Being the stale response, it must not clear
    // the fresher tombstone the decision just set.
    await act(async () => {
      reRead.resolve(proposal({ id: PROPOSAL_ID, status: "pending" }));
    });

    expect(byIdResult.current.notFound).toBe(true);
    expect(byIdResult.current.proposal).toBeUndefined();
  });
});

describe("resume, message and move rows", () => {
  const KIND_ID = "k-1";

  function kindRow(kind: "resume" | "message" | "move", overrides: Record<string, unknown> = {}) {
    const specs = {
      resume: { task_id: "t-1", rationale: "r" },
      message: { task_id: "t-1", text: "hello", rationale: "r" },
      move: {
        task_id: "t-1",
        workflow_id: "wf",
        from_step_id: "a",
        to_step_id: "b",
        rationale: "r",
      },
    };
    return {
      ...proposal({ id: KIND_ID }),
      kind,
      spec: specs[kind],
      ...overrides,
    } as unknown as Proposal;
  }

  it.each(["resume", "message", "move"] as const)(
    "stores and lists a %s row with the flag on",
    async (kind) => {
      listProposalsMock.mockResolvedValueOnce({ proposals: [kindRow(kind)] });
      const { result } = renderHook(() => useProposals(WORKSPACE_ID, COORDINATOR_ID, true));
      await waitFor(() => expect(result.current.proposals.value).toHaveLength(1));
      expect(result.current.proposals.value?.[0].id).toBe(KIND_ID);
    },
  );

  it("hides and does not count a kind row with the flag off", async () => {
    listProposalsMock.mockResolvedValueOnce({
      proposals: [kindRow("resume"), proposal({ id: "c-1" })],
    });
    const { result } = renderHook(() => useProposals(WORKSPACE_ID, COORDINATOR_ID, false));
    await waitFor(() =>
      expect((result.current.proposals.value ?? []).map((p) => p.id)).toEqual(["c-1"]),
    );
  });

  it("never stores a row of a kind this client does not know", async () => {
    listProposalsMock.mockResolvedValueOnce({
      proposals: [kindRow("resume", { kind: "teleport" })],
    });
    const { result } = renderHook(() => useProposals(WORKSPACE_ID, COORDINATOR_ID, true));
    await waitFor(() => expect(listProposalsMock).toHaveBeenCalled());
    expect(result.current.proposals.value ?? []).toHaveLength(0);
    expect(
      useProposalsStore.getState().byCoordinator[COORDINATOR_ID]?.byId[KIND_ID],
    ).toBeUndefined();
  });

  it.each(["resume", "message", "move"] as const)(
    "interleaving: a stale %s re-read cannot undo a fresher decision",
    async (kind) => {
      seedProposal(COORDINATOR_ID, kindRow(kind, { status: "pending" }) as Proposal);
      const stale = deferred<Proposal>();
      const fresh = deferred<Proposal>();
      getProposalMock.mockReturnValueOnce(stale.promise).mockReturnValueOnce(fresh.promise);
      listProposalsMock.mockResolvedValue({ proposals: [] });

      renderHook(() => useProposalById(WORKSPACE_ID, COORDINATOR_ID, KIND_ID));
      renderHook(() => useProposals(WORKSPACE_ID, COORDINATOR_ID, true));
      await waitFor(() => expect(getProposalMock).toHaveBeenCalledTimes(2));

      await act(async () => {
        fresh.resolve(kindRow(kind, { status: "approved", updated_at: T10 }));
      });
      await act(async () => {
        stale.resolve(kindRow(kind, { status: "pending", updated_at: T0 }));
      });
      expect(
        useProposalsStore.getState().byCoordinator[COORDINATOR_ID]?.byId[KIND_ID]?.status,
      ).toBe("approved");
    },
  );

  it.each(["resume", "message", "move"] as const)(
    "interleaving: a stale %s by-id 404 issued before a fresher backfill success cannot evict its row",
    async (kind) => {
      seedProposal(COORDINATOR_ID, kindRow(kind, { status: "pending" }) as Proposal);
      const byIdRead = deferred<Proposal>();
      const backfillRead = deferred<Proposal>();
      getProposalMock
        .mockReturnValueOnce(byIdRead.promise)
        .mockReturnValueOnce(backfillRead.promise);
      listProposalsMock.mockResolvedValue({ proposals: [] });

      const { result } = renderHook(() => useProposalById(WORKSPACE_ID, COORDINATOR_ID, KIND_ID));
      renderHook(() => useProposals(WORKSPACE_ID, COORDINATOR_ID, true));
      await waitFor(() => expect(getProposalMock).toHaveBeenCalledTimes(2));

      await act(async () => {
        backfillRead.resolve(kindRow(kind, { status: "approved", updated_at: T10 }));
      });
      await act(async () => {
        byIdRead.reject(new ApiError("gone", 404, {}));
      });
      expect(
        useProposalsStore.getState().byCoordinator[COORDINATOR_ID]?.byId[KIND_ID]?.status,
      ).toBe("approved");
      expect(result.current.notFound).toBe(false);
    },
  );
});
