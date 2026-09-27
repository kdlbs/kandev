import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";

const mockUseAllWorkflowSnapshots = vi.fn();
const mockRequestWorkspaceContextRefresh = vi.fn();

type Workflow = { id: string; workspaceId: string; name: string };
type WorkspaceContextRead = {
  workspaceId: string | null;
  snapshotPending: boolean;
  snapshotError: string | null;
  snapshotRequestId: string | null;
};
type MockState = {
  workflows: { items: Workflow[] };
  kanbanMulti: { snapshots: Record<string, unknown> };
  workspaceContextRead: WorkspaceContextRead;
  requestWorkspaceContextRefresh: typeof mockRequestWorkspaceContextRefresh;
};

let mockState: MockState;

function baseState(overrides: Partial<MockState> = {}): MockState {
  return {
    workflows: { items: [] },
    kanbanMulti: { snapshots: {} },
    workspaceContextRead: {
      workspaceId: null,
      snapshotPending: false,
      snapshotError: null,
      snapshotRequestId: null,
    },
    requestWorkspaceContextRefresh: mockRequestWorkspaceContextRefresh,
    ...overrides,
  };
}

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/hooks/domains/kanban/use-all-workflow-snapshots", () => ({
  useAllWorkflowSnapshots: (...args: unknown[]) => mockUseAllWorkflowSnapshots(...args),
}));

import { useCoordinatorTasks } from "./use-coordinator-tasks";

const WORKSPACE_ID = "workspace-1";

beforeEach(() => {
  vi.clearAllMocks();
  mockState = baseState();
});

describe("useCoordinatorTasks - flattening", () => {
  it("flattens tasks across every workflow snapshot of the workspace, with each task's step name", () => {
    mockState = baseState({
      workflows: {
        items: [
          { id: "wf-a", workspaceId: WORKSPACE_ID, name: "A" },
          { id: "wf-b", workspaceId: WORKSPACE_ID, name: "B" },
          { id: "wf-other", workspaceId: "other-workspace", name: "Other" },
        ],
      },
      kanbanMulti: {
        snapshots: {
          "wf-a": {
            steps: [{ id: "step-1", title: "Build" }],
            tasks: [{ id: "t-1", title: "Task 1", workflowStepId: "step-1" }],
          },
          "wf-b": {
            steps: [{ id: "step-2", title: "Review" }],
            tasks: [{ id: "t-2", title: "Task 2", workflowStepId: "unknown-step" }],
          },
          "wf-other": {
            steps: [{ id: "step-3", title: "Ignored" }],
            tasks: [{ id: "t-3", title: "Task 3", workflowStepId: "step-3" }],
          },
        },
      },
    });

    const { result } = renderHook(() => useCoordinatorTasks(WORKSPACE_ID));

    expect(result.current.tasks.map((t) => t.id)).toEqual(["t-1", "t-2"]);
    expect(result.current.stepNameByTaskId.get("t-1")).toBe("Build");
    expect(result.current.stepNameByTaskId.has("t-2")).toBe(false);
    expect(result.current.workflowNameById.get("wf-a")).toBe("A");
    expect(result.current.workflowNameById.get("wf-b")).toBe("B");
    expect(result.current.workflowNameById.has("wf-other")).toBe(false);
    expect(result.current.stepNameByWorkflowStep.get("wf-a:step-1")).toBe("Build");
    expect(result.current.stepNameByWorkflowStep.get("wf-b:step-2")).toBe("Review");
    expect(result.current.stepNameByWorkflowStep.has("wf-other:step-3")).toBe(false);
  });
});

describe("useCoordinatorTasks - error and load time", () => {
  it("reports no error and no load time before the first success", () => {
    mockState = baseState({
      workspaceContextRead: {
        workspaceId: WORKSPACE_ID,
        snapshotPending: true,
        snapshotError: null,
        snapshotRequestId: "req-1",
      },
    });

    const { result } = renderHook(() => useCoordinatorTasks(WORKSPACE_ID));

    expect(result.current.error).toBe(false);
    expect(result.current.loadedAt).toBeUndefined();
  });

  it("reports an error while snapshotError is set for the matching workspace", () => {
    mockState = baseState({
      workspaceContextRead: {
        workspaceId: WORKSPACE_ID,
        snapshotPending: false,
        snapshotError: "transient",
        snapshotRequestId: "req-1",
      },
    });

    const { result } = renderHook(() => useCoordinatorTasks(WORKSPACE_ID));

    expect(result.current.error).toBe(true);
  });

  it("records a load time once a request completes successfully", () => {
    mockState = baseState({
      workspaceContextRead: {
        workspaceId: WORKSPACE_ID,
        snapshotPending: false,
        snapshotError: null,
        snapshotRequestId: "req-1",
      },
    });

    const { result } = renderHook(() => useCoordinatorTasks(WORKSPACE_ID));

    expect(result.current.loadedAt).toBeDefined();
  });

  it("ignores workspaceContextRead for a different workspace", () => {
    mockState = baseState({
      workspaceContextRead: {
        workspaceId: "other-workspace",
        snapshotPending: false,
        snapshotError: "transient",
        snapshotRequestId: "req-1",
      },
    });

    const { result } = renderHook(() => useCoordinatorTasks(WORKSPACE_ID));

    expect(result.current.error).toBe(false);
    expect(result.current.loadedAt).toBeUndefined();
  });
});

describe("useCoordinatorTasks - retry", () => {
  it("calls requestWorkspaceContextRefresh", () => {
    const { result } = renderHook(() => useCoordinatorTasks(WORKSPACE_ID));

    result.current.retry();

    expect(mockRequestWorkspaceContextRefresh).toHaveBeenCalledTimes(1);
  });
});
