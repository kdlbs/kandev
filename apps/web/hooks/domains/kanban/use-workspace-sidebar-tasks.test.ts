import { describe, it, expect, beforeEach, vi } from "vitest";
import { renderHook } from "@testing-library/react";

const mockUseAllWorkflowSnapshots = vi.fn();
let mockWorkflows: Array<{ id: string; workspaceId: string; name: string; hidden?: boolean }> = [];

type Snapshot = {
  workflowId: string;
  workflowName: string;
  steps: Array<{ id: string; title: string; color: string; position: number }>;
  tasks: Array<{ id: string; workflowStepId: string; title: string; position: number }>;
};

vi.mock("@/hooks/domains/kanban/use-all-workflow-snapshots", () => ({
  useAllWorkflowSnapshots: (workspaceId: string | null) => mockUseAllWorkflowSnapshots(workspaceId),
}));

vi.mock("@/hooks/use-workflow-cache", () => ({
  useCachedWorkflows: (workspaceId: string | null) =>
    workspaceId ? mockWorkflows.filter((workflow) => workflow.workspaceId === workspaceId) : [],
}));

import { useWorkspaceSidebarTasks } from "./use-workspace-sidebar-tasks";

function setSnapshotResult(snapshots: Record<string, Snapshot>, isLoading = false) {
  mockUseAllWorkflowSnapshots.mockReturnValue({ snapshots, isLoading });
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
    setSnapshotResult({});
    mockWorkflows = [];
  });

  it("fires useAllWorkflowSnapshots with the workspaceId", () => {
    renderHook(() => useWorkspaceSidebarTasks("ws-1"));
    expect(mockUseAllWorkflowSnapshots).toHaveBeenCalledWith("ws-1");
  });

  it("aggregates tasks from every workflow snapshot scoped to the workspace", () => {
    setSnapshotResult({
      "wf-A": makeSnapshot("wf-A", "Alpha", ["t-a1", "t-a2"]),
      "wf-B": makeSnapshot("wf-B", "Beta", ["t-b1"]),
    });
    mockWorkflows = [
      { id: "wf-A", workspaceId: "ws-1", name: "Alpha" },
      { id: "wf-B", workspaceId: "ws-1", name: "Beta" },
    ];

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
    setSnapshotResult({
      "wf-A": makeSnapshot("wf-A", "Alpha", ["t-a1"]),
      "wf-B": makeSnapshot("wf-B", "Beta", ["t-b1"]),
    });
    mockWorkflows = [
      { id: "wf-A", workspaceId: "ws-1", name: "Alpha" },
      { id: "wf-B", workspaceId: "ws-2", name: "Beta" },
    ];

    const { result } = renderHook(() => useWorkspaceSidebarTasks(null));
    expect(result.current.allTasks).toEqual([]);
    expect(result.current.workflows).toEqual([]);
  });

  it("filters out snapshots from other workspaces (stale hydration)", () => {
    setSnapshotResult({
      "wf-A": makeSnapshot("wf-A", "Alpha", ["t-a1"]),
      "wf-X": makeSnapshot("wf-X", "Stale", ["t-x1"]),
    });
    mockWorkflows = [
      { id: "wf-A", workspaceId: "ws-1", name: "Alpha" },
      { id: "wf-X", workspaceId: "ws-other", name: "Stale" },
    ];

    const { result } = renderHook(() => useWorkspaceSidebarTasks("ws-1"));
    expect(result.current.allTasks.map((t) => t.id)).toEqual(["t-a1"]);
    expect(result.current.workflows.map((w) => w.id)).toEqual(["wf-A"]);
  });

  it("waits for Query snapshots instead of falling back to the old active kanban mirror", () => {
    mockWorkflows = [{ id: "wf-A", workspaceId: "ws-1", name: "Alpha" }];

    const { result } = renderHook(() => useWorkspaceSidebarTasks("ws-1"));

    expect(result.current.allTasks).toEqual([]);
  });
});

describe("useWorkspaceSidebarTasks — loading", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setSnapshotResult({});
    mockWorkflows = [];
  });

  it("reports loading only on the first fetch, not on refreshes", () => {
    setSnapshotResult({}, true);
    mockWorkflows = [{ id: "wf-A", workspaceId: "ws-1", name: "Alpha" }];
    expect(renderHook(() => useWorkspaceSidebarTasks("ws-1")).result.current.isLoading).toBe(true);

    setSnapshotResult({ "wf-A": makeSnapshot("wf-A", "Alpha", ["t-a1"]) }, true);
    expect(renderHook(() => useWorkspaceSidebarTasks("ws-1")).result.current.isLoading).toBe(false);
  });
});
