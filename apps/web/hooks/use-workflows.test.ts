import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

const mockListWorkflows = vi.fn();
const mockSetWorkflows = vi.fn();

type MockState = {
  workflows: { items: Array<{ id: string; workspaceId: string; name: string }> };
  workspaces: { activeId: string | null };
  setWorkflows: typeof mockSetWorkflows;
};

let mockState: MockState = {
  workflows: { items: [] },
  workspaces: { activeId: null },
  setWorkflows: mockSetWorkflows,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
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
    sort_order: 0,
    agent_profile_id: null,
    hidden: false,
    style: null,
  };
}

describe("useWorkflows — stale response guard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockState = {
      workflows: { items: [] },
      workspaces: { activeId: null },
      setWorkflows: mockSetWorkflows,
    };
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

describe("useEnsureWorkspaceWorkflows", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListWorkflows.mockResolvedValue({ workflows: [] });
    mockState = {
      workflows: { items: [] },
      workspaces: { activeId: "ws-A" },
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
});
