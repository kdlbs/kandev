import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";

const mockUseAllWorkflowSnapshots = vi.fn();
const mockRequestWorkspaceContextRefresh = vi.fn();
const mockListWorkflows = vi.fn();
const mockFetchWorkflowSnapshot = vi.fn();

type Workflow = { id: string; workspaceId: string; name: string };
type WorkspaceContextRead = {
  workspaceId: string | null;
  snapshotPending: boolean;
  snapshotError: string | null;
  snapshotRequestId: string | null;
};
type MockState = {
  workspaces: { activeId: string | null };
  workflows: { items: Workflow[] };
  kanbanMulti: { snapshots: Record<string, unknown> };
  workspaceContextRead: WorkspaceContextRead;
  requestWorkspaceContextRefresh: typeof mockRequestWorkspaceContextRefresh;
};

let mockState: MockState;

function baseState(overrides: Partial<MockState> = {}): MockState {
  return {
    workspaces: { activeId: WORKSPACE_ID },
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

vi.mock("@/lib/api/domains/kanban-api", () => ({
  listWorkflows: (...args: unknown[]) => mockListWorkflows(...args),
  fetchWorkflowSnapshot: (...args: unknown[]) => mockFetchWorkflowSnapshot(...args),
}));

import { useCoordinatorTasks } from "./use-coordinator-tasks";

const WORKSPACE_ID = "workspace-1";

beforeEach(() => {
  vi.clearAllMocks();
  mockState = baseState();
});

describe("useCoordinatorTasks - active workspace (live cache)", () => {
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

  it("calls requestWorkspaceContextRefresh on retry", () => {
    const { result } = renderHook(() => useCoordinatorTasks(WORKSPACE_ID));

    result.current.retry();

    expect(mockRequestWorkspaceContextRefresh).toHaveBeenCalledTimes(1);
  });
});

// The route's workspace can differ from the globally active one (a deep link
// or cold load into another workspace) — `workflows.items`/
// `kanbanMulti.snapshots` only ever describe the active workspace, so this
// path must never read them: it fetches directly instead (build round 4's
// fix for the R2/R4/R5 fix-induced chain).
describe("useCoordinatorTasks - non-active workspace (direct fetch)", () => {
  beforeEach(() => {
    mockState = baseState({ workspaces: { activeId: "some-other-active-workspace" } });
  });

  it("never reads the shared workflows/snapshots cache", () => {
    mockListWorkflows.mockResolvedValue({ workflows: [], total: 0 });
    renderHook(() => useCoordinatorTasks(WORKSPACE_ID));
    expect(mockUseAllWorkflowSnapshots).toHaveBeenCalledWith(null);
  });

  it("fetches the route workspace's workflows and snapshots directly, flattening tasks", async () => {
    mockListWorkflows.mockResolvedValue({
      workflows: [{ id: "wf-a", workspace_id: WORKSPACE_ID, name: "A" }],
      total: 1,
    });
    mockFetchWorkflowSnapshot.mockResolvedValue({
      workflow: { id: "wf-a", name: "A" },
      steps: [{ id: "step-1", name: "Build" }],
      tasks: [
        {
          id: "t-1",
          title: "Task 1",
          state: "in_progress",
          workflow_step_id: "step-1",
          updated_at: "2026-09-27T00:00:00Z",
        },
      ],
    });

    const { result } = renderHook(() => useCoordinatorTasks(WORKSPACE_ID));

    await waitFor(() => expect(result.current.loadedAt).toBeDefined());

    expect(mockListWorkflows).toHaveBeenCalledWith(WORKSPACE_ID);
    expect(result.current.tasks.map((t) => t.id)).toEqual(["t-1"]);
    expect(result.current.stepNameByTaskId.get("t-1")).toBe("Build");
    expect(result.current.workflowNameById.get("wf-a")).toBe("A");
    expect(result.current.stepNameByWorkflowStep.get("wf-a:step-1")).toBe("Build");
    expect(result.current.error).toBe(false);
  });

  it("drops ephemeral tasks and tasks whose step is not in the snapshot", async () => {
    mockListWorkflows.mockResolvedValue({
      workflows: [{ id: "wf-a", workspace_id: WORKSPACE_ID, name: "A" }],
      total: 1,
    });
    mockFetchWorkflowSnapshot.mockResolvedValue({
      workflow: { id: "wf-a", name: "A" },
      steps: [{ id: "step-1", name: "Build" }],
      tasks: [
        { id: "t-ephemeral", title: "Ephemeral", workflow_step_id: "step-1", is_ephemeral: true },
        { id: "t-unknown-step", title: "Unknown step", workflow_step_id: "unknown-step" },
        { id: "t-1", title: "Task 1", workflow_step_id: "step-1" },
      ],
    });

    const { result } = renderHook(() => useCoordinatorTasks(WORKSPACE_ID));

    await waitFor(() => expect(result.current.loadedAt).toBeDefined());

    expect(result.current.tasks.map((t) => t.id)).toEqual(["t-1"]);
  });

  it("reports an error when the direct fetch fails", async () => {
    mockListWorkflows.mockRejectedValue(new Error("network error"));

    const { result } = renderHook(() => useCoordinatorTasks(WORKSPACE_ID));

    await waitFor(() => expect(result.current.error).toBe(true));
    expect(result.current.loadedAt).toBeUndefined();
  });

  it("re-fetches on retry", async () => {
    mockListWorkflows.mockResolvedValue({ workflows: [], total: 0 });
    const { result } = renderHook(() => useCoordinatorTasks(WORKSPACE_ID));
    await waitFor(() => expect(result.current.loadedAt).toBeDefined());

    expect(mockListWorkflows).toHaveBeenCalledTimes(1);
    act(() => {
      result.current.retry();
    });
    await waitFor(() => expect(mockListWorkflows).toHaveBeenCalledTimes(2));
  });
});

// Review round 4, R6/R7: split from the describe block above to keep each
// test function under the file's line limit.
describe("useCoordinatorTasks - non-active workspace transitions and partial failure", () => {
  beforeEach(() => {
    mockState = baseState({ workspaces: { activeId: "some-other-active-workspace" } });
  });

  // R6: a coordinator navigation between two different non-active workspaces
  // reuses the same mounted hook instance (no `key` remounts
  // `CoordinatorRoute`), so the prior workspace's tasks must not linger —
  // not even transiently, and not if the new workspace's own fetch then
  // fails.
  it("clears stale tasks when the route workspace changes to a different non-active workspace, even if the new fetch fails", async () => {
    mockListWorkflows.mockResolvedValueOnce({
      workflows: [{ id: "wf-a", workspace_id: "workspace-a", name: "A" }],
      total: 1,
    });
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({
      workflow: { id: "wf-a", name: "A" },
      steps: [{ id: "step-1", name: "Build" }],
      tasks: [{ id: "t-a", title: "Task A", workflow_step_id: "step-1" }],
    });

    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string }) => useCoordinatorTasks(workspaceId),
      { initialProps: { workspaceId: "workspace-a" } },
    );
    await waitFor(() => expect(result.current.tasks.map((t) => t.id)).toEqual(["t-a"]));

    let rejectB: (err: Error) => void = () => {};
    mockListWorkflows.mockImplementationOnce(
      () =>
        new Promise((_resolve, reject) => {
          rejectB = reject;
        }),
    );

    rerender({ workspaceId: "workspace-b" });

    expect(result.current.tasks).toEqual([]);
    expect(result.current.loadedAt).toBeUndefined();

    await act(async () => {
      rejectB(new Error("network error"));
    });

    await waitFor(() => expect(result.current.error).toBe(true));
    expect(result.current.tasks).toEqual([]);
    expect(result.current.loadedAt).toBeUndefined();
  });

  // R7 (Review round 4): the tasks input's documented failure contract
  // (docs/specs/coordinator/system-design/needs-you.md#failure-and-recovery)
  // requires a partial failure to still show the workflows that loaded.
  it("keeps a sibling workflow's tasks when one workflow's snapshot fetch fails", async () => {
    mockListWorkflows.mockResolvedValue({
      workflows: [
        { id: "wf-ok", workspace_id: WORKSPACE_ID, name: "OK" },
        { id: "wf-bad", workspace_id: WORKSPACE_ID, name: "Bad" },
      ],
      total: 2,
    });
    mockFetchWorkflowSnapshot.mockImplementation((workflowId: string) => {
      if (workflowId === "wf-bad") return Promise.reject(new Error("snapshot fetch failed"));
      return Promise.resolve({
        workflow: { id: "wf-ok", name: "OK" },
        steps: [{ id: "step-1", name: "Build" }],
        tasks: [{ id: "t-ok", title: "Task OK", workflow_step_id: "step-1" }],
      });
    });

    const { result } = renderHook(() => useCoordinatorTasks(WORKSPACE_ID));

    await waitFor(() => expect(result.current.error).toBe(true));
    expect(result.current.tasks.map((t) => t.id)).toEqual(["t-ok"]);
    expect(result.current.workflowNameById.get("wf-ok")).toBe("OK");
    expect(result.current.loadedAt).toBeDefined();
  });

  // R8 (Review round 5): a same-workspace `retry()` re-runs the fetch effect
  // via `retryNonce` alone, with `workspaceId` unchanged. That must not reset
  // state before the retried fetch starts — otherwise a retry whose own
  // fetch then fails permanently loses the previously-loaded tasks/loadedAt,
  // violating the documented failure/recovery contract ("a read that fails
  // again keeps the previous value and loadedAt").
  it("keeps the prior successful tasks and loadedAt when a same-workspace retry itself fails", async () => {
    mockListWorkflows.mockResolvedValueOnce({
      workflows: [{ id: "wf-a", workspace_id: WORKSPACE_ID, name: "A" }],
      total: 1,
    });
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({
      workflow: { id: "wf-a", name: "A" },
      steps: [{ id: "step-1", name: "Build" }],
      tasks: [{ id: "t-1", title: "Task 1", workflow_step_id: "step-1" }],
    });

    const { result } = renderHook(() => useCoordinatorTasks(WORKSPACE_ID));
    await waitFor(() => expect(result.current.tasks.map((t) => t.id)).toEqual(["t-1"]));
    const loadedAtBeforeRetry = result.current.loadedAt;
    expect(loadedAtBeforeRetry).toBeDefined();

    mockListWorkflows.mockRejectedValueOnce(new Error("network error"));
    act(() => {
      result.current.retry();
    });

    await waitFor(() => expect(result.current.error).toBe(true));
    expect(result.current.tasks.map((t) => t.id)).toEqual(["t-1"]);
    expect(result.current.loadedAt).toBe(loadedAtBeforeRetry);
  });
});
