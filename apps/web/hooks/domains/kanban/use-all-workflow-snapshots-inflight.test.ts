import { describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

type Workflow = { id: string; workspaceId: string; name: string };
type SnapshotTask = {
  id: string;
  workflowStepId: string;
  title: string;
  position: number;
  state: "IN_PROGRESS";
  parentTaskId?: string;
};
type MockState = {
  connection: { status: string };
  workflows: { items: Workflow[] };
  kanbanMulti: {
    snapshots: Record<
      string,
      {
        workflowId: string;
        workflowName: string;
        steps: [];
        tasks: SnapshotTask[];
        isPlaceholder?: boolean;
      }
    >;
    isLoading: boolean;
  };
  clearKanbanMulti: ReturnType<typeof vi.fn>;
  setKanbanMultiLoading: ReturnType<typeof vi.fn>;
  setWorkflowSnapshot: ReturnType<typeof vi.fn>;
};

const WORKFLOW_ID = "wf-A";
const WORKSPACE_ID = "ws-A";
const STEP_ID = "step-1";
const PARENT_TASK_ID = "parent-task";

const mocks = vi.hoisted(() => ({
  clearKanbanMulti: vi.fn(),
  fetchWorkflowSnapshot: vi.fn(),
  setKanbanMultiLoading: vi.fn(),
  setWorkflowSnapshot: vi.fn(),
  state: undefined as MockState | undefined,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mocks.state!),
  useAppStoreApi: () => ({ getState: () => mocks.state! }),
}));

vi.mock("@/lib/api", () => ({
  fetchWorkflowSnapshot: (...args: unknown[]) => mocks.fetchWorkflowSnapshot(...args),
}));

import { useAllWorkflowSnapshots } from "./use-all-workflow-snapshots";

function resetState() {
  vi.clearAllMocks();
  mocks.state = {
    connection: { status: "connected" },
    workflows: { items: [{ id: WORKFLOW_ID, workspaceId: WORKSPACE_ID, name: "A" }] },
    kanbanMulti: { snapshots: {}, isLoading: false },
    clearKanbanMulti: mocks.clearKanbanMulti,
    setKanbanMultiLoading: mocks.setKanbanMultiLoading,
    setWorkflowSnapshot: mocks.setWorkflowSnapshot,
  };
}

function staleSnapshotResponse() {
  return {
    steps: [{ id: STEP_ID, name: "Doing", color: null, position: 0 }],
    tasks: [],
  };
}

function setLightweightSnapshot(task: SnapshotTask) {
  mocks.state!.kanbanMulti.snapshots[WORKFLOW_ID] = {
    workflowId: WORKFLOW_ID,
    workflowName: "A",
    steps: [],
    tasks: [task],
    isPlaceholder: true,
  };
}

describe("useAllWorkflowSnapshots lightweight snapshots", () => {
  it("does not treat a lightweight websocket snapshot as boot-hydrated", async () => {
    resetState();
    mocks.fetchWorkflowSnapshot.mockResolvedValueOnce(staleSnapshotResponse());
    setLightweightSnapshot({
      id: "child-before-hydration",
      workflowStepId: STEP_ID,
      title: "Child before hydration",
      position: 0,
      state: "IN_PROGRESS",
    });

    renderHook(() => useAllWorkflowSnapshots(WORKSPACE_ID));

    await waitFor(() => expect(mocks.fetchWorkflowSnapshot).toHaveBeenCalledTimes(1));
  });

  it("preserves tasks already present in a lightweight snapshot before fetch starts", async () => {
    resetState();
    mocks.fetchWorkflowSnapshot.mockResolvedValueOnce(staleSnapshotResponse());
    setLightweightSnapshot({
      id: "child-before-fetch",
      workflowStepId: STEP_ID,
      title: "Child before fetch",
      position: 0,
      state: "IN_PROGRESS",
      parentTaskId: PARENT_TASK_ID,
    });

    renderHook(() => useAllWorkflowSnapshots(WORKSPACE_ID));

    await waitFor(() =>
      expect(mocks.setWorkflowSnapshot).toHaveBeenCalledWith(
        WORKFLOW_ID,
        expect.objectContaining({
          tasks: [
            expect.objectContaining({
              id: "child-before-fetch",
              parentTaskId: PARENT_TASK_ID,
            }),
          ],
        }),
      ),
    );
  });
});

describe("useAllWorkflowSnapshots in-flight websocket tasks", () => {
  it("preserves tasks created while the workflow snapshot fetch is in flight", async () => {
    resetState();
    let resolveFetch: (value: unknown) => void = () => {};
    mocks.fetchWorkflowSnapshot.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveFetch = resolve;
      }),
    );

    renderHook(() => useAllWorkflowSnapshots(WORKSPACE_ID));
    await waitFor(() =>
      expect(mocks.fetchWorkflowSnapshot).toHaveBeenCalledWith(WORKFLOW_ID, expect.anything()),
    );

    setLightweightSnapshot({
      id: "child-created-during-fetch",
      workflowStepId: STEP_ID,
      title: "Child created during fetch",
      position: 0,
      state: "IN_PROGRESS",
      parentTaskId: PARENT_TASK_ID,
    });
    resolveFetch(staleSnapshotResponse());

    await waitFor(() =>
      expect(mocks.setWorkflowSnapshot).toHaveBeenCalledWith(
        WORKFLOW_ID,
        expect.objectContaining({
          tasks: [
            expect.objectContaining({
              id: "child-created-during-fetch",
              parentTaskId: PARENT_TASK_ID,
            }),
          ],
        }),
      ),
    );
  });
});
