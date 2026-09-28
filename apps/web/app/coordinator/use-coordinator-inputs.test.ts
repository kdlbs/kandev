import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Proposal, Stall } from "@/lib/api/domains/coordinator-api";

const listCoordinatorStallsMock = vi.fn();
const listProposalsMock = vi.fn();

vi.mock("@/lib/api/domains/coordinator-api", () => ({
  listCoordinatorStalls: (...args: unknown[]) => listCoordinatorStallsMock(...args),
  listProposals: (...args: unknown[]) => listProposalsMock(...args),
}));

const clients = vi.hoisted(() => ({ active: undefined as unknown }));

vi.mock("@/lib/ws/connection", () => ({
  useWebSocketClient: () => clients.active,
}));

// Import after mocks so the hook picks up the mocked modules.
import { useCoordinatorInputs } from "./use-coordinator-inputs";

const WORKSPACE_ID = "workspace-1";
const COORDINATOR_ID = "coordinator-1";

function stall(taskId: string): Stall {
  return {
    task_id: taskId,
    stalled_for_ms: 60_000,
    last_event_at: "2026-09-27T00:00:00Z",
    detected_at: "2026-09-27T00:01:00Z",
  };
}

function proposal(id: string): Proposal {
  return {
    id,
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
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
  };
}

type UpdatedHandler = (message: { payload: Record<string, unknown> }) => void;

function makeWsClient() {
  return {
    on: vi.fn((_type: string, _handler: UpdatedHandler) => vi.fn()),
  };
}

beforeEach(() => {
  listCoordinatorStallsMock.mockReset();
  listProposalsMock.mockReset();
  clients.active = undefined;
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("useCoordinatorInputs - initial load", () => {
  it("loads stalls and proposals on mount", async () => {
    listCoordinatorStallsMock.mockResolvedValue({ stalls: [stall("t-1")] });
    listProposalsMock.mockResolvedValue({ proposals: [proposal("p-1")] });

    const { result } = renderHook(() => useCoordinatorInputs(WORKSPACE_ID, COORDINATOR_ID));

    await waitFor(() => expect(result.current.stalls.value).toBeDefined());
    await waitFor(() => expect(result.current.proposals.value).toBeDefined());

    expect(result.current.stalls.value).toEqual([stall("t-1")]);
    expect(result.current.stalls.error).toBe(false);
    expect(result.current.proposals.value).toEqual([proposal("p-1")]);
    expect(result.current.proposals.error).toBe(false);
    expect(listCoordinatorStallsMock).toHaveBeenCalledWith(WORKSPACE_ID);
    expect(listProposalsMock).toHaveBeenCalledWith(WORKSPACE_ID, COORDINATOR_ID, "pending");
  });

  it("does not fetch when workspaceId or coordinatorId is null", () => {
    renderHook(() => useCoordinatorInputs(null, null));

    expect(listCoordinatorStallsMock).not.toHaveBeenCalled();
    expect(listProposalsMock).not.toHaveBeenCalled();
  });
});

describe("useCoordinatorInputs - per-input failure", () => {
  it("marks only the failed input as an error, leaving the other's success intact", async () => {
    listCoordinatorStallsMock.mockRejectedValue(new Error("stalls unavailable"));
    listProposalsMock.mockResolvedValue({ proposals: [proposal("p-1")] });

    const { result } = renderHook(() => useCoordinatorInputs(WORKSPACE_ID, COORDINATOR_ID));

    await waitFor(() => expect(result.current.stalls.error).toBe(true));
    await waitFor(() => expect(result.current.proposals.value).toBeDefined());

    expect(result.current.stalls.value).toBeUndefined();
    expect(result.current.proposals.error).toBe(false);
  });
});

describe("useCoordinatorInputs - retryFailed", () => {
  it("retryFailed re-issues only the inputs currently in an error state", async () => {
    listCoordinatorStallsMock.mockRejectedValueOnce(new Error("stalls unavailable"));
    listProposalsMock.mockResolvedValue({ proposals: [] });

    const { result } = renderHook(() => useCoordinatorInputs(WORKSPACE_ID, COORDINATOR_ID));

    await waitFor(() => expect(result.current.stalls.error).toBe(true));
    await waitFor(() => expect(result.current.proposals.value).toBeDefined());

    listCoordinatorStallsMock.mockResolvedValueOnce({ stalls: [stall("t-2")] });
    listProposalsMock.mockClear();

    act(() => {
      result.current.retryFailed();
    });

    await waitFor(() => expect(result.current.stalls.value).toEqual([stall("t-2")]));
    expect(listCoordinatorStallsMock).toHaveBeenCalledTimes(2);
    expect(listProposalsMock).not.toHaveBeenCalled();
  });
});

describe("useCoordinatorInputs - coordinator.updated", () => {
  it("re-reads both inputs on a matching coordinator.updated event", async () => {
    const wsClient = makeWsClient();
    clients.active = wsClient;
    listCoordinatorStallsMock.mockResolvedValue({ stalls: [] });
    listProposalsMock.mockResolvedValue({ proposals: [] });

    const { result } = renderHook(() => useCoordinatorInputs(WORKSPACE_ID, COORDINATOR_ID));

    await waitFor(() => expect(result.current.stalls.value).toBeDefined());
    expect(wsClient.on).toHaveBeenCalledWith("coordinator.updated", expect.any(Function));

    const handler = wsClient.on.mock.calls[0]?.[1] as UpdatedHandler;
    listCoordinatorStallsMock.mockResolvedValueOnce({ stalls: [stall("t-3")] });

    act(() => {
      handler({
        payload: {
          workspace_id: WORKSPACE_ID,
          coordinator_id: COORDINATOR_ID,
          open_proposals: 1,
        },
      });
    });

    await waitFor(() => expect(result.current.stalls.value).toEqual([stall("t-3")]));
    expect(listCoordinatorStallsMock).toHaveBeenCalledTimes(2);
    expect(listProposalsMock).toHaveBeenCalledTimes(2);
  });

  it("ignores a coordinator.updated event for a different workspace or coordinator", async () => {
    const wsClient = makeWsClient();
    clients.active = wsClient;
    listCoordinatorStallsMock.mockResolvedValue({ stalls: [] });
    listProposalsMock.mockResolvedValue({ proposals: [] });

    const { result } = renderHook(() => useCoordinatorInputs(WORKSPACE_ID, COORDINATOR_ID));

    await waitFor(() => expect(result.current.stalls.value).toBeDefined());
    const handler = wsClient.on.mock.calls[0]?.[1] as UpdatedHandler;

    listCoordinatorStallsMock.mockClear();
    listProposalsMock.mockClear();

    act(() => {
      handler({
        payload: {
          workspace_id: "other-workspace",
          coordinator_id: COORDINATOR_ID,
          open_proposals: 1,
        },
      });
    });

    expect(listCoordinatorStallsMock).not.toHaveBeenCalled();
    expect(listProposalsMock).not.toHaveBeenCalled();
  });
});

describe("useCoordinatorInputs - coordinator changes", () => {
  it("resets to the initial entry and re-fetches when the coordinator changes", async () => {
    listCoordinatorStallsMock.mockResolvedValue({ stalls: [stall("t-1")] });
    listProposalsMock.mockResolvedValue({ proposals: [] });

    const { result, rerender } = renderHook(
      ({ coordinatorId }: { coordinatorId: string | null }) =>
        useCoordinatorInputs(WORKSPACE_ID, coordinatorId),
      { initialProps: { coordinatorId: COORDINATOR_ID } },
    );

    await waitFor(() => expect(result.current.stalls.value).toEqual([stall("t-1")]));

    listCoordinatorStallsMock.mockClear();
    listCoordinatorStallsMock.mockResolvedValue({ stalls: [stall("t-9")] });

    rerender({ coordinatorId: "coordinator-2" });

    expect(result.current.stalls.value).toBeUndefined();
    await waitFor(() => expect(result.current.stalls.value).toEqual([stall("t-9")]));
    expect(listProposalsMock).toHaveBeenCalledWith(WORKSPACE_ID, "coordinator-2", "pending");
  });
});

describe("useCoordinatorInputs - stale request protection", () => {
  it("discards a stale in-flight read when a newer read for the same input has already resolved", async () => {
    let resolveFirst: ((v: { stalls: Stall[] }) => void) | undefined;
    let resolveSecond: ((v: { stalls: Stall[] }) => void) | undefined;
    listCoordinatorStallsMock
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveFirst = resolve;
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveSecond = resolve;
          }),
      );
    listProposalsMock.mockResolvedValue({ proposals: [] });
    const wsClient = makeWsClient();
    clients.active = wsClient;

    const { result } = renderHook(() => useCoordinatorInputs(WORKSPACE_ID, COORDINATOR_ID));
    await waitFor(() => expect(listCoordinatorStallsMock).toHaveBeenCalledTimes(1));

    const handler = wsClient.on.mock.calls[0]?.[1] as UpdatedHandler;
    act(() => {
      handler({
        payload: {
          workspace_id: WORKSPACE_ID,
          coordinator_id: COORDINATOR_ID,
          open_proposals: 1,
        },
      });
    });
    await waitFor(() => expect(listCoordinatorStallsMock).toHaveBeenCalledTimes(2));

    act(() => {
      resolveSecond?.({ stalls: [stall("t-fresh")] });
    });
    await waitFor(() => expect(result.current.stalls.value).toEqual([stall("t-fresh")]));

    act(() => {
      resolveFirst?.({ stalls: [stall("t-stale")] });
    });
    await act(async () => {
      await Promise.resolve();
    });

    expect(result.current.stalls.value).toEqual([stall("t-fresh")]);
  });
});
