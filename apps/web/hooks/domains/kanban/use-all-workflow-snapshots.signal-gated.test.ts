import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

const WORKSPACE_ID = "ws-A";
const WORKFLOW_ID = "wf-A";
const mockSetWorkflowSnapshot = vi.fn();
const mockFetchWorkflowSnapshot = vi.fn();

type Workflow = { id: string; workspaceId: string; name: string };
type MockState = {
  connection: { status: string };
  workspaces: { activeId: string | null };
  workspaceContextGeneration: number;
  workflows: { items: Workflow[] };
  kanbanMulti: { snapshots: Record<string, unknown>; isLoading: boolean };
  clearKanbanMulti: () => void;
  setKanbanMultiLoading: () => void;
  setWorkflowSnapshot: typeof mockSetWorkflowSnapshot;
};

let mockState: MockState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
  useAppStoreApi: () => ({ getState: () => mockState }),
}));

vi.mock("@/lib/api", () => ({
  fetchWorkflowSnapshot: (...args: unknown[]) => mockFetchWorkflowSnapshot(...args),
}));

import { useAllWorkflowSnapshots } from "./use-all-workflow-snapshots";

beforeEach(() => {
  vi.clearAllMocks();
  mockFetchWorkflowSnapshot.mockResolvedValue({ steps: [], tasks: [] });
  mockState = {
    connection: { status: "connected" },
    workspaces: { activeId: WORKSPACE_ID },
    workspaceContextGeneration: 0,
    workflows: { items: [{ id: WORKFLOW_ID, workspaceId: WORKSPACE_ID, name: "A" }] },
    kanbanMulti: { snapshots: {}, isLoading: false },
    clearKanbanMulti: vi.fn(),
    setKanbanMultiLoading: vi.fn(),
    setWorkflowSnapshot: mockSetWorkflowSnapshot,
  };
});

describe("useAllWorkflowSnapshots signal-gated mapping", () => {
  it("preserves the signal-gated flag in refreshed workflow snapshots", async () => {
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({
      steps: [{ id: "step-1", name: "Review", position: 1, auto_advance_requires_signal: true }],
      tasks: [],
    });

    renderHook(() => useAllWorkflowSnapshots(WORKSPACE_ID));

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    expect(mockSetWorkflowSnapshot.mock.calls[0][1].steps[0]).toMatchObject({
      auto_advance_requires_signal: true,
    });
  });
});
