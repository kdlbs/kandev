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
      { workflowId: string; workflowName: string; steps: []; tasks: SnapshotTask[] }
    >;
    isLoading: boolean;
  };
  clearKanbanMulti: ReturnType<typeof vi.fn>;
  setKanbanMultiLoading: ReturnType<typeof vi.fn>;
  setWorkflowSnapshot: ReturnType<typeof vi.fn>;
};

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
    workflows: { items: [{ id: "wf-A", workspaceId: "ws-A", name: "A" }] },
    kanbanMulti: { snapshots: {}, isLoading: false },
    clearKanbanMulti: mocks.clearKanbanMulti,
    setKanbanMultiLoading: mocks.setKanbanMultiLoading,
    setWorkflowSnapshot: mocks.setWorkflowSnapshot,
  };
}

describe("useAllWorkflowSnapshots in-flight websocket tasks", () => {
  it("does not treat a lightweight websocket snapshot as boot-hydrated", async () => {
    resetState();
    mocks.fetchWorkflowSnapshot.mockResolvedValueOnce({
      steps: [{ id: "step-1", name: "Doing", color: null, position: 0 }],
      tasks: [],
    });
    mocks.state!.kanbanMulti.snapshots["wf-A"] = {
      workflowId: "wf-A",
      workflowName: "A",
      steps: [],
      tasks: [
        {
          id: "child-before-hydration",
          workflowStepId: "step-1",
          title: "Child before hydration",
          position: 0,
          state: "IN_PROGRESS",
        },
      ],
    };

    renderHook(() => useAllWorkflowSnapshots("ws-A"));

    await waitFor(() => expect(mocks.fetchWorkflowSnapshot).toHaveBeenCalledTimes(1));
  });

  it("preserves tasks created while the workflow snapshot fetch is in flight", async () => {
    resetState();
    let resolveFetch: (value: unknown) => void = () => {};
    mocks.fetchWorkflowSnapshot.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveFetch = resolve;
      }),
    );

    renderHook(() => useAllWorkflowSnapshots("ws-A"));
    await waitFor(() =>
      expect(mocks.fetchWorkflowSnapshot).toHaveBeenCalledWith("wf-A", expect.anything()),
    );

    mocks.state!.kanbanMulti.snapshots["wf-A"] = {
      workflowId: "wf-A",
      workflowName: "A",
      steps: [],
      tasks: [
        {
          id: "child-created-during-fetch",
          workflowStepId: "step-1",
          title: "Child created during fetch",
          position: 0,
          state: "IN_PROGRESS",
          parentTaskId: "parent-task",
        },
      ],
    };
    resolveFetch({
      steps: [{ id: "step-1", name: "Doing", color: null, position: 0 }],
      tasks: [],
    });

    await waitFor(() =>
      expect(mocks.setWorkflowSnapshot).toHaveBeenCalledWith(
        "wf-A",
        expect.objectContaining({
          tasks: [
            expect.objectContaining({
              id: "child-created-during-fetch",
              parentTaskId: "parent-task",
            }),
          ],
        }),
      ),
    );
  });
});
