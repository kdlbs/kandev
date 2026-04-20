import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

const mockClearKanbanMulti = vi.fn();
const mockSetKanbanMultiLoading = vi.fn();
const mockSetWorkflowSnapshot = vi.fn();
const mockFetchWorkflowSnapshot = vi.fn();

type Workflow = { id: string; workspaceId: string; name: string };
type MockState = {
  connection: { status: string };
  workflows: { items: Workflow[] };
  kanbanMulti: { snapshots: Record<string, unknown>; isLoading: boolean };
  clearKanbanMulti: typeof mockClearKanbanMulti;
  setKanbanMultiLoading: typeof mockSetKanbanMultiLoading;
  setWorkflowSnapshot: typeof mockSetWorkflowSnapshot;
};

let mockState: MockState = {
  connection: { status: "connected" },
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
    mockState.workflows = { items: [{ id: "wf-B", workspaceId: "ws-B", name: "B" }] };
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
});
