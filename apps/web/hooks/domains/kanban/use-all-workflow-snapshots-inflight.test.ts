import { describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";

type Workflow = { id: string; workspaceId: string; name: string };
type SnapshotTask = {
  id: string;
  workflowStepId: string;
  title: string;
  position: number;
  state: "IN_PROGRESS" | "REVIEW";
  updatedAt?: string;
  autoStartFailed?: boolean;
  parentTaskId?: string;
  statusSummary?: {
    revision: number;
    updated_at: string;
    primary_session?: { id: string; state: "RUNNING" | "WAITING_FOR_INPUT" };
  };
};
type MockState = {
  connection: { status: string };
  workflows: { items: Workflow[] };
  workspaces: { activeId: string | null };
  workspaceContextGeneration: number;
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
const LIVE_STEP_ID = "step-2";
const PARENT_TASK_ID = "parent-task";
const LIVE_PLACEMENT_TASK_ID = "task-with-live-placement";
const INVALIDATED_STATUS_TASK_ID = "task-with-invalidated-status";
const LIVE_STATE_TASK_ID = "task-with-live-state";
const LIVE_TASK_UPDATED_AT = "2026-08-29T09:14:02Z";

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
    workspaces: { activeId: WORKSPACE_ID },
    workspaceContextGeneration: 0,
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

  it("keeps a newer live status summary when an older snapshot finishes later", async () => {
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
      id: "task-with-live-status",
      workflowStepId: STEP_ID,
      title: "Live status",
      position: 0,
      state: "IN_PROGRESS",
      statusSummary: {
        revision: 4,
        updated_at: "2026-08-13T00:00:04Z",
        primary_session: { id: "session-1", state: "WAITING_FOR_INPUT" },
      },
    });
    resolveFetch({
      steps: [{ id: STEP_ID, name: "Doing", color: null, position: 0 }],
      tasks: [
        {
          id: "task-with-live-status",
          workflow_step_id: STEP_ID,
          title: "Live status",
          position: 0,
          state: "IN_PROGRESS",
          status_summary: {
            revision: 3,
            updated_at: "2026-08-13T00:00:03Z",
            primary_session: { id: "session-1", state: "RUNNING" },
          },
        },
      ],
    });

    await waitFor(() => expect(mocks.setWorkflowSnapshot).toHaveBeenCalled());
    expect(mocks.setWorkflowSnapshot).toHaveBeenCalledWith(
      WORKFLOW_ID,
      expect.objectContaining({
        tasks: [
          expect.objectContaining({
            id: "task-with-live-status",
            statusSummary: expect.objectContaining({ revision: 4 }),
          }),
        ],
      }),
    );
  });
});

describe("useAllWorkflowSnapshots task state races", () => {
  it("keeps a newer live task state when an older snapshot finishes later", async () => {
    // @covers AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.3
    resetState();
    let resolveFetch: (value: unknown) => void = () => {};
    mocks.fetchWorkflowSnapshot.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveFetch = resolve;
      }),
    );
    setLightweightSnapshot({
      id: LIVE_STATE_TASK_ID,
      workflowStepId: STEP_ID,
      title: "Live state",
      position: 0,
      state: "REVIEW",
      updatedAt: "2026-08-29T09:14:00Z",
    });

    renderHook(() => useAllWorkflowSnapshots(WORKSPACE_ID));
    await waitFor(() =>
      expect(mocks.fetchWorkflowSnapshot).toHaveBeenCalledWith(WORKFLOW_ID, expect.anything()),
    );

    setLightweightSnapshot({
      id: LIVE_STATE_TASK_ID,
      workflowStepId: STEP_ID,
      title: "Live state",
      position: 0,
      state: "IN_PROGRESS",
      updatedAt: LIVE_TASK_UPDATED_AT,
    });
    resolveFetch({
      steps: [{ id: STEP_ID, name: "Doing", color: null, position: 0 }],
      tasks: [
        {
          id: LIVE_STATE_TASK_ID,
          workflow_step_id: STEP_ID,
          title: "Live state",
          position: 0,
          state: "REVIEW",
          updated_at: "2026-08-29T09:14:01Z",
        },
      ],
    });

    await waitFor(() => expect(mocks.setWorkflowSnapshot).toHaveBeenCalled());
    expect(mocks.setWorkflowSnapshot).toHaveBeenCalledWith(
      WORKFLOW_ID,
      expect.objectContaining({
        tasks: [
          expect.objectContaining({
            id: LIVE_STATE_TASK_ID,
            state: "IN_PROGRESS",
            updatedAt: LIVE_TASK_UPDATED_AT,
          }),
        ],
      }),
    );
  });

  it("accepts a newer snapshot task state", async () => {
    resetState();
    setLightweightSnapshot({
      id: "task-with-newer-snapshot-state",
      workflowStepId: STEP_ID,
      title: "Newer snapshot state",
      position: 0,
      state: "IN_PROGRESS",
      updatedAt: LIVE_TASK_UPDATED_AT,
    });
    mocks.fetchWorkflowSnapshot.mockResolvedValueOnce({
      steps: [{ id: STEP_ID, name: "Doing", color: null, position: 0 }],
      tasks: [
        {
          id: "task-with-newer-snapshot-state",
          workflow_step_id: STEP_ID,
          title: "Newer snapshot state",
          position: 0,
          state: "REVIEW",
          updated_at: "2026-08-29T09:14:03Z",
        },
      ],
    });

    renderHook(() => useAllWorkflowSnapshots(WORKSPACE_ID));

    await waitFor(() => expect(mocks.setWorkflowSnapshot).toHaveBeenCalled());
    expect(mocks.setWorkflowSnapshot).toHaveBeenCalledWith(
      WORKFLOW_ID,
      expect.objectContaining({
        tasks: [
          expect.objectContaining({
            id: "task-with-newer-snapshot-state",
            state: "REVIEW",
            updatedAt: "2026-08-29T09:14:03Z",
          }),
        ],
      }),
    );
  });
});

describe("useAllWorkflowSnapshots failed refreshes", () => {
  it("keeps the current task projection usable when a snapshot refresh fails", async () => {
    // @covers AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.7
    resetState();
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    setLightweightSnapshot({
      id: LIVE_STATE_TASK_ID,
      workflowStepId: STEP_ID,
      title: "Current live state",
      position: 0,
      state: "IN_PROGRESS",
      updatedAt: LIVE_TASK_UPDATED_AT,
    });
    mocks.fetchWorkflowSnapshot.mockRejectedValueOnce(new Error("snapshot unavailable"));

    renderHook(() => useAllWorkflowSnapshots(WORKSPACE_ID));

    await waitFor(() => expect(mocks.setWorkflowSnapshot).toHaveBeenCalled());
    expect(mocks.setWorkflowSnapshot).toHaveBeenCalledWith(
      WORKFLOW_ID,
      expect.objectContaining({
        isPlaceholder: false,
        tasks: [
          expect.objectContaining({
            id: LIVE_STATE_TASK_ID,
            state: "IN_PROGRESS",
          }),
        ],
      }),
    );
    errorSpy.mockRestore();
  });

  it("keeps a resolved task projection when a snapshot refresh fails", async () => {
    resetState();
    const resolvedSnapshot: MockState["kanbanMulti"]["snapshots"][string] = {
      workflowId: WORKFLOW_ID,
      workflowName: "A",
      steps: [],
      tasks: [
        {
          id: LIVE_STATE_TASK_ID,
          workflowStepId: STEP_ID,
          title: "Current resolved state",
          position: 0,
          state: "IN_PROGRESS" as const,
          updatedAt: LIVE_TASK_UPDATED_AT,
        },
      ],
      isPlaceholder: false,
    };
    mocks.state!.kanbanMulti.snapshots[WORKFLOW_ID] = resolvedSnapshot;
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    mocks.fetchWorkflowSnapshot.mockRejectedValueOnce(new Error("snapshot unavailable"));

    renderHook(() => useAllWorkflowSnapshots(WORKSPACE_ID));
    act(() => window.dispatchEvent(new Event("focus")));

    await waitFor(() => expect(errorSpy).toHaveBeenCalled());
    expect(mocks.setWorkflowSnapshot).not.toHaveBeenCalled();
    expect(mocks.state!.kanbanMulti.snapshots[WORKFLOW_ID]).toBe(resolvedSnapshot);
    errorSpy.mockRestore();
  });
});

describe("useAllWorkflowSnapshots auto-start marker races", () => {
  it("keeps a newer live auto-start marker when an older snapshot finishes later", async () => {
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
      id: "task-with-live-auto-start-marker",
      workflowStepId: STEP_ID,
      title: "Live auto-start marker",
      position: 0,
      state: "IN_PROGRESS",
      autoStartFailed: true,
    });
    resolveFetch({
      steps: [{ id: STEP_ID, name: "Doing", color: null, position: 0 }],
      tasks: [
        {
          id: "task-with-live-auto-start-marker",
          workflow_step_id: STEP_ID,
          title: "Live auto-start marker",
          position: 0,
          state: "IN_PROGRESS",
          auto_start_failed: false,
        },
      ],
    });

    await waitFor(() => expect(mocks.setWorkflowSnapshot).toHaveBeenCalled());
    expect(mocks.setWorkflowSnapshot).toHaveBeenCalledWith(
      WORKFLOW_ID,
      expect.objectContaining({
        tasks: [
          expect.objectContaining({
            id: "task-with-live-auto-start-marker",
            autoStartFailed: true,
          }),
        ],
      }),
    );
  });
});

describe("useAllWorkflowSnapshots status invalidation races", () => {
  it("keeps a live summary that advanced while an invalidating snapshot was in flight", async () => {
    resetState();
    let resolveFetch: (value: unknown) => void = () => {};
    mocks.fetchWorkflowSnapshot.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveFetch = resolve;
      }),
    );
    setLightweightSnapshot({
      id: INVALIDATED_STATUS_TASK_ID,
      workflowStepId: STEP_ID,
      title: "Invalidated status",
      position: 0,
      state: "IN_PROGRESS",
      statusSummary: {
        revision: 4,
        updated_at: "2026-08-13T00:00:04Z",
      },
    });

    renderHook(() => useAllWorkflowSnapshots(WORKSPACE_ID));
    await waitFor(() =>
      expect(mocks.fetchWorkflowSnapshot).toHaveBeenCalledWith(WORKFLOW_ID, expect.anything()),
    );

    setLightweightSnapshot({
      id: INVALIDATED_STATUS_TASK_ID,
      workflowStepId: STEP_ID,
      title: "Invalidated status",
      position: 0,
      state: "IN_PROGRESS",
      statusSummary: {
        revision: 5,
        updated_at: "2026-08-13T00:00:05Z",
      },
    });
    resolveFetch({
      steps: [{ id: STEP_ID, name: "Doing", color: null, position: 0 }],
      tasks: [
        {
          id: INVALIDATED_STATUS_TASK_ID,
          workflow_step_id: STEP_ID,
          title: "Invalidated status",
          position: 0,
          state: "IN_PROGRESS",
          status_summary_invalidated: true,
        },
      ],
    });

    await waitFor(() => expect(mocks.setWorkflowSnapshot).toHaveBeenCalled());
    expect(mocks.setWorkflowSnapshot).toHaveBeenCalledWith(
      WORKFLOW_ID,
      expect.objectContaining({
        tasks: [
          expect.objectContaining({
            id: INVALIDATED_STATUS_TASK_ID,
            statusSummary: expect.objectContaining({ revision: 5 }),
          }),
        ],
      }),
    );
  });
});

describe("useAllWorkflowSnapshots live task placement", () => {
  it("keeps a newer live task placement when an older snapshot finishes later", async () => {
    resetState();
    let resolveFetch: (value: unknown) => void = () => {};
    mocks.fetchWorkflowSnapshot.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveFetch = resolve;
      }),
    );

    setLightweightSnapshot({
      id: LIVE_PLACEMENT_TASK_ID,
      workflowStepId: STEP_ID,
      title: "Live placement",
      position: 0,
      state: "IN_PROGRESS",
    });

    renderHook(() => useAllWorkflowSnapshots(WORKSPACE_ID));
    await waitFor(() =>
      expect(mocks.fetchWorkflowSnapshot).toHaveBeenCalledWith(WORKFLOW_ID, expect.anything()),
    );

    setLightweightSnapshot({
      id: LIVE_PLACEMENT_TASK_ID,
      workflowStepId: LIVE_STEP_ID,
      title: "Live placement",
      position: 2,
      state: "IN_PROGRESS",
    });
    resolveFetch({
      steps: [
        { id: STEP_ID, name: "Doing", color: null, position: 0 },
        { id: LIVE_STEP_ID, name: "Done", color: null, position: 1 },
      ],
      tasks: [
        {
          id: LIVE_PLACEMENT_TASK_ID,
          workflow_step_id: STEP_ID,
          title: "Live placement",
          position: 0,
          state: "IN_PROGRESS",
        },
      ],
    });

    await waitFor(() => expect(mocks.setWorkflowSnapshot).toHaveBeenCalled());
    expect(mocks.setWorkflowSnapshot).toHaveBeenCalledWith(
      WORKFLOW_ID,
      expect.objectContaining({
        tasks: [
          expect.objectContaining({
            id: LIVE_PLACEMENT_TASK_ID,
            workflowStepId: LIVE_STEP_ID,
            position: 2,
          }),
        ],
      }),
    );
  });
});
