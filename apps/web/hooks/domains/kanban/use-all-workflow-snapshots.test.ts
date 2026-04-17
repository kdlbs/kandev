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
  clearKanbanMulti: typeof mockClearKanbanMulti;
  setKanbanMultiLoading: typeof mockSetKanbanMultiLoading;
  setWorkflowSnapshot: typeof mockSetWorkflowSnapshot;
};

let mockState: MockState = {
  connection: { status: "connected" },
  workflows: { items: [] },
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

    // New array reference with identical contents — dedup key should match.
    mockState.workflows = { items: [...workflows] };
    rerender({ workspaceId: "ws-A" });
    // Wait long enough that any queued effect would have run and issued a
    // second fetch; then assert the count is unchanged.
    await new Promise((r) => setTimeout(r, 50));

    expect(mockFetchWorkflowSnapshot).toHaveBeenCalledTimes(1);
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

    // Switch to workspace B before A's fetch resolves.
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({ steps: [], tasks: [] });
    mockState.workflows = { items: [{ id: "wf-B", workspaceId: "ws-B", name: "B" }] };
    rerender({ workspaceId: "ws-B" });
    await waitFor(() => expect(mockClearKanbanMulti).toHaveBeenCalledTimes(1));

    // Now let workspace A's fetch finally resolve. Its write must be dropped.
    resolveStale({ steps: [], tasks: [] });
    await new Promise((r) => setTimeout(r, 50));

    const writtenIds = mockSetWorkflowSnapshot.mock.calls.map((args) => args[0]);
    expect(writtenIds).not.toContain("wf-A");
  });
});
