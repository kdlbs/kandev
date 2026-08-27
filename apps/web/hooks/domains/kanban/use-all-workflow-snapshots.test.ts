import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";

const mockClearKanbanMulti = vi.fn();
const mockSetKanbanMultiLoading = vi.fn();
const mockSetWorkflowSnapshot = vi.fn();
const mockFetchWorkflowSnapshot = vi.fn();

type Workflow = { id: string; workspaceId: string; name: string };
type MockState = {
  connection: { status: string };
  workspaces: { activeId: string | null };
  workspaceContextGeneration: number;
  workflows: { items: Workflow[] };
  kanbanMulti: { snapshots: Record<string, unknown>; isLoading: boolean };
  clearKanbanMulti: typeof mockClearKanbanMulti;
  setKanbanMultiLoading: typeof mockSetKanbanMultiLoading;
  setWorkflowSnapshot: typeof mockSetWorkflowSnapshot;
};

let mockState: MockState = {
  connection: { status: "connected" },
  workspaces: { activeId: "ws-A" },
  workspaceContextGeneration: 0,
  workflows: { items: [] },
  kanbanMulti: { snapshots: {}, isLoading: false },
  clearKanbanMulti: mockClearKanbanMulti,
  setKanbanMultiLoading: mockSetKanbanMultiLoading,
  setWorkflowSnapshot: mockSetWorkflowSnapshot,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
  useAppStoreApi: () => ({ getState: () => mockState }),
}));

vi.mock("@/lib/api", () => ({
  fetchWorkflowSnapshot: (...args: unknown[]) => mockFetchWorkflowSnapshot(...args),
}));

import { useAllWorkflowSnapshots } from "./use-all-workflow-snapshots";

function resetMocks(workflows: Workflow[] = []) {
  vi.clearAllMocks();
  mockFetchWorkflowSnapshot.mockResolvedValue({ steps: [], tasks: [] });
  mockState = {
    connection: { status: "connected" },
    workspaces: { activeId: workflows[0]?.workspaceId ?? null },
    workspaceContextGeneration: 0,
    workflows: { items: workflows },
    kanbanMulti: { snapshots: {}, isLoading: false },
    clearKanbanMulti: mockClearKanbanMulti,
    setKanbanMultiLoading: mockSetKanbanMultiLoading,
    setWorkflowSnapshot: mockSetWorkflowSnapshot,
  };
}

describe("useAllWorkflowSnapshots — workspace scoping", () => {
  beforeEach(() => {
    resetMocks([{ id: "wf-A", workspaceId: "ws-A", name: "A" }]);
  });

  it("does not clear snapshots on initial mount (SSR preservation)", async () => {
    renderHook(
      ({ workspaceId }: { workspaceId: string | null }) => useAllWorkflowSnapshots(workspaceId),
      {
        initialProps: { workspaceId: "ws-A" },
      },
    );

    // Allow the effect + Promise.all to settle.
    await waitFor(() => expect(mockSetKanbanMultiLoading).toHaveBeenCalledWith(true));
    expect(mockClearKanbanMulti).not.toHaveBeenCalled();
  });

  it("does not fetch on initial mount when all workflow snapshots are boot-hydrated", () => {
    mockState.kanbanMulti.snapshots = {
      "wf-A": { workflowId: "wf-A", workflowName: "A", steps: [], tasks: [] },
    };

    renderHook(
      ({ workspaceId }: { workspaceId: string | null }) => useAllWorkflowSnapshots(workspaceId),
      {
        initialProps: { workspaceId: "ws-A" },
      },
    );

    expect(mockFetchWorkflowSnapshot).not.toHaveBeenCalled();
    expect(mockSetKanbanMultiLoading).not.toHaveBeenCalledWith(true);
    expect(mockClearKanbanMulti).not.toHaveBeenCalled();
  });

  it("refetches boot-hydrated snapshots when the Kandev window regains focus", async () => {
    mockState.kanbanMulti.snapshots = {
      "wf-A": { workflowId: "wf-A", workflowName: "A", steps: [], tasks: [] },
    };

    renderHook(() => useAllWorkflowSnapshots("ws-A"));
    expect(mockFetchWorkflowSnapshot).not.toHaveBeenCalled();

    act(() => window.dispatchEvent(new Event("focus")));

    await waitFor(() =>
      expect(mockFetchWorkflowSnapshot).toHaveBeenCalledWith("wf-A", {
        cache: "no-store",
      }),
    );
  });

  it("clears snapshots when workspaceId changes", async () => {
    const { rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string | null }) => useAllWorkflowSnapshots(workspaceId),
      { initialProps: { workspaceId: "ws-A" } },
    );
    await waitFor(() => expect(mockSetKanbanMultiLoading).toHaveBeenCalledWith(true));
    expect(mockClearKanbanMulti).not.toHaveBeenCalled();

    // Switch to workspace B — must clear A's snapshots.
    mockState.workflows = { items: [{ id: "wf-B", workspaceId: "ws-B", name: "B" }] };
    rerender({ workspaceId: "ws-B" });

    await waitFor(() => expect(mockClearKanbanMulti).toHaveBeenCalledTimes(1));
  });

  it("skips refetch when workspace + workflow set is unchanged across renders", async () => {
    const workflows = [{ id: "wf-A", workspaceId: "ws-A", name: "A" }];
    const { rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string | null }) => useAllWorkflowSnapshots(workspaceId),
      { initialProps: { workspaceId: "ws-A" } },
    );
    await waitFor(() => expect(mockFetchWorkflowSnapshot).toHaveBeenCalledTimes(1));

    // Same-workflow rerender — dedup key unchanged, must not refetch.
    mockState.workflows = { items: [...workflows] };
    rerender({ workspaceId: "ws-A" });

    // Positive signal: follow up with a DIFFERENT workflow set, which must
    // trigger a fetch. If dedup worked, the total is 2 (initial + this one).
    // If dedup failed, the same-key rerender would have fired a fetch before
    // this one, making the total 3. Waiting for count==2 proves both:
    // the dedup rerender was skipped AND the next real change still fetches.
    mockState.workflows = { items: [{ id: "wf-A2", workspaceId: "ws-A", name: "A2" }] };
    rerender({ workspaceId: "ws-A" });
    await waitFor(() => expect(mockFetchWorkflowSnapshot).toHaveBeenCalledTimes(2));
    expect(mockFetchWorkflowSnapshot.mock.calls[1][0]).toBe("wf-A2");
    expect(mockClearKanbanMulti).not.toHaveBeenCalled();
  });
});

describe("useAllWorkflowSnapshots — fetch guards", () => {
  beforeEach(() => {
    resetMocks([{ id: "wf-A", workspaceId: "ws-A", name: "A" }]);
  });

  it("discards a stale in-flight fetch when workspace switches mid-fetch", async () => {
    // Hold the first fetch open so it resolves after the workspace switch.
    let resolveStale: (v: { steps: []; tasks: [] }) => void = () => {};
    mockFetchWorkflowSnapshot.mockImplementationOnce(
      () =>
        new Promise((res) => {
          resolveStale = res;
        }),
    );

    const { rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string | null }) => useAllWorkflowSnapshots(workspaceId),
      { initialProps: { workspaceId: "ws-A" } },
    );
    await waitFor(() =>
      expect(mockFetchWorkflowSnapshot).toHaveBeenCalledWith("wf-A", expect.anything()),
    );

    // Switch to workspace B before A's fetch resolves. Wait for B's fetch
    // to settle (positive signal) so the new-gen effect is fully in place.
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({ steps: [], tasks: [] });
    mockState = {
      ...mockState,
      workspaces: { activeId: "ws-B" },
      workspaceContextGeneration: 1,
      workflows: { items: [{ id: "wf-B", workspaceId: "ws-B", name: "B" }] },
    };
    rerender({ workspaceId: "ws-B" });
    await waitFor(() =>
      expect(mockSetWorkflowSnapshot).toHaveBeenCalledWith("wf-B", expect.anything()),
    );

    // Resolve A's stale fetch and drain the microtask queue so its .then
    // and .finally callbacks run. Flushing microtasks is deterministic —
    // unlike setTimeout, it doesn't depend on CI wall-clock speed.
    resolveStale({ steps: [], tasks: [] });
    for (let i = 0; i < 5; i++) await Promise.resolve();

    const writtenIds = mockSetWorkflowSnapshot.mock.calls.map((args) => args[0]);
    expect(writtenIds).not.toContain("wf-A");
  });

  it("discards an A snapshot after reset activates B but before rerender", async () => {
    let resolveA: (value: { steps: []; tasks: [] }) => void = () => {};
    mockFetchWorkflowSnapshot.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveA = resolve;
        }),
    );

    renderHook(() => useAllWorkflowSnapshots("ws-A"));
    await waitFor(() =>
      expect(mockFetchWorkflowSnapshot).toHaveBeenCalledWith("wf-A", expect.anything()),
    );

    mockState = {
      ...mockState,
      workspaces: { activeId: "ws-B" },
      workspaceContextGeneration: 1,
      kanbanMulti: { snapshots: {}, isLoading: false },
    };
    resolveA({ steps: [], tasks: [] });
    for (let i = 0; i < 5; i++) await Promise.resolve();

    expect(mockSetWorkflowSnapshot).not.toHaveBeenCalled();
    expect(mockSetKanbanMultiLoading).not.toHaveBeenLastCalledWith(false);
  });
});

const WORKTREE_EXECUTOR_FIELDS = {
  primaryExecutorId: "exec-worktree",
  primaryExecutorType: "worktree",
  primaryExecutorName: "Worktree",
  isRemoteExecutor: false,
};

function seedCachedTask(taskOverrides: Record<string, unknown>) {
  mockState.kanbanMulti.snapshots = {
    "wf-A": {
      workflowId: "wf-A",
      workflowName: "A",
      isPlaceholder: true,
      steps: [],
      tasks: [{ id: "task-1", workflowStepId: "step-1", ...taskOverrides }],
    },
  };
}

describe("useAllWorkflowSnapshots — snapshot mapping", () => {
  beforeEach(() => {
    resetMocks([{ id: "wf-A", workspaceId: "ws-A", name: "A" }]);
  });

  it("preserves workflow step WIP fields in snapshots", async () => {
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({
      steps: [
        {
          id: "step-1",
          name: "Review",
          position: 1,
          color: "bg-blue-500",
          wip_limit: 2,
          pull_from_step_id: "step-0",
        },
      ],
      tasks: [],
    });

    renderHook(
      ({ workspaceId }: { workspaceId: string | null }) => useAllWorkflowSnapshots(workspaceId),
      { initialProps: { workspaceId: "ws-A" } },
    );

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    expect(mockSetWorkflowSnapshot.mock.calls[0][1].steps[0]).toMatchObject({
      wip_limit: 2,
      pull_from_step_id: "step-0",
    });
  });

  it("preserves a cached autopilot marker when a fresh snapshot omits it", async () => {
    seedCachedTask({ autopilot: true });
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({
      steps: [{ id: "step-1", name: "Review", position: 1 }],
      tasks: [{ id: "task-1", workflow_step_id: "step-1", title: "Task" }],
    });

    renderHook(() => useAllWorkflowSnapshots("ws-A"));

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    expect(mockSetWorkflowSnapshot.mock.calls[0][1].tasks[0].autopilot).toBe(true);
  });

  it("keeps an explicit false autopilot value from the fresh snapshot", async () => {
    seedCachedTask({ autopilot: true });
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({
      steps: [{ id: "step-1", name: "Review", position: 1 }],
      tasks: [{ id: "task-1", workflow_step_id: "step-1", title: "Task", autopilot: false }],
    });

    renderHook(() => useAllWorkflowSnapshots("ws-A"));

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    expect(mockSetWorkflowSnapshot.mock.calls[0][1].tasks[0].autopilot).toBe(false);
  });
});

describe("useAllWorkflowSnapshots — executor binding merge", () => {
  beforeEach(() => {
    resetMocks([{ id: "wf-A", workspaceId: "ws-A", name: "A" }]);
  });

  it("preserves a newer live executor binding when a delayed snapshot omits it", async () => {
    seedCachedTask({});
    let resolveSnapshot: (value: { steps: unknown[]; tasks: unknown[] }) => void = () => {};
    mockFetchWorkflowSnapshot.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveSnapshot = resolve;
        }),
    );

    renderHook(() => useAllWorkflowSnapshots("ws-A"));
    await waitFor(() => expect(mockFetchWorkflowSnapshot).toHaveBeenCalled());

    // A task.updated event adds the executor while the snapshot is in flight.
    mockState.kanbanMulti.snapshots = {
      "wf-A": {
        workflowId: "wf-A",
        workflowName: "A",
        isPlaceholder: true,
        steps: [],
        tasks: [{ id: "task-1", workflowStepId: "step-1", ...WORKTREE_EXECUTOR_FIELDS }],
      },
    };
    resolveSnapshot({
      steps: [{ id: "step-1", name: "Review", position: 1 }],
      tasks: [{ id: "task-1", workflow_step_id: "step-1", title: "Task" }],
    });

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    expect(mockSetWorkflowSnapshot.mock.calls[0][1].tasks[0]).toMatchObject(
      WORKTREE_EXECUTOR_FIELDS,
    );
  });

  it("clears a cached executor when a current snapshot omits the primary session", async () => {
    seedCachedTask({ primarySessionId: "session-1", ...WORKTREE_EXECUTOR_FIELDS });
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({
      steps: [{ id: "step-1", name: "Review", position: 1 }],
      // The backend omits primary_session_id and primary_executor_* when the
      // task has no primary session because those DTO fields use omitempty.
      tasks: [{ id: "task-1", workflow_step_id: "step-1", title: "Task" }],
    });

    renderHook(() => useAllWorkflowSnapshots("ws-A"));

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    expect(mockSetWorkflowSnapshot.mock.calls[0][1].tasks[0].primaryExecutorType).toBeUndefined();
  });

  it("preserves each omitted executor field from a newer live binding independently", async () => {
    seedCachedTask({});
    let resolveSnapshot: (value: { steps: unknown[]; tasks: unknown[] }) => void = () => {};
    mockFetchWorkflowSnapshot.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveSnapshot = resolve;
        }),
    );

    renderHook(() => useAllWorkflowSnapshots("ws-A"));
    await waitFor(() => expect(mockFetchWorkflowSnapshot).toHaveBeenCalled());

    mockState.kanbanMulti.snapshots = {
      "wf-A": {
        workflowId: "wf-A",
        workflowName: "A",
        isPlaceholder: true,
        steps: [],
        tasks: [{ id: "task-1", workflowStepId: "step-1", ...WORKTREE_EXECUTOR_FIELDS }],
      },
    };
    resolveSnapshot({
      steps: [{ id: "step-1", name: "Review", position: 1 }],
      tasks: [
        {
          id: "task-1",
          workflow_step_id: "step-1",
          title: "Task",
          primary_executor_type: "worktree",
        },
      ],
    });

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    expect(mockSetWorkflowSnapshot.mock.calls[0][1].tasks[0]).toMatchObject(
      WORKTREE_EXECUTOR_FIELDS,
    );
  });
});

describe("useAllWorkflowSnapshots — executor binding mapping", () => {
  beforeEach(() => {
    resetMocks([{ id: "wf-A", workspaceId: "ws-A", name: "A" }]);
  });

  it("clears a cached executor binding when the response explicitly clears the primary session", async () => {
    seedCachedTask({ primarySessionId: "session-1", ...WORKTREE_EXECUTOR_FIELDS });
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({
      steps: [{ id: "step-1", name: "Review", position: 1 }],
      tasks: [
        {
          id: "task-1",
          workflow_step_id: "step-1",
          title: "Task",
          primary_session_id: null,
        },
      ],
    });

    renderHook(() => useAllWorkflowSnapshots("ws-A"));

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    expect(mockSetWorkflowSnapshot.mock.calls[0][1].tasks[0].primaryExecutorType).toBeUndefined();
  });

  it("adopts a fresh primary executor binding when the response includes one", async () => {
    seedCachedTask({});
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({
      steps: [{ id: "step-1", name: "Review", position: 1 }],
      tasks: [
        {
          id: "task-1",
          workflow_step_id: "step-1",
          title: "Task",
          primary_executor_id: "exec-worktree",
          primary_executor_type: "worktree",
          primary_executor_name: "Worktree",
          is_remote_executor: false,
        },
      ],
    });

    renderHook(() => useAllWorkflowSnapshots("ws-A"));

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    expect(mockSetWorkflowSnapshot.mock.calls[0][1].tasks[0]).toMatchObject(
      WORKTREE_EXECUTOR_FIELDS,
    );
  });
});

const REMOTE_EXECUTOR_NAME = "Remote box";

function seedExecutor(executor: Record<string, unknown>) {
  mockState.kanbanMulti.snapshots = {
    "wf-A": {
      workflowId: "wf-A",
      workflowName: "A",
      isPlaceholder: true,
      steps: [],
      tasks: [{ id: "task-1", workflowStepId: "step-1", ...executor }],
    },
  };
}

function seedCachedExecutor() {
  seedExecutor({
    primaryExecutorId: "exec-1",
    primaryExecutorType: "worktree",
    primaryExecutorName: "Worktree",
    isRemoteExecutor: false,
  });
}

function seedNewerLiveExecutor() {
  seedExecutor({
    primaryExecutorId: "exec-2",
    primaryExecutorType: "ssh",
    primaryExecutorName: REMOTE_EXECUTOR_NAME,
    isRemoteExecutor: true,
  });
}

describe("useAllWorkflowSnapshots — executor field preservation", () => {
  beforeEach(() => {
    resetMocks([{ id: "wf-A", workspaceId: "ws-A", name: "A" }]);
  });

  it("honors an executor clear from a fresh snapshot", async () => {
    seedCachedExecutor();
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({
      steps: [{ id: "step-1", name: "Review", position: 1 }],
      tasks: [{ id: "task-1", workflow_step_id: "step-1", title: "Task" }],
    });

    renderHook(() => useAllWorkflowSnapshots("ws-A"));

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    const task = mockSetWorkflowSnapshot.mock.calls[0][1].tasks[0];
    expect(task.primaryExecutorId).toBeUndefined();
    expect(task.primaryExecutorType).toBeUndefined();
    expect(task.primaryExecutorName).toBeUndefined();
  });

  it("preserves a newer live executor when a stale snapshot omits it", async () => {
    seedCachedExecutor();
    let resolveSnapshot: ((value: { steps: unknown[]; tasks: unknown[] }) => void) | undefined;
    mockFetchWorkflowSnapshot.mockImplementationOnce(
      () =>
        new Promise<{ steps: unknown[]; tasks: unknown[] }>((resolve) => {
          resolveSnapshot = resolve;
        }),
    );

    renderHook(() => useAllWorkflowSnapshots("ws-A"));
    await waitFor(() => expect(mockFetchWorkflowSnapshot).toHaveBeenCalled());

    seedNewerLiveExecutor();
    resolveSnapshot?.({
      steps: [{ id: "step-1", name: "Review", position: 1 }],
      tasks: [{ id: "task-1", workflow_step_id: "step-1", title: "Stale task" }],
    });

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    expect(mockSetWorkflowSnapshot.mock.calls[0][1].tasks[0]).toMatchObject({
      primaryExecutorId: "exec-2",
      primaryExecutorType: "ssh",
      primaryExecutorName: REMOTE_EXECUTOR_NAME,
      isRemoteExecutor: true,
    });
  });

  it("adopts a legitimately different executor value from the fresh snapshot", async () => {
    seedCachedExecutor();
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({
      steps: [{ id: "step-1", name: "Review", position: 1 }],
      tasks: [
        {
          id: "task-1",
          workflow_step_id: "step-1",
          title: "Task",
          primary_executor_id: "exec-2",
          primary_executor_type: "ssh",
          primary_executor_name: "Remote box",
          is_remote_executor: true,
        },
      ],
    });

    renderHook(() => useAllWorkflowSnapshots("ws-A"));

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    expect(mockSetWorkflowSnapshot.mock.calls[0][1].tasks[0]).toMatchObject({
      primaryExecutorId: "exec-2",
      primaryExecutorType: "ssh",
      primaryExecutorName: "Remote box",
      isRemoteExecutor: true,
    });
  });
});

describe("useAllWorkflowSnapshots — executor race ordering", () => {
  beforeEach(() => {
    resetMocks([{ id: "wf-A", workspaceId: "ws-A", name: "A" }]);
  });

  it("keeps a live detach when a stale snapshot carries the old executor", async () => {
    seedExecutor({
      primaryExecutorId: "exec-1",
      primaryExecutorType: "worktree",
      primaryExecutorName: "Worktree",
      isRemoteExecutor: true,
    });
    let resolveSnapshot: ((value: { steps: unknown[]; tasks: unknown[] }) => void) | undefined;
    mockFetchWorkflowSnapshot.mockImplementationOnce(
      () =>
        new Promise<{ steps: unknown[]; tasks: unknown[] }>((resolve) => {
          resolveSnapshot = resolve;
        }),
    );

    renderHook(() => useAllWorkflowSnapshots("ws-A"));
    await waitFor(() => expect(mockFetchWorkflowSnapshot).toHaveBeenCalled());

    seedExecutor({ isRemoteExecutor: false });
    resolveSnapshot?.({
      steps: [{ id: "step-1", name: "Review", position: 1 }],
      tasks: [
        {
          id: "task-1",
          workflow_step_id: "step-1",
          title: "Stale task",
          primary_executor_id: "exec-1",
          primary_executor_type: "worktree",
          primary_executor_name: "Worktree",
          is_remote_executor: true,
        },
      ],
    });

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    const task = mockSetWorkflowSnapshot.mock.calls[0][1].tasks[0];
    expect(task.primaryExecutorId).toBeUndefined();
    expect(task.primaryExecutorType).toBeUndefined();
    expect(task.primaryExecutorName).toBeUndefined();
    expect(task.isRemoteExecutor).toBe(false);
  });

  it("keeps the live executor for a task added during the snapshot fetch", async () => {
    let resolveSnapshot: ((value: { steps: unknown[]; tasks: unknown[] }) => void) | undefined;
    mockFetchWorkflowSnapshot.mockImplementationOnce(
      () =>
        new Promise<{ steps: unknown[]; tasks: unknown[] }>((resolve) => {
          resolveSnapshot = resolve;
        }),
    );

    renderHook(() => useAllWorkflowSnapshots("ws-A"));
    await waitFor(() => expect(mockFetchWorkflowSnapshot).toHaveBeenCalled());

    seedNewerLiveExecutor();
    resolveSnapshot?.({
      steps: [{ id: "step-1", name: "Review", position: 1 }],
      tasks: [{ id: "task-1", workflow_step_id: "step-1", title: "Stale task" }],
    });

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    expect(mockSetWorkflowSnapshot.mock.calls[0][1].tasks[0]).toMatchObject({
      primaryExecutorId: "exec-2",
      primaryExecutorType: "ssh",
      primaryExecutorName: REMOTE_EXECUTOR_NAME,
      isRemoteExecutor: true,
    });
  });
});

/**
 * A snapshot request issued before a `task.status_summary.updated` delta can
 * land after it. Writing the response's summary unconditionally regresses the
 * cached row to the older revision, and a settled task emits no further
 * deltas — so the row stays wrong until the next full hydrate. This surfaced
 * as a finished task stuck behind a "preparing" spinner in the sidebar.
 */
type SummaryFixture = Record<string, unknown> | undefined;

function seedSummaryRace(
  cached: SummaryFixture,
  response: SummaryFixture,
  statusSummaryInvalidated = false,
) {
  mockState.kanbanMulti.snapshots = {
    "wf-A": {
      workflowId: "wf-A",
      workflowName: "A",
      steps: [],
      tasks: [{ id: "task-1", workflowStepId: "step-1", statusSummary: cached }],
    },
  };
  // Not `...Once`: the hook can fetch more than once (mount + foreground
  // refresh), and falling back to the empty default would make the assertion
  // read a write that carries no tasks at all.
  mockFetchWorkflowSnapshot.mockResolvedValue({
    steps: [{ id: "step-1", name: "In Progress", position: 0 }],
    tasks: [
      {
        id: "task-1",
        workflow_step_id: "step-1",
        title: "Task",
        ...(response ? { status_summary: response } : {}),
        ...(statusSummaryInvalidated ? { status_summary_invalidated: true } : {}),
      },
    ],
  });
  renderHook(() => useAllWorkflowSnapshots("ws-A"));
  // Boot-hydrated snapshots skip the mount fetch; focus forces the refetch.
  act(() => window.dispatchEvent(new Event("focus")));
}

async function writtenSummary() {
  await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
  return mockSetWorkflowSnapshot.mock.calls.at(-1)![1].tasks[0].statusSummary;
}

describe("useAllWorkflowSnapshots — status summary revision race", () => {
  beforeEach(() => {
    resetMocks([{ id: "wf-A", workspaceId: "ws-A", name: "A" }]);
  });

  it("keeps the newer cached summary when a slow response carries an older revision", async () => {
    seedSummaryRace(
      { revision: 4, primary_session: { state: "WAITING_FOR_INPUT" } },
      { revision: 1, primary_session: { state: "STARTING" } },
    );

    const written = await writtenSummary();
    expect(written.revision).toBe(4);
    expect(written.primary_session.state).toBe("WAITING_FOR_INPUT");
  });

  it("adopts the response summary when it is newer than the cached one", async () => {
    seedSummaryRace(
      { revision: 1, primary_session: { state: "STARTING" } },
      { revision: 5, primary_session: { state: "WAITING_FOR_INPUT" } },
    );

    const written = await writtenSummary();
    expect(written.revision).toBe(5);
    expect(written.primary_session.state).toBe("WAITING_FOR_INPUT");
  });

  it("takes an equal-revision response so a re-stamped queued count is not pinned", async () => {
    // The snapshot endpoint re-stamps queued_prompt_count from a fresh queue
    // read without incrementing the revision, so preferring the cached copy at
    // an equal revision would pin a stale queued badge.
    seedSummaryRace(
      { revision: 3, queued_prompt_count: 0 },
      { revision: 3, queued_prompt_count: 5 },
    );

    expect((await writtenSummary()).queued_prompt_count).toBe(5);
  });

  it("keeps the cached summary when the response omits it entirely", async () => {
    seedSummaryRace({ revision: 4, primary_session: { state: "WAITING_FOR_INPUT" } }, undefined);

    expect((await writtenSummary()).revision).toBe(4);
  });

  it("clears an unchanged cached summary when the response explicitly invalidates it", async () => {
    seedSummaryRace({ revision: 4, pending_action: "clarification" }, undefined, true);

    expect(await writtenSummary()).toBeUndefined();
  });
});
