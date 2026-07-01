import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

const mockListWorkflows = vi.fn();
const mockSetWorkflows = vi.fn();

type MockState = {
  workflows: { items: Array<{ id: string; workspaceId: string; name: string }> };
  setWorkflows: typeof mockSetWorkflows;
};

let mockState: MockState = {
  workflows: { items: [] },
  setWorkflows: mockSetWorkflows,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/lib/api", () => ({
  listWorkflows: (...args: unknown[]) => mockListWorkflows(...args),
}));

import { useWorkflows } from "./use-workflows";

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
});
