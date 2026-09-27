import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi, beforeEach } from "vitest";
import type { AttentionTask } from "@/lib/coordinator/attention";
import type { Stall } from "@/lib/api/domains/coordinator-api";

const mockUseCoordinatorTasks = vi.fn();
const mockUseCoordinatorInputs = vi.fn();
const mockUseNowTick = vi.fn();

vi.mock("./use-coordinator-tasks", () => ({
  useCoordinatorTasks: (...args: unknown[]) => mockUseCoordinatorTasks(...args),
}));

vi.mock("./use-coordinator-inputs", () => ({
  useCoordinatorInputs: (...args: unknown[]) => mockUseCoordinatorInputs(...args),
}));

vi.mock("./use-now-tick", () => ({
  useNowTick: () => mockUseNowTick(),
}));

import { useCoordinatorAttention } from "./use-coordinator-attention";

const WORKSPACE_ID = "workspace-1";
const COORDINATOR_ID = "coordinator-1";

const retryTasksMock = vi.fn();
const retryFailedMock = vi.fn();

function task(id: string): AttentionTask {
  return { id, title: `Task ${id}` };
}

function stall(taskId: string): Stall {
  return {
    task_id: taskId,
    stalled_for_ms: 60_000,
    last_event_at: "2026-09-27T00:00:00Z",
    detected_at: "2026-09-27T00:01:00Z",
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  mockUseNowTick.mockReturnValue(1_000);
  mockUseCoordinatorTasks.mockReturnValue({
    tasks: [],
    stepNameByTaskId: new Map(),
    workflowNameById: new Map(),
    stepNameByWorkflowStep: new Map(),
    error: false,
    loadedAt: 500,
    retry: retryTasksMock,
  });
  mockUseCoordinatorInputs.mockReturnValue({
    stalls: { value: undefined, loadedAt: undefined, error: false },
    proposals: { value: undefined, loadedAt: undefined, error: false },
    retryFailed: retryFailedMock,
  });
});

describe("useCoordinatorAttention - classification", () => {
  it("classifies tasks, stalls and proposals into needsYou/queue", () => {
    mockUseCoordinatorTasks.mockReturnValue({
      tasks: [task("t-1")],
      stepNameByTaskId: new Map([["t-1", "Build"]]),
      workflowNameById: new Map([["wf-1", "Planner"]]),
      stepNameByWorkflowStep: new Map([["wf-1:step-1", "Build"]]),
      error: false,
      loadedAt: 500,
      retry: retryTasksMock,
    });
    mockUseCoordinatorInputs.mockReturnValue({
      stalls: { value: [stall("t-1")], loadedAt: 500, error: false },
      proposals: { value: [], loadedAt: 500, error: false },
      retryFailed: retryFailedMock,
    });

    const { result } = renderHook(() => useCoordinatorAttention(WORKSPACE_ID, COORDINATOR_ID));

    expect(result.current.classification.needsYou).toHaveLength(1);
    expect(result.current.classification.needsYou[0]?.kind).toBe("stall");
    expect(result.current.stepNameByTaskId.get("t-1")).toBe("Build");
    expect(result.current.workflowNameById.get("wf-1")).toBe("Planner");
    expect(result.current.stepNameByWorkflowStep.get("wf-1:step-1")).toBe("Build");
    expect(result.current.openTasksById.get("t-1")).toEqual(task("t-1"));
  });

  it("excludes archived tasks from openTasksById", () => {
    mockUseCoordinatorTasks.mockReturnValue({
      tasks: [{ ...task("t-1"), isArchived: true }, task("t-2")],
      stepNameByTaskId: new Map(),
      workflowNameById: new Map(),
      stepNameByWorkflowStep: new Map(),
      error: false,
      loadedAt: 500,
      retry: retryTasksMock,
    });

    const { result } = renderHook(() => useCoordinatorAttention(WORKSPACE_ID, COORDINATOR_ID));

    expect(result.current.openTasksById.has("t-1")).toBe(false);
    expect(result.current.openTasksById.has("t-2")).toBe(true);
  });

  it("treats an unloaded stalls or proposals input as empty for classification", () => {
    const { result } = renderHook(() => useCoordinatorAttention(WORKSPACE_ID, COORDINATOR_ID));

    expect(result.current.classification.needsYou).toEqual([]);
  });
});

describe("useCoordinatorAttention - input status", () => {
  it("reports tasksNeverLoaded when the tasks input has no load time", () => {
    mockUseCoordinatorTasks.mockReturnValue({
      tasks: [],
      stepNameByTaskId: new Map(),
      workflowNameById: new Map(),
      stepNameByWorkflowStep: new Map(),
      error: true,
      loadedAt: undefined,
      retry: retryTasksMock,
    });

    const { result } = renderHook(() => useCoordinatorAttention(WORKSPACE_ID, COORDINATOR_ID));

    expect(result.current.tasksNeverLoaded).toBe(true);
    expect(result.current.inputs).toEqual([
      { kind: "tasks", error: true, loadedAt: undefined },
      { kind: "stalls", error: false, loadedAt: undefined },
      { kind: "proposals", error: false, loadedAt: undefined },
    ]);
  });

  it("orders inputs as tasks, stalls, proposals", () => {
    const { result } = renderHook(() => useCoordinatorAttention(WORKSPACE_ID, COORDINATOR_ID));

    expect(result.current.inputs.map((i) => i.kind)).toEqual(["tasks", "stalls", "proposals"]);
  });
});

describe("useCoordinatorAttention - retryFailed", () => {
  it("retries tasks only when the tasks input has failed, and always calls the inputs retry", () => {
    const { result, rerender } = renderHook(() =>
      useCoordinatorAttention(WORKSPACE_ID, COORDINATOR_ID),
    );

    result.current.retryFailed();
    expect(retryTasksMock).not.toHaveBeenCalled();
    expect(retryFailedMock).toHaveBeenCalledTimes(1);

    mockUseCoordinatorTasks.mockReturnValue({
      tasks: [],
      stepNameByTaskId: new Map(),
      workflowNameById: new Map(),
      stepNameByWorkflowStep: new Map(),
      error: true,
      loadedAt: 500,
      retry: retryTasksMock,
    });
    rerender();

    result.current.retryFailed();
    expect(retryTasksMock).toHaveBeenCalledTimes(1);
    expect(retryFailedMock).toHaveBeenCalledTimes(2);
  });
});
