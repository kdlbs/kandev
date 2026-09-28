import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Proposal } from "@/lib/api/domains/coordinator-api";
import { ApiError } from "@/lib/api/client";

const listProposalsMock = vi.fn();
const getProposalMock = vi.fn();

vi.mock("@/lib/api/domains/coordinator-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/coordinator-api")>();
  return {
    ...actual,
    listProposals: (...args: unknown[]) => listProposalsMock(...args),
    getProposal: (...args: unknown[]) => getProposalMock(...args),
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

type UpdatedHandler = (message: { payload: Record<string, unknown> }) => void;

function makeWsClient() {
  return {
    on: vi.fn((_type: string, _handler: UpdatedHandler) => vi.fn()),
  };
}

beforeEach(() => {
  listProposalsMock.mockReset();
  getProposalMock.mockReset();
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
