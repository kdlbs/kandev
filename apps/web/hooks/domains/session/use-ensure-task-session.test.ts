import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook } from "@testing-library/react";

const mockEnsureTaskSession = vi.fn();
const mockLoadSessions = vi.fn().mockResolvedValue(undefined);
let mockSessionsResult: {
  sessions: Array<{ id: string }>;
  isLoaded: boolean;
  loadSessions: (force?: boolean) => Promise<void>;
} = {
  sessions: [],
  isLoaded: true,
  loadSessions: mockLoadSessions,
};

let mockStoreState: {
  userSettings: { preventAutoStartAgentOnOpen: boolean };
  kanban: {
    workflowId: string | null;
    steps: Array<{ id: string; position: number }>;
    isLoading: boolean;
  };
  kanbanMulti: {
    snapshots: Record<
      string,
      { steps: Array<{ id: string; position: number }>; isPlaceholder?: boolean }
    >;
  };
} = {
  userSettings: { preventAutoStartAgentOnOpen: false },
  kanban: { workflowId: "wf-active", steps: [], isLoading: false },
  kanbanMulti: { snapshots: {} },
};

vi.mock("@/lib/services/session-launch-service", () => ({
  ensureTaskSession: (taskId: string, opts?: { autoStart?: boolean }) =>
    mockEnsureTaskSession(taskId, opts),
}));

vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: () => mockSessionsResult,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockStoreState) => unknown) => selector(mockStoreState),
}));

import { useEnsureTaskSession, isFinalWorkflowStep } from "./use-ensure-task-session";

const TASK = { id: "task-1" };
const OTHER_DONE_STEP = "other-done";
const OTHER_STEP = "other-step";
const OTHER_WORKFLOW = "wf-other";

function flushMicrotasks() {
  return act(() => Promise.resolve());
}

function resetEnsureTaskSessionMocks() {
  vi.clearAllMocks();
  mockLoadSessions.mockResolvedValue(undefined);
  mockSessionsResult = { sessions: [], isLoaded: true, loadSessions: mockLoadSessions };
  mockStoreState = {
    userSettings: { preventAutoStartAgentOnOpen: false },
    kanban: { workflowId: "wf-active", steps: [], isLoading: false },
    kanbanMulti: { snapshots: {} },
  };
  mockEnsureTaskSession.mockResolvedValue({
    success: true,
    task_id: "task-1",
    session_id: "sess-new",
    state: "CREATED",
    source: "created_prepare",
    newly_created: true,
  });
}

describe("useEnsureTaskSession", () => {
  beforeEach(resetEnsureTaskSessionMocks);

  it("calls the backend ensure endpoint once when the task has no sessions", async () => {
    const { result } = renderHook(() => useEnsureTaskSession(TASK));

    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(1);
    expect(mockEnsureTaskSession).toHaveBeenCalledWith("task-1", undefined);
    expect(result.current.status).toBe("preparing");
    await flushMicrotasks();
    expect(result.current.status).toBe("idle");
  });

  it("force-reloads the session list after a successful ensure", async () => {
    renderHook(() => useEnsureTaskSession(TASK));
    await flushMicrotasks();
    // Two awaits: ensure().then(loadSessions(true)).then(setStatus).
    await flushMicrotasks();
    expect(mockLoadSessions).toHaveBeenCalledWith(true);
  });

  it("no-ops when the task already has a session", () => {
    mockSessionsResult = {
      sessions: [{ id: "sess-1" }],
      isLoaded: true,
      loadSessions: mockLoadSessions,
    };
    renderHook(() => useEnsureTaskSession(TASK));
    expect(mockEnsureTaskSession).not.toHaveBeenCalled();
  });

  it("no-ops while sessions are still loading", () => {
    mockSessionsResult = { sessions: [], isLoaded: false, loadSessions: mockLoadSessions };
    renderHook(() => useEnsureTaskSession(TASK));
    expect(mockEnsureTaskSession).not.toHaveBeenCalled();
  });

  it("no-ops when disabled", () => {
    renderHook(() => useEnsureTaskSession(TASK, { enabled: false }));
    expect(mockEnsureTaskSession).not.toHaveBeenCalled();
  });

  it("no-ops when task id is missing", () => {
    renderHook(() => useEnsureTaskSession(null));
    expect(mockEnsureTaskSession).not.toHaveBeenCalled();
  });

  it("is idempotent across re-renders for the same task", () => {
    const { rerender } = renderHook(() => useEnsureTaskSession(TASK));
    rerender();
    rerender();
    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(1);
  });

  it("reports an error and exposes a working retry()", async () => {
    mockEnsureTaskSession.mockRejectedValueOnce(new Error("boom"));
    const { result } = renderHook(() => useEnsureTaskSession(TASK));

    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(1);
    await flushMicrotasks();
    expect(result.current.status).toBe("error");
    expect(result.current.error?.message).toBe("boom");

    mockEnsureTaskSession.mockResolvedValueOnce({
      success: true,
      task_id: "task-1",
      session_id: "sess-new",
      state: "CREATED",
      source: "created_prepare",
      newly_created: true,
    });
    act(() => result.current.retry());
    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(2);
    await flushMicrotasks();
    expect(result.current.status).toBe("idle");
  });

  // @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.7
  // @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.8
  it("keeps a failed ensure latched until the user retries", async () => {
    mockEnsureTaskSession.mockRejectedValue(new Error("workspace is not attachable"));
    const { result, rerender } = renderHook(() => useEnsureTaskSession(TASK));

    await flushMicrotasks();
    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(1);

    mockSessionsResult = {
      ...mockSessionsResult,
      loadSessions: vi.fn().mockResolvedValue(undefined),
    };
    rerender();
    await flushMicrotasks();
    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(1);

    act(() => result.current.retry());
    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(2);
  });
});

describe("useEnsureTaskSession failed gate changes", () => {
  beforeEach(resetEnsureTaskSessionMocks);

  it("keeps a failed ensure latched when the final-step gate changes", async () => {
    mockStoreState = {
      userSettings: { preventAutoStartAgentOnOpen: true },
      kanban: {
        workflowId: "wf-active",
        steps: [
          { id: "step-1", position: 0 },
          { id: "step-done", position: 1 },
        ],
        isLoading: false,
      },
      kanbanMulti: { snapshots: {} },
    };
    mockEnsureTaskSession.mockRejectedValue(new Error("workspace is not attachable"));
    const { rerender } = renderHook(() =>
      useEnsureTaskSession({ id: "task-1", workflowStepId: "step-1", workflowId: "wf-active" }),
    );

    await flushMicrotasks();
    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(1);
    expect(mockEnsureTaskSession).toHaveBeenCalledWith("task-1", undefined);

    mockStoreState.kanban.steps = [{ id: "step-1", position: 0 }];
    rerender();
    await flushMicrotasks();

    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(1);
  });
});

describe("useEnsureTaskSession — task changes", () => {
  beforeEach(resetEnsureTaskSessionMocks);

  it("clears a stale error when switching to a task that already has a session", async () => {
    mockEnsureTaskSession.mockRejectedValueOnce(new Error("task one failed"));
    const { result, rerender } = renderHook(
      ({ task }: { task: { id: string } }) => useEnsureTaskSession(task),
      { initialProps: { task: TASK } },
    );

    await flushMicrotasks();
    expect(result.current.status).toBe("error");
    expect(result.current.error?.message).toBe("task one failed");

    mockSessionsResult = {
      sessions: [{ id: "sess-2" }],
      isLoaded: true,
      loadSessions: mockLoadSessions,
    };
    rerender({ task: { id: "task-2" } });

    expect(result.current.status).toBe("idle");
    expect(result.current.error).toBeNull();
    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(1);
  });

  it("calls ensure again when the task id changes", () => {
    const { rerender } = renderHook(
      ({ task }: { task: { id: string } }) => useEnsureTaskSession(task),
      { initialProps: { task: TASK } },
    );
    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(1);
    rerender({ task: { id: "task-2" } });
    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(2);
    expect(mockEnsureTaskSession).toHaveBeenLastCalledWith("task-2", undefined);
  });
});

describe("isFinalWorkflowStep", () => {
  const steps = [
    { id: "a", position: 0 },
    { id: "b", position: 1 },
    { id: "c", position: 2 },
  ];

  it("returns true for the max-position step", () => {
    expect(isFinalWorkflowStep("c", steps)).toBe(true);
  });

  it("returns false for non-terminal steps", () => {
    expect(isFinalWorkflowStep("a", steps)).toBe(false);
    expect(isFinalWorkflowStep("b", steps)).toBe(false);
  });

  it("breaks position ties deterministically by max id", () => {
    const tied = [
      { id: "early", position: 2 },
      { id: "late", position: 2 },
    ];
    expect(isFinalWorkflowStep("late", tied)).toBe(true);
    expect(isFinalWorkflowStep("early", tied)).toBe(false);
  });

  it("treats missing step id or empty steps as not final", () => {
    expect(isFinalWorkflowStep(undefined, steps)).toBe(false);
    expect(isFinalWorkflowStep("c", [])).toBe(false);
    expect(isFinalWorkflowStep("missing", steps)).toBe(false);
  });
});

// eslint-disable-next-line max-lines-per-function -- test describe block, splitting hurts readability
describe("useEnsureTaskSession prevent-auto-start gate", () => {
  it("requests autoStart:false for a final-step task when the preference is on", async () => {
    mockStoreState = {
      userSettings: { preventAutoStartAgentOnOpen: true },
      kanban: {
        workflowId: "wf-active",
        steps: [
          { id: "step-1", position: 0 },
          { id: "step-done", position: 1 },
        ],
        isLoading: false,
      },
      kanbanMulti: { snapshots: {} },
    };
    const { result } = renderHook(() =>
      useEnsureTaskSession({ id: "task-1", workflowStepId: "step-done", workflowId: "wf-active" }),
    );
    await flushMicrotasks();
    expect(mockEnsureTaskSession).toHaveBeenCalledWith("task-1", { autoStart: false });
    expect(result.current.status).toBe("idle");
  });

  it("keeps the default ensure call for a non-final step even when the preference is on", async () => {
    mockStoreState = {
      userSettings: { preventAutoStartAgentOnOpen: true },
      kanban: {
        workflowId: "wf-active",
        steps: [
          { id: "step-1", position: 0 },
          { id: "step-done", position: 1 },
        ],
        isLoading: false,
      },
      kanbanMulti: { snapshots: {} },
    };
    renderHook(() =>
      useEnsureTaskSession({ id: "task-1", workflowStepId: "step-1", workflowId: "wf-active" }),
    );
    await flushMicrotasks();
    expect(mockEnsureTaskSession).toHaveBeenCalledWith("task-1", undefined);
  });

  it("does NOT gate a task whose workflow id is missing even when its step id matches the active workflow's terminal step", async () => {
    mockStoreState = {
      userSettings: { preventAutoStartAgentOnOpen: true },
      kanban: {
        workflowId: "wf-active",
        steps: [
          { id: "step-1", position: 0 },
          { id: "step-done", position: 1 },
        ],
        isLoading: false,
      },
      kanbanMulti: { snapshots: {} },
    };
    // No workflowId on the task: resolving against the active workflow's
    // steps would gate a task whose own workflow is unknown, suppressing the
    // auto-start the user expects. Missing workflow id → no gate.
    renderHook(() => useEnsureTaskSession({ id: "task-1", workflowStepId: "step-done" }));
    await flushMicrotasks();
    expect(mockEnsureTaskSession).toHaveBeenCalledWith("task-1", undefined);
  });

  it("resolves the step list from the multi-workflow snapshot for a cross-workflow task", async () => {
    mockStoreState = {
      userSettings: { preventAutoStartAgentOnOpen: true },
      kanban: {
        workflowId: "wf-active",
        steps: [{ id: "active-step", position: 0 }],
        isLoading: false,
      },
      kanbanMulti: {
        snapshots: {
          [OTHER_WORKFLOW]: {
            steps: [
              { id: OTHER_STEP, position: 0 },
              { id: OTHER_DONE_STEP, position: 1 },
            ],
          },
        },
      },
    };
    renderHook(() =>
      useEnsureTaskSession({
        id: "task-2",
        workflowStepId: OTHER_DONE_STEP,
        workflowId: OTHER_WORKFLOW,
      }),
    );
    await flushMicrotasks();
    expect(mockEnsureTaskSession).toHaveBeenCalledWith("task-2", { autoStart: false });
  });

  it("does not gate when the preference is off", async () => {
    mockStoreState = {
      userSettings: { preventAutoStartAgentOnOpen: false },
      kanban: {
        workflowId: "wf-active",
        steps: [
          { id: "step-1", position: 0 },
          { id: "step-done", position: 1 },
        ],
        isLoading: false,
      },
      kanbanMulti: { snapshots: {} },
    };
    renderHook(() =>
      useEnsureTaskSession({ id: "task-1", workflowStepId: "step-done", workflowId: "wf-active" }),
    );
    await flushMicrotasks();
    expect(mockEnsureTaskSession).toHaveBeenCalledWith("task-1", undefined);
  });
});

describe("useEnsureTaskSession late step hydration", () => {
  beforeEach(resetEnsureTaskSessionMocks);

  it("waits for the workflow steps before ensuring, then applies the gate", async () => {
    mockStoreState = {
      userSettings: { preventAutoStartAgentOnOpen: true },
      kanban: {
        workflowId: "wf-active",
        // Steps not yet hydrated — isLoading still true.
        steps: [],
        isLoading: true,
      },
      kanbanMulti: { snapshots: {} },
    };
    const { rerender } = renderHook(() =>
      useEnsureTaskSession({ id: "task-1", workflowStepId: "step-done", workflowId: "wf-active" }),
    );
    await flushMicrotasks();
    // No ensure while the step list is unresolved.
    expect(mockEnsureTaskSession).not.toHaveBeenCalled();

    // Steps hydrate: the final step appears and loading completes.
    mockStoreState = {
      userSettings: { preventAutoStartAgentOnOpen: true },
      kanban: {
        workflowId: "wf-active",
        steps: [
          { id: "step-1", position: 0 },
          { id: "step-done", position: 1 },
        ],
        isLoading: false,
      },
      kanbanMulti: { snapshots: {} },
    };
    rerender();
    await flushMicrotasks();

    // Exactly one ensure, gated: never an ungated call.
    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(1);
    expect(mockEnsureTaskSession).toHaveBeenCalledWith("task-1", { autoStart: false });
  });
});

describe("useEnsureTaskSession placeholder snapshot", () => {
  beforeEach(resetEnsureTaskSessionMocks);

  it("waits for the real snapshot when only a placeholder exists", async () => {
    mockStoreState = {
      userSettings: { preventAutoStartAgentOnOpen: true },
      kanban: { workflowId: "wf-active", steps: [], isLoading: false },
      kanbanMulti: {
        snapshots: {
          [OTHER_WORKFLOW]: { steps: [], isPlaceholder: true },
        },
      },
    };
    const { rerender } = renderHook(() =>
      useEnsureTaskSession({
        id: "task-2",
        workflowStepId: OTHER_DONE_STEP,
        workflowId: OTHER_WORKFLOW,
      }),
    );
    await flushMicrotasks();
    // A placeholder snapshot must not satisfy stepsKnown — no ensure yet.
    expect(mockEnsureTaskSession).not.toHaveBeenCalled();

    // The real workflow snapshot arrives with the terminal step.
    mockStoreState = {
      userSettings: { preventAutoStartAgentOnOpen: true },
      kanban: { workflowId: "wf-active", steps: [], isLoading: false },
      kanbanMulti: {
        snapshots: {
          [OTHER_WORKFLOW]: {
            steps: [
              { id: OTHER_STEP, position: 0 },
              { id: OTHER_DONE_STEP, position: 1 },
            ],
          },
        },
      },
    };
    rerender();
    await flushMicrotasks();

    expect(mockEnsureTaskSession).toHaveBeenCalledTimes(1);
    expect(mockEnsureTaskSession).toHaveBeenCalledWith("task-2", { autoStart: false });
  });
});
