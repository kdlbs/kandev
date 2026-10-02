import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AttentionTask } from "@/lib/coordinator/attention";
import type { Stall } from "@/lib/api/domains/coordinator-api";

const tasksMock = vi.fn();
const inputsMock = vi.fn();
const watchMock = vi.fn();
const retryWatch = vi.fn();
const retryStallsAndProposals = vi.fn();
const retryTasks = vi.fn();

vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => true }));
vi.mock("./use-coordinator-tasks", () => ({ useCoordinatorTasks: () => tasksMock() }));
vi.mock("./use-coordinator-prs", () => ({ useCoordinatorPRs: () => new Map() }));
vi.mock("./use-coordinator-inputs", () => ({ useCoordinatorInputs: () => inputsMock() }));
vi.mock("./use-now-tick", () => ({ useNowTick: () => 1_000 }));
vi.mock("./use-coordinator-watch-set", () => ({ useCoordinatorWatchSet: () => watchMock() }));

import { useCoordinatorAttention } from "./use-coordinator-attention";

const task = (id: string, workflowId: string | null): AttentionTask => ({
  id,
  title: id,
  workflowId,
});
const stall = (taskId: string): Stall => ({
  task_id: taskId,
  stalled_for_ms: 60_000,
  last_event_at: "2026-09-27T00:00:00Z",
  detected_at: "2026-09-27T00:01:00Z",
});
const watch = (value: unknown, error = false, loadedAt: number | undefined = 5) => ({
  input: { value, error, loadedAt },
  retry: retryWatch,
});

beforeEach(() => {
  vi.clearAllMocks();
  tasksMock.mockReturnValue({
    tasks: [task("in", "wf-1"), task("out", "wf-2"), task("none", null)],
    stepNameByTaskId: new Map(),
    workflowNameById: new Map(),
    stepNameByWorkflowStep: new Map(),
    error: false,
    loadedAt: 1,
    retry: retryTasks,
  });
  inputsMock.mockReturnValue({
    stalls: { value: [stall("in"), stall("out")], loadedAt: 1, error: false },
    proposals: { value: [], loadedAt: 1, error: false },
    retryFailed: retryStallsAndProposals,
  });
});

describe("useCoordinatorAttention with phase 2", () => {
  it("filters Needs you and the count to the watched boards, keeping openTasksById whole", () => {
    watchMock.mockReturnValue(watch({ scope: "selected", workflowIds: ["wf-1"] }));
    const { result } = renderHook(() => useCoordinatorAttention("w", "c"));
    const ids = result.current.classification.needsYou.map((item) =>
      item.kind === "stall" ? item.stall.task_id : "",
    );
    expect(ids).toEqual(["in"]);
    expect(result.current.computeNeedsYouCount()).toBe(1);
    expect(result.current.openTasksById.size).toBe(3);
    expect(result.current.tasks).toHaveLength(3);
    expect(result.current.watchSetUnavailable).toBe(false);
  });

  it("watches every board when the scope is all, including an unattributed task", () => {
    watchMock.mockReturnValue(watch({ scope: "all", workflowIds: [] }));
    const { result } = renderHook(() => useCoordinatorAttention("w", "c"));
    expect(result.current.classification.needsYou).toHaveLength(2);
  });

  it("fails closed while the watch set has not loaded", () => {
    watchMock.mockReturnValue(watch(undefined, false, undefined));
    const { result } = renderHook(() => useCoordinatorAttention("w", "c"));
    expect(result.current.watchSetUnavailable).toBe(true);
    expect(result.current.classification.needsYou).toEqual([]);
  });

  it("adds the watches input last and retries it when it failed", () => {
    watchMock.mockReturnValue(watch(undefined, true, undefined));
    const { result } = renderHook(() => useCoordinatorAttention("w", "c"));
    expect(result.current.inputs.map((input) => input.kind)).toEqual([
      "tasks",
      "stalls",
      "proposals",
      "watches",
    ]);
    result.current.retryFailed();
    expect(retryWatch).toHaveBeenCalledTimes(1);
  });

  it("keeps the last watch set on a failed re-read", () => {
    watchMock.mockReturnValue(watch({ scope: "selected", workflowIds: ["wf-2"] }, true, 5));
    const { result } = renderHook(() => useCoordinatorAttention("w", "c"));
    expect(result.current.watchSetUnavailable).toBe(false);
    expect(result.current.classification.needsYou).toHaveLength(1);
    expect(result.current.inputs[3]).toEqual({ kind: "watches", error: true, loadedAt: 5 });
  });

  it("adds a projects input and retries the watch set while the project list is unknown", () => {
    watchMock.mockReturnValue(
      watch({
        scope: "all",
        workflowIds: [],
        projects: { scope: "selected", repositoryIds: null, includeNoRepository: false },
      }),
    );
    const { result } = renderHook(() => useCoordinatorAttention("w", "c"));
    expect(result.current.inputs.at(-1)).toEqual({ kind: "projects", error: true, loadedAt: 5 });
    result.current.retryFailed();
    expect(retryWatch).toHaveBeenCalledTimes(1);
  });

  it("adds no projects input once the project list is known", () => {
    watchMock.mockReturnValue(
      watch({
        scope: "all",
        workflowIds: [],
        projects: { scope: "selected", repositoryIds: ["r"], includeNoRepository: false },
      }),
    );
    const { result } = renderHook(() => useCoordinatorAttention("w", "c"));
    expect(result.current.inputs.map((input) => input.kind)).not.toContain("projects");
  });
});
