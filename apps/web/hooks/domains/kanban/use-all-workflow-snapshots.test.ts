import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";

const mockClearKanbanMulti = vi.fn();
const mockSetKanbanMultiLoading = vi.fn();
const mockSetWorkflowSnapshot = vi.fn();
const mockFetchWorkflowSnapshot = vi.fn();

type Workflow = { id: string; workspaceId: string; name: string };
type MockState = {
  connection: { status: string };
  workspaces: { activeId: string | null };
  workspaceContextGeneration: number;
  workflows: { items: Workflow[] };
  kanbanMulti: { snapshots: Record<string, unknown>; isLoading: boolean };
  clearKanbanMulti: typeof mockClearKanbanMulti;
  setKanbanMultiLoading: typeof mockSetKanbanMultiLoading;
  setWorkflowSnapshot: typeof mockSetWorkflowSnapshot;
};

let mockState: MockState = {
  connection: { status: "connected" },
  workspaces: { activeId: "ws-A" },
  workspaceContextGeneration: 0,
  workflows: { items: [] },
  kanbanMulti: { snapshots: {}, isLoading: false },
  clearKanbanMulti: mockClearKanbanMulti,
  setKanbanMultiLoading: mockSetKanbanMultiLoading,
  setWorkflowSnapshot: mockSetWorkflowSnapshot,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
  useAppStoreApi: () => ({ getState: () => mockState }),
}));

vi.mock("@/lib/api", () => ({
  fetchWorkflowSnapshot: (...args: unknown[]) => mockFetchWorkflowSnapshot(...args),
}));

import { useAllWorkflowSnapshots } from "./use-all-workflow-snapshots";

function resetMocks(workflows: Workflow[] = []) {
  vi.clearAllMocks();
  mockFetchWorkflowSnapshot.mockResolvedValue({ steps: [], tasks: [] });
  mockState = {
    connection: { status: "connected" },
    workspaces: { activeId: workflows[0]?.workspaceId ?? null },
    workspaceContextGeneration: 0,
    workflows: { items: workflows },
    kanbanMulti: { snapshots: {}, isLoading: false },
    clearKanbanMulti: mockClearKanbanMulti,
    setKanbanMultiLoading: mockSetKanbanMultiLoading,
    setWorkflowSnapshot: mockSetWorkflowSnapshot,
  };
}

describe("useAllWorkflowSnapshots — workspace scoping", () => {
  beforeEach(() => {
    resetMocks([{ id: "wf-A", workspaceId: "ws-A", name: "A" }]);
  });

  it("does not clear snapshots on initial mount (SSR preservation)", async () => {
    renderHook(
      ({ workspaceId }: { workspaceId: string | null }) => useAllWorkflowSnapshots(workspaceId),
      {
        initialProps: { workspaceId: "ws-A" },
      },
    );

    // Allow the effect + Promise.all to settle.
    await waitFor(() => expect(mockSetKanbanMultiLoading).toHaveBeenCalledWith(true));
    expect(mockClearKanbanMulti).not.toHaveBeenCalled();
  });

  it("does not fetch on initial mount when all workflow snapshots are boot-hydrated", () => {
    mockState.kanbanMulti.snapshots = {
      "wf-A": { workflowId: "wf-A", workflowName: "A", steps: [], tasks: [] },
    };

    renderHook(
      ({ workspaceId }: { workspaceId: string | null }) => useAllWorkflowSnapshots(workspaceId),
      {
        initialProps: { workspaceId: "ws-A" },
      },
    );

    expect(mockFetchWorkflowSnapshot).not.toHaveBeenCalled();
    expect(mockSetKanbanMultiLoading).not.toHaveBeenCalledWith(true);
    expect(mockClearKanbanMulti).not.toHaveBeenCalled();
  });

  it("refetches boot-hydrated snapshots when the Kandev window regains focus", async () => {
    mockState.kanbanMulti.snapshots = {
      "wf-A": { workflowId: "wf-A", workflowName: "A", steps: [], tasks: [] },
    };

    renderHook(() => useAllWorkflowSnapshots("ws-A"));
    expect(mockFetchWorkflowSnapshot).not.toHaveBeenCalled();

    act(() => window.dispatchEvent(new Event("focus")));

    await waitFor(() =>
      expect(mockFetchWorkflowSnapshot).toHaveBeenCalledWith("wf-A", {
        cache: "no-store",
      }),
    );
  });

  it("clears snapshots when workspaceId changes", async () => {
    const { rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string | null }) => useAllWorkflowSnapshots(workspaceId),
      { initialProps: { workspaceId: "ws-A" } },
    );
    await waitFor(() => expect(mockSetKanbanMultiLoading).toHaveBeenCalledWith(true));
    expect(mockClearKanbanMulti).not.toHaveBeenCalled();

    // Switch to workspace B — must clear A's snapshots.
    mockState.workflows = { items: [{ id: "wf-B", workspaceId: "ws-B", name: "B" }] };
    rerender({ workspaceId: "ws-B" });

    await waitFor(() => expect(mockClearKanbanMulti).toHaveBeenCalledTimes(1));
  });

  it("skips refetch when workspace + workflow set is unchanged across renders", async () => {
    const workflows = [{ id: "wf-A", workspaceId: "ws-A", name: "A" }];
    const { rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string | null }) => useAllWorkflowSnapshots(workspaceId),
      { initialProps: { workspaceId: "ws-A" } },
    );
    await waitFor(() => expect(mockFetchWorkflowSnapshot).toHaveBeenCalledTimes(1));

    // Same-workflow rerender — dedup key unchanged, must not refetch.
    mockState.workflows = { items: [...workflows] };
    rerender({ workspaceId: "ws-A" });

    // Positive signal: follow up with a DIFFERENT workflow set, which must
    // trigger a fetch. If dedup worked, the total is 2 (initial + this one).
    // If dedup failed, the same-key rerender would have fired a fetch before
    // this one, making the total 3. Waiting for count==2 proves both:
    // the dedup rerender was skipped AND the next real change still fetches.
    mockState.workflows = { items: [{ id: "wf-A2", workspaceId: "ws-A", name: "A2" }] };
    rerender({ workspaceId: "ws-A" });
    await waitFor(() => expect(mockFetchWorkflowSnapshot).toHaveBeenCalledTimes(2));
    expect(mockFetchWorkflowSnapshot.mock.calls[1][0]).toBe("wf-A2");
    expect(mockClearKanbanMulti).not.toHaveBeenCalled();
  });
});

describe("useAllWorkflowSnapshots — fetch guards", () => {
  beforeEach(() => {
    resetMocks([{ id: "wf-A", workspaceId: "ws-A", name: "A" }]);
  });

  it("discards a stale in-flight fetch when workspace switches mid-fetch", async () => {
    // Hold the first fetch open so it resolves after the workspace switch.
    let resolveStale: (v: { steps: []; tasks: [] }) => void = () => {};
    mockFetchWorkflowSnapshot.mockImplementationOnce(
      () =>
        new Promise((res) => {
          resolveStale = res;
        }),
    );

    const { rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string | null }) => useAllWorkflowSnapshots(workspaceId),
      { initialProps: { workspaceId: "ws-A" } },
    );
    await waitFor(() =>
      expect(mockFetchWorkflowSnapshot).toHaveBeenCalledWith("wf-A", expect.anything()),
    );

    // Switch to workspace B before A's fetch resolves. Wait for B's fetch
    // to settle (positive signal) so the new-gen effect is fully in place.
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({ steps: [], tasks: [] });
    mockState = {
      ...mockState,
      workspaces: { activeId: "ws-B" },
      workspaceContextGeneration: 1,
      workflows: { items: [{ id: "wf-B", workspaceId: "ws-B", name: "B" }] },
    };
    rerender({ workspaceId: "ws-B" });
    await waitFor(() =>
      expect(mockSetWorkflowSnapshot).toHaveBeenCalledWith("wf-B", expect.anything()),
    );

    // Resolve A's stale fetch and drain the microtask queue so its .then
    // and .finally callbacks run. Flushing microtasks is deterministic —
    // unlike setTimeout, it doesn't depend on CI wall-clock speed.
    resolveStale({ steps: [], tasks: [] });
    for (let i = 0; i < 5; i++) await Promise.resolve();

    const writtenIds = mockSetWorkflowSnapshot.mock.calls.map((args) => args[0]);
    expect(writtenIds).not.toContain("wf-A");
  });

  it("discards an A snapshot after reset activates B but before rerender", async () => {
    let resolveA: (value: { steps: []; tasks: [] }) => void = () => {};
    mockFetchWorkflowSnapshot.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveA = resolve;
        }),
    );

    renderHook(() => useAllWorkflowSnapshots("ws-A"));
    await waitFor(() =>
      expect(mockFetchWorkflowSnapshot).toHaveBeenCalledWith("wf-A", expect.anything()),
    );

    mockState = {
      ...mockState,
      workspaces: { activeId: "ws-B" },
      workspaceContextGeneration: 1,
      kanbanMulti: { snapshots: {}, isLoading: false },
    };
    resolveA({ steps: [], tasks: [] });
    for (let i = 0; i < 5; i++) await Promise.resolve();

    expect(mockSetWorkflowSnapshot).not.toHaveBeenCalled();
    expect(mockSetKanbanMultiLoading).not.toHaveBeenLastCalledWith(false);
  });
});

describe("useAllWorkflowSnapshots — snapshot mapping", () => {
  beforeEach(() => {
    resetMocks([{ id: "wf-A", workspaceId: "ws-A", name: "A" }]);
  });

  it("preserves workflow step WIP fields in snapshots", async () => {
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({
      steps: [
        {
          id: "step-1",
          name: "Review",
          position: 1,
          color: "bg-blue-500",
          wip_limit: 2,
          pull_from_step_id: "step-0",
        },
      ],
      tasks: [],
    });

    renderHook(
      ({ workspaceId }: { workspaceId: string | null }) => useAllWorkflowSnapshots(workspaceId),
      { initialProps: { workspaceId: "ws-A" } },
    );

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    expect(mockSetWorkflowSnapshot.mock.calls[0][1].steps[0]).toMatchObject({
      wip_limit: 2,
      pull_from_step_id: "step-0",
    });
  });

  it("preserves a cached autopilot marker when a fresh snapshot omits it", async () => {
    mockState.kanbanMulti.snapshots = {
      "wf-A": {
        workflowId: "wf-A",
        workflowName: "A",
        isPlaceholder: true,
        steps: [],
        tasks: [{ id: "task-1", workflowStepId: "step-1", autopilot: true }],
      },
    };
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({
      steps: [{ id: "step-1", name: "Review", position: 1 }],
      tasks: [{ id: "task-1", workflow_step_id: "step-1", title: "Task" }],
    });

    renderHook(() => useAllWorkflowSnapshots("ws-A"));

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    expect(mockSetWorkflowSnapshot.mock.calls[0][1].tasks[0].autopilot).toBe(true);
  });

  it("keeps an explicit false autopilot value from the fresh snapshot", async () => {
    mockState.kanbanMulti.snapshots = {
      "wf-A": {
        workflowId: "wf-A",
        workflowName: "A",
        isPlaceholder: true,
        steps: [],
        tasks: [{ id: "task-1", workflowStepId: "step-1", autopilot: true }],
      },
    };
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({
      steps: [{ id: "step-1", name: "Review", position: 1 }],
      tasks: [{ id: "task-1", workflow_step_id: "step-1", title: "Task", autopilot: false }],
    });

    renderHook(() => useAllWorkflowSnapshots("ws-A"));

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    expect(mockSetWorkflowSnapshot.mock.calls[0][1].tasks[0].autopilot).toBe(false);
  });
});
