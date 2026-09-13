import { afterEach, describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";

const mockListWorkflows = vi.fn();
const mockSetWorkflows = vi.fn();
const mockSetWorkspaceContextRead = vi.fn();
const mockRequestWorkspaceContextRefresh = vi.fn();

type WorkspaceContextReadResult =
  | "pending"
  | "success"
  | "transient"
  | "access_denied"
  | "not_found"
  | "invalid"
  | "cancelled"
  | "unknown";

type MockWorkspaceContextRead = {
  workspaceId: string | null;
  generation: number;
  pending: { workflows: boolean; repositories: boolean; steps: boolean };
  errors: Record<"workflows" | "repositories" | "steps", WorkspaceContextReadResult | null>;
  retryAfterMs: Record<"workflows" | "repositories" | "steps", number | null>;
  retryVersion: number;
  retryCycle: number;
  snapshotPending?: boolean;
  snapshotError?: WorkspaceContextReadResult | null;
  snapshotRetryAfterMs?: number | null;
};

type MockState = {
  workflows: { items: Array<{ id: string; workspaceId: string; name: string }> };
  workspaces: { activeId: string | null };
  workspaceContextGeneration: number;
  setWorkflows: typeof mockSetWorkflows;
  workspaceContextRead?: MockWorkspaceContextRead;
  setWorkspaceContextRead?: typeof mockSetWorkspaceContextRead;
  requestWorkspaceContextRefresh?: typeof mockRequestWorkspaceContextRefresh;
};

let mockState: MockState = {
  workflows: { items: [] },
  workspaces: { activeId: null },
  workspaceContextGeneration: 0,
  setWorkflows: mockSetWorkflows,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
  useAppStoreApi: () => ({ getState: () => mockState }),
}));

vi.mock("@/lib/api", () => ({
  listWorkflows: (...args: unknown[]) => mockListWorkflows(...args),
}));

import { useWorkflows, useEnsureWorkspaceWorkflows } from "./use-workflows";

function makeWorkflow(id: string, workspaceId: string) {
  return {
    id,
    workspace_id: workspaceId,
    name: id,
    description: null,
    prompt: undefined as string | undefined,
    sort_order: 0,
    agent_profile_id: null,
    hidden: false,
    style: null,
  };
}

function setVisibility(value: DocumentVisibilityState) {
  Object.defineProperty(document, "visibilityState", { configurable: true, value });
}

describe("useWorkflows — stale response guard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockState = {
      workflows: { items: [] },
      workspaces: { activeId: "ws-A" },
      workspaceContextGeneration: 0,
      setWorkflows: mockSetWorkflows,
    };
  });

  it("discards an A response that resolves after reset activates B but before rerender", async () => {
    let resolveA: (value: { workflows: ReturnType<typeof makeWorkflow>[] }) => void = () => {};
    mockState.workspaces.activeId = "ws-A";
    mockListWorkflows.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveA = resolve;
        }),
    );

    renderHook(() => useWorkflows("ws-A", true));
    await waitFor(() => expect(mockListWorkflows).toHaveBeenCalledWith("ws-A", expect.anything()));

    mockState = {
      ...mockState,
      workspaces: { activeId: "ws-B" },
      workspaceContextGeneration: 1,
    };
    resolveA({ workflows: [makeWorkflow("wf-A", "ws-A")] });
    for (let i = 0; i < 5; i++) await Promise.resolve();

    expect(mockSetWorkflows).not.toHaveBeenCalled();
  });

  it("discards a stale in-flight response when the workspace switches mid-fetch", async () => {
    let resolveStale: (v: unknown) => void = () => {};
    mockListWorkflows.mockImplementationOnce(
      () =>
        new Promise((res) => {
          resolveStale = res;
        }),
    );

    const { rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string | null }) => useWorkflows(workspaceId, true),
      { initialProps: { workspaceId: "ws-A" } },
    );
    await waitFor(() => expect(mockListWorkflows).toHaveBeenCalledWith("ws-A", expect.anything()));

    // Switch to workspace B before A's fetch resolves; B resolves first.
    mockState = {
      ...mockState,
      workspaces: { activeId: "ws-B" },
      workspaceContextGeneration: 1,
    };
    mockListWorkflows.mockResolvedValueOnce({ workflows: [makeWorkflow("wf-B", "ws-B")] });
    rerender({ workspaceId: "ws-B" });
    await waitFor(() =>
      expect(mockSetWorkflows).toHaveBeenCalledWith([expect.objectContaining({ id: "wf-B" })]),
    );

    // Now let A resolve. It must NOT overwrite the store with A's workflows.
    resolveStale({ workflows: [makeWorkflow("wf-A", "ws-A")] });
    for (let i = 0; i < 5; i++) await Promise.resolve();

    const written = mockSetWorkflows.mock.calls.map((call) => call[0]);
    const wroteA = written.some(
      (list: Array<{ id: string }>) => list.length > 0 && list.some((w) => w.id === "wf-A"),
    );
    expect(wroteA).toBe(false);
  });

  it("does not clear workflows when a stale fetch fails after the workspace switched", async () => {
    let rejectStale: (e: Error) => void = () => {};
    mockListWorkflows.mockImplementationOnce(
      () =>
        new Promise((_res, rej) => {
          rejectStale = rej;
        }),
    );

    const { rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string | null }) => useWorkflows(workspaceId, true),
      { initialProps: { workspaceId: "ws-A" } },
    );
    await waitFor(() => expect(mockListWorkflows).toHaveBeenCalledWith("ws-A", expect.anything()));

    // Switch to workspace B; B resolves with data.
    mockState = {
      ...mockState,
      workspaces: { activeId: "ws-B" },
      workspaceContextGeneration: 1,
    };
    mockListWorkflows.mockResolvedValueOnce({ workflows: [makeWorkflow("wf-B", "ws-B")] });
    rerender({ workspaceId: "ws-B" });
    await waitFor(() =>
      expect(mockSetWorkflows).toHaveBeenCalledWith([expect.objectContaining({ id: "wf-B" })]),
    );

    // A's fetch fails after B already succeeded. Catch must NOT wipe the store.
    rejectStale(new Error("network"));
    for (let i = 0; i < 5; i++) await Promise.resolve();

    const cleared = mockSetWorkflows.mock.calls.some(
      (call) => Array.isArray(call[0]) && call[0].length === 0,
    );
    expect(cleared).toBe(false);
  });

  it("does not clear hydrated workflows when the fetch for the current workspace fails", async () => {
    // The sidebar mounts on every route (task detail, settings, ...) and boot
    // hydrates `state.workflows.items` before the sidebar's refresh fetch
    // fires. If that fetch flakes, blowing the store away leaves the sidebar,
    // board, and kanban scoping with no workflow IDs until another success.
    mockListWorkflows.mockRejectedValueOnce(new Error("network"));

    renderHook(() => useWorkflows("ws-A", true));

    await waitFor(() => expect(mockListWorkflows).toHaveBeenCalledWith("ws-A", expect.anything()));
    for (let i = 0; i < 5; i++) await Promise.resolve();

    expect(mockSetWorkflows).not.toHaveBeenCalled();
  });
});

describe("useWorkflows — explicit workspace selection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockState = {
      workflows: { items: [] },
      workspaces: { activeId: "ws-A" },
      workspaceContextGeneration: 0,
      setWorkflows: mockSetWorkflows,
    };
  });

  it("loads workflows when the selected workspace is not globally active", async () => {
    mockListWorkflows.mockResolvedValueOnce({ workflows: [makeWorkflow("wf-B", "ws-B")] });

    renderHook(() => useWorkflows("ws-B", true));

    await waitFor(() =>
      expect(mockSetWorkflows).toHaveBeenCalledWith([
        expect.objectContaining({ id: "wf-B", workspaceId: "ws-B" }),
      ]),
    );
  });

  it("maps the workflow's prompt into the store item", async () => {
    mockListWorkflows.mockResolvedValueOnce({
      workflows: [{ ...makeWorkflow("wf-B", "ws-B"), prompt: "Do the thing" }],
    });

    renderHook(() => useWorkflows("ws-B", true));

    await waitFor(() =>
      expect(mockSetWorkflows).toHaveBeenCalledWith([
        expect.objectContaining({ id: "wf-B", prompt: "Do the thing" }),
      ]),
    );
  });
});

// eslint-disable-next-line max-lines-per-function -- related workspace recovery cases share one fixture
describe("useEnsureWorkspaceWorkflows", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListWorkflows.mockResolvedValue({ workflows: [] });
    mockState = {
      workflows: { items: [] },
      workspaces: { activeId: "ws-A" },
      workspaceContextGeneration: 0,
      setWorkflows: mockSetWorkflows,
    };
  });

  it("fetches workflows for the store's active workspace on mount", async () => {
    renderHook(() => useEnsureWorkspaceWorkflows());
    await waitFor(() => expect(mockListWorkflows).toHaveBeenCalledWith("ws-A", expect.anything()));
  });

  it("re-fetches when the store's active workspace changes", async () => {
    const { rerender } = renderHook(() => useEnsureWorkspaceWorkflows());
    await waitFor(() => expect(mockListWorkflows).toHaveBeenCalledWith("ws-A", expect.anything()));

    mockState = { ...mockState, workspaces: { activeId: "ws-B" } };
    rerender();
    await waitFor(() => expect(mockListWorkflows).toHaveBeenCalledWith("ws-B", expect.anything()));
  });

  it("bounds scheduled retry attempts and does not restart the cycle for timer refreshes", async () => {
    vi.useFakeTimers();
    const contextRead: MockWorkspaceContextRead = {
      workspaceId: "ws-A",
      generation: 0,
      pending: { workflows: false, repositories: false, steps: false },
      errors: { workflows: "transient", repositories: null, steps: null },
      retryAfterMs: { workflows: null, repositories: null, steps: null },
      retryVersion: 0,
      retryCycle: 0,
    };
    mockSetWorkspaceContextRead.mockImplementation(
      (
        collection: "workflows" | "repositories" | "steps",
        workspaceId: string,
        generation: number,
        result: WorkspaceContextReadResult,
        retryAfterMs?: number,
      ) => {
        if (!mockState.workspaceContextRead) return;
        mockState = {
          ...mockState,
          workspaceContextRead: {
            ...mockState.workspaceContextRead,
            workspaceId,
            generation,
            pending: {
              ...mockState.workspaceContextRead.pending,
              [collection]: result === "pending",
            },
            errors: {
              ...mockState.workspaceContextRead.errors,
              [collection]: result === "pending" || result === "success" ? null : result,
            },
            retryAfterMs: {
              ...mockState.workspaceContextRead.retryAfterMs,
              [collection]: result === "transient" ? (retryAfterMs ?? null) : null,
            },
          },
        };
      },
    );
    mockRequestWorkspaceContextRefresh.mockImplementation((resetRetryCycle = true) => {
      if (!mockState.workspaceContextRead) return;
      mockState = {
        ...mockState,
        workspaceContextRead: {
          ...mockState.workspaceContextRead,
          retryVersion: mockState.workspaceContextRead.retryVersion + 1,
          retryCycle: mockState.workspaceContextRead.retryCycle + (resetRetryCycle ? 1 : 0),
        },
      };
    });
    mockState = {
      ...mockState,
      workspaceContextRead: contextRead,
      setWorkspaceContextRead: mockSetWorkspaceContextRead,
      requestWorkspaceContextRefresh: mockRequestWorkspaceContextRefresh,
    };
    mockListWorkflows.mockRejectedValue(new Error("offline"));

    const { rerender } = renderHook(() => useEnsureWorkspaceWorkflows());
    await act(async () => {
      await Promise.resolve();
    });
    expect(mockListWorkflows).toHaveBeenCalledTimes(1);

    await act(async () => {
      vi.advanceTimersByTime(2_000);
    });
    expect(mockRequestWorkspaceContextRefresh).toHaveBeenCalledWith(false);
    rerender();
    await act(async () => {
      await Promise.resolve();
    });

    await act(async () => {
      vi.advanceTimersByTime(5_000);
    });
    expect(mockRequestWorkspaceContextRefresh).toHaveBeenCalledTimes(2);
    rerender();
    await act(async () => {
      await Promise.resolve();
    });

    await act(async () => {
      vi.advanceTimersByTime(10_000);
    });
    expect(mockRequestWorkspaceContextRefresh).toHaveBeenCalledTimes(2);
  });

  it("cancels a scheduled retry when the workflow loader unmounts", async () => {
    vi.useFakeTimers();
    mockState = {
      ...mockState,
      workspaceContextRead: {
        workspaceId: "ws-A",
        generation: 0,
        pending: { workflows: false, repositories: false, steps: false },
        errors: { workflows: "transient", repositories: null, steps: null },
        retryAfterMs: { workflows: null, repositories: null, steps: null },
        retryVersion: 0,
        retryCycle: 0,
      },
      setWorkspaceContextRead: mockSetWorkspaceContextRead,
      requestWorkspaceContextRefresh: mockRequestWorkspaceContextRefresh,
    };
    mockListWorkflows.mockRejectedValue(new Error("offline"));

    const { unmount } = renderHook(() => useEnsureWorkspaceWorkflows());
    unmount();
    await act(async () => {
      vi.advanceTimersByTime(10_000);
    });

    expect(mockRequestWorkspaceContextRefresh).not.toHaveBeenCalled();
  });

  it("defers recovery while hidden and starts one bounded cycle on foreground", async () => {
    vi.useFakeTimers();
    setVisibility("hidden");
    mockState = {
      ...mockState,
      workspaceContextRead: {
        workspaceId: "ws-A",
        generation: 0,
        pending: { workflows: false, repositories: false, steps: false },
        errors: { workflows: "transient", repositories: null, steps: null },
        retryAfterMs: { workflows: null, repositories: null, steps: null },
        retryVersion: 0,
        retryCycle: 0,
      },
      setWorkspaceContextRead: mockSetWorkspaceContextRead,
      requestWorkspaceContextRefresh: mockRequestWorkspaceContextRefresh,
    };
    mockListWorkflows.mockRejectedValue(new Error("offline"));

    const { rerender } = renderHook(() => useEnsureWorkspaceWorkflows());
    await act(async () => {
      await Promise.resolve();
      vi.advanceTimersByTime(10_000);
    });
    expect(mockRequestWorkspaceContextRefresh).not.toHaveBeenCalled();

    setVisibility("visible");
    act(() => document.dispatchEvent(new Event("visibilitychange")));
    expect(mockRequestWorkspaceContextRefresh).toHaveBeenCalledWith();
    rerender();

    await act(async () => {
      vi.advanceTimersByTime(2_000);
    });
    expect(mockRequestWorkspaceContextRefresh).toHaveBeenCalledWith(false);
  });

  it("does not spend a retry attempt when a timer is replaced before dispatch", async () => {
    vi.useFakeTimers();
    setVisibility("visible");
    const contextRead: MockWorkspaceContextRead = {
      workspaceId: "ws-A",
      generation: 0,
      pending: { workflows: false, repositories: false, steps: false },
      errors: { workflows: "transient", repositories: null, steps: null },
      retryAfterMs: { workflows: null, repositories: null, steps: null },
      retryVersion: 0,
      retryCycle: 0,
    };
    mockState = {
      ...mockState,
      workspaceContextRead: contextRead,
      setWorkspaceContextRead: mockSetWorkspaceContextRead,
      requestWorkspaceContextRefresh: mockRequestWorkspaceContextRefresh,
    };
    mockListWorkflows.mockRejectedValue(new Error("offline"));

    const { rerender } = renderHook(() => useEnsureWorkspaceWorkflows());
    await act(async () => {
      await Promise.resolve();
    });

    mockState = {
      ...mockState,
      workspaceContextRead: { ...contextRead, retryAfterMs: { ...contextRead.retryAfterMs } },
    };
    rerender();
    mockState = {
      ...mockState,
      workspaceContextRead: { ...contextRead, retryAfterMs: { ...contextRead.retryAfterMs } },
    };
    rerender();

    await act(async () => {
      vi.advanceTimersByTime(2_000);
    });
    expect(mockRequestWorkspaceContextRefresh).toHaveBeenCalledWith(false);
  });
});

afterEach(() => {
  vi.useRealTimers();
});
