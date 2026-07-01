import { describe, it, expect, beforeEach, vi } from "vitest";
import { renderHook } from "@testing-library/react";

const mockUseAllWorkflowSnapshots = vi.fn();

type Snapshot = {
  workflowId: string;
  workflowName: string;
  steps: Array<{ id: string; title: string; color: string; position: number }>;
  tasks: Array<{ id: string; workflowStepId: string; title: string; position: number }>;
};

type MockState = {
  kanbanMulti: {
    snapshots: Record<string, Snapshot>;
    isLoading: boolean;
  };
  workflows: { items: Array<{ id: string; workspaceId: string; name: string; hidden?: boolean }> };
  kanban: {
    workflowId: string | null;
    tasks: Array<{ id: string; workflowStepId: string; title: string; position: number }>;
    steps: Array<{ id: string; title: string; color: string; position: number }>;
  };
};

let mockState: MockState = {
  kanbanMulti: { snapshots: {}, isLoading: false },
  workflows: { items: [] },
  kanban: { workflowId: null, tasks: [], steps: [] },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
  useAppStoreApi: () => ({ getState: () => mockState }),
}));

vi.mock("@/hooks/domains/kanban/use-all-workflow-snapshots", () => ({
  useAllWorkflowSnapshots: (workspaceId: string | null) => mockUseAllWorkflowSnapshots(workspaceId),
}));

import { useWorkspaceSidebarTasks } from "./use-workspace-sidebar-tasks";

function setMockState(patch: Partial<MockState>) {
  mockState = {
    kanbanMulti: { ...mockState.kanbanMulti, ...(patch.kanbanMulti ?? {}) },
    workflows: { ...mockState.workflows, ...(patch.workflows ?? {}) },
    kanban: { ...mockState.kanban, ...(patch.kanban ?? {}) },
  };
}

function makeSnapshot(
  workflowId: string,
  workflowName: string,
  taskIds: string[],
  stepId = "step-1",
): Snapshot {
  return {
    workflowId,
    workflowName,
    steps: [{ id: stepId, title: "Step 1", color: "bg-blue-500", position: 0 }],
    tasks: taskIds.map((id, i) => ({ id, workflowStepId: stepId, title: id, position: i })),
  };
}

describe("useWorkspaceSidebarTasks", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockState = {
      kanbanMulti: { snapshots: {}, isLoading: false },
      workflows: { items: [] },
      kanban: { workflowId: null, tasks: [], steps: [] },
    };
  });

  it("fires useAllWorkflowSnapshots with the workspaceId", () => {
    renderHook(() => useWorkspaceSidebarTasks("ws-1"));
    expect(mockUseAllWorkflowSnapshots).toHaveBeenCalledWith("ws-1");
  });

  it("aggregates tasks from every workflow snapshot scoped to the workspace", () => {
    setMockState({
      workflows: {
        items: [
          { id: "wf-A", workspaceId: "ws-1", name: "Alpha" },
          { id: "wf-B", workspaceId: "ws-1", name: "Beta" },
        ],
      },
      kanbanMulti: {
        snapshots: {
          "wf-A": makeSnapshot("wf-A", "Alpha", ["t-a1", "t-a2"]),
          "wf-B": makeSnapshot("wf-B", "Beta", ["t-b1"]),
        },
        isLoading: false,
      },
    });

    const { result } = renderHook(() => useWorkspaceSidebarTasks("ws-1"));
    const ids = result.current.allTasks.map((t) => t.id);
    expect(ids).toEqual(["t-a1", "t-a2", "t-b1"]);
    // Tagged with their workflow so downstream UI can group.
    expect(result.current.allTasks[0]._workflowId).toBe("wf-A");
    expect(result.current.allTasks[2]._workflowId).toBe("wf-B");
    expect(Object.keys(result.current.stepsByWorkflowId).sort()).toEqual(["wf-A", "wf-B"]);
    expect(result.current.workflows.map((w) => w.id)).toEqual(["wf-A", "wf-B"]);
  });

  it("returns an empty scope when workspaceId is null (no cross-workspace leak)", () => {
    setMockState({
      workflows: {
        items: [
          { id: "wf-A", workspaceId: "ws-1", name: "Alpha" },
          { id: "wf-B", workspaceId: "ws-2", name: "Beta" },
        ],
      },
      kanbanMulti: {
        snapshots: {
          "wf-A": makeSnapshot("wf-A", "Alpha", ["t-a1"]),
          "wf-B": makeSnapshot("wf-B", "Beta", ["t-b1"]),
        },
        isLoading: false,
      },
    });

    const { result } = renderHook(() => useWorkspaceSidebarTasks(null));
    expect(result.current.allTasks).toEqual([]);
    expect(result.current.workflows).toEqual([]);
  });

  it("filters out snapshots from other workspaces (stale hydration)", () => {
    setMockState({
      workflows: {
        items: [
          { id: "wf-A", workspaceId: "ws-1", name: "Alpha" },
          { id: "wf-X", workspaceId: "ws-other", name: "Stale" },
        ],
      },
      kanbanMulti: {
        snapshots: {
          "wf-A": makeSnapshot("wf-A", "Alpha", ["t-a1"]),
          "wf-X": makeSnapshot("wf-X", "Stale", ["t-x1"]),
        },
        isLoading: false,
      },
    });

    const { result } = renderHook(() => useWorkspaceSidebarTasks("ws-1"));
    expect(result.current.allTasks.map((t) => t.id)).toEqual(["t-a1"]);
    expect(result.current.workflows.map((w) => w.id)).toEqual(["wf-A"]);
  });

  it("falls back to the active kanban slice for tasks not yet in snapshots", () => {
    // Snapshot for wf-A hasn't loaded yet, but the page-level useTasks call
    // already populated `kanban.tasks` for the current workflow.
    setMockState({
      workflows: { items: [{ id: "wf-A", workspaceId: "ws-1", name: "Alpha" }] },
      kanbanMulti: { snapshots: {}, isLoading: false },
      kanban: {
        workflowId: "wf-A",
        tasks: [{ id: "t-a1", workflowStepId: "step-1", title: "A1", position: 0 }],
        steps: [{ id: "step-1", title: "Step 1", color: "bg-blue-500", position: 0 }],
      },
    });

    const { result } = renderHook(() => useWorkspaceSidebarTasks("ws-1"));
    expect(result.current.allTasks.map((t) => t.id)).toEqual(["t-a1"]);
    expect(result.current.allTasks[0]._workflowId).toBe("wf-A");
  });
});

describe("useWorkspaceSidebarTasks — loading", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockState = {
      kanbanMulti: { snapshots: {}, isLoading: false },
      workflows: { items: [] },
      kanban: { workflowId: null, tasks: [], steps: [] },
    };
  });

  it("reports loading only on the first fetch, not on refreshes", () => {
    setMockState({
      workflows: { items: [{ id: "wf-A", workspaceId: "ws-1", name: "Alpha" }] },
      kanbanMulti: { snapshots: {}, isLoading: true },
    });
    expect(renderHook(() => useWorkspaceSidebarTasks("ws-1")).result.current.isLoading).toBe(true);

    setMockState({
      kanbanMulti: {
        snapshots: { "wf-A": makeSnapshot("wf-A", "Alpha", ["t-a1"]) },
        isLoading: true,
      },
    });
    expect(renderHook(() => useWorkspaceSidebarTasks("ws-1")).result.current.isLoading).toBe(false);
  });
});
