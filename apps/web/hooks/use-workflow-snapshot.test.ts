import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

const mockHydrate = vi.fn();
const mockFetchWorkflowSnapshot = vi.fn();
const mockSetState = vi.fn();

type MockKanban = {
  workflowId: string | null;
  tasks: unknown[];
  steps: unknown[];
  isLoading?: boolean;
};
type MockState = {
  connection: { status: string };
  kanban: MockKanban;
  hydrate: typeof mockHydrate;
};

let mockState: MockState = {
  connection: { status: "connected" },
  kanban: { workflowId: null, tasks: [], steps: [], isLoading: false },
  hydrate: mockHydrate,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
  useAppStoreApi: () => ({
    getState: () => mockState,
    setState: (updater: (s: MockState) => MockState) => {
      const next = updater(mockState);
      mockSetState(next);
      mockState = next;
    },
  }),
}));

vi.mock("@/lib/api", () => ({
  fetchWorkflowSnapshot: (...args: unknown[]) => mockFetchWorkflowSnapshot(...args),
}));

vi.mock("@/lib/ssr/mapper", () => ({
  snapshotToState: (snapshot: unknown) => ({ snapshot }),
}));

import { useWorkflowSnapshot } from "./use-workflow-snapshot";

function resetState(kanban: Partial<MockKanban> = {}) {
  vi.clearAllMocks();
  mockState = {
    connection: { status: "connected" },
    kanban: {
      workflowId: null,
      tasks: [],
      steps: [],
      isLoading: false,
      ...kanban,
    },
    hydrate: mockHydrate,
  };
}

describe("useWorkflowSnapshot — kanban.isLoading", () => {
  beforeEach(() => resetState());

  it("flips isLoading true while a fetch for an un-hydrated workflow is in flight", () => {
    mockFetchWorkflowSnapshot.mockReturnValue(new Promise(() => {}));
    renderHook(() => useWorkflowSnapshot("wf-1"));
    expect(mockState.kanban.isLoading).toBe(true);
  });

  it("flips isLoading back to false after the fetch resolves", async () => {
    mockFetchWorkflowSnapshot.mockResolvedValue({ steps: [], tasks: [] });
    renderHook(() => useWorkflowSnapshot("wf-1"));
    await waitFor(() => expect(mockHydrate).toHaveBeenCalled());
    await waitFor(() => expect(mockState.kanban.isLoading).toBe(false));
  });

  it("flips isLoading back to false after the fetch rejects", async () => {
    mockFetchWorkflowSnapshot.mockRejectedValue(new Error("nope"));
    renderHook(() => useWorkflowSnapshot("wf-1"));
    await waitFor(() => expect(mockState.kanban.isLoading).toBe(false));
    expect(mockHydrate).not.toHaveBeenCalled();
  });

  it("does not flip isLoading true if the requested workflow is already hydrated", () => {
    resetState({ workflowId: "wf-1", isLoading: false });
    mockFetchWorkflowSnapshot.mockReturnValue(new Promise(() => {}));
    renderHook(() => useWorkflowSnapshot("wf-1"));
    expect(mockState.kanban.isLoading).toBe(false);
  });

  it("does nothing when workflowId is null", () => {
    renderHook(() => useWorkflowSnapshot(null));
    expect(mockFetchWorkflowSnapshot).not.toHaveBeenCalled();
    expect(mockState.kanban.isLoading).toBe(false);
  });
});
