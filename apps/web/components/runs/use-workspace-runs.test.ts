import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mockList = vi.fn();
vi.mock("@/lib/api/domains/automation-api", () => ({
  listWorkspaceAutomationRuns: (...args: unknown[]) => mockList(...args),
}));

import { useWorkspaceRuns, WORKSPACE_RUNS_LIMIT } from "./use-workspace-runs";
import type { WorkspaceAutomationRun } from "@/lib/types/automation";

const WORKSPACE = "ws-1";

function mkRun(id: string): WorkspaceAutomationRun {
  return {
    id,
    automation_id: "a1",
    trigger_id: "t1",
    trigger_type: "scheduled",
    task_id: "task-1",
    status: "succeeded",
    dedup_key: "",
    trigger_data: {},
    error_message: "",
    created_at: new Date().toISOString(),
    automation_name: "nightly",
  } as WorkspaceAutomationRun;
}

beforeEach(() => {
  mockList.mockResolvedValue([]);
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("useWorkspaceRuns", () => {
  it("loads the active workspace's runs with the capped limit", async () => {
    mockList.mockResolvedValue([mkRun("r1")]);

    const { result } = renderHook(() => useWorkspaceRuns(WORKSPACE));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mockList).toHaveBeenCalledWith(WORKSPACE, WORKSPACE_RUNS_LIMIT);
    expect(result.current.runs.map((r) => r.id)).toEqual(["r1"]);
    expect(result.current.error).toBeNull();
  });

  it("does not call the API without a workspace", async () => {
    const { result } = renderHook(() => useWorkspaceRuns(undefined));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mockList).not.toHaveBeenCalled();
    expect(result.current.runs).toEqual([]);
  });

  it("surfaces a load failure instead of showing an empty feed", async () => {
    mockList.mockRejectedValue(new Error("boom"));

    const { result } = renderHook(() => useWorkspaceRuns(WORKSPACE));

    await waitFor(() => expect(result.current.error).toBe("boom"));
    expect(result.current.loading).toBe(false);
  });

  it("recovers on refresh after a failure", async () => {
    mockList.mockRejectedValueOnce(new Error("boom")).mockResolvedValue([mkRun("r2")]);

    const { result } = renderHook(() => useWorkspaceRuns(WORKSPACE));
    await waitFor(() => expect(result.current.error).toBe("boom"));

    act(() => result.current.refresh());

    await waitFor(() => expect(result.current.runs.map((r) => r.id)).toEqual(["r2"]));
    expect(result.current.error).toBeNull();
  });
});

describe("useWorkspaceRuns request ordering", () => {
  it("lets a later refresh win over a slow first load", async () => {
    // The guard that matters: responses are ordered by issue, not arrival. A
    // first request that resolves late must not overwrite fresher data the user
    // asked for afterwards.
    let resolveFirst: (value: WorkspaceAutomationRun[]) => void = () => {};
    mockList
      .mockImplementationOnce(
        () =>
          new Promise<WorkspaceAutomationRun[]>((resolve) => {
            resolveFirst = resolve;
          }),
      )
      .mockResolvedValue([mkRun("fresh")]);

    const { result } = renderHook(() => useWorkspaceRuns(WORKSPACE));

    act(() => result.current.refresh());
    await waitFor(() => expect(result.current.runs.map((r) => r.id)).toEqual(["fresh"]));

    // The stale first response lands only now.
    await act(async () => {
      resolveFirst([mkRun("stale")]);
    });

    expect(result.current.runs.map((r) => r.id)).toEqual(["fresh"]);
  });

  it("drops the previous workspace's runs on a switch", async () => {
    // Runs are workspace-scoped. Holding the old rows while the next workspace
    // loads attributes another workspace's activity to this one.
    mockList.mockResolvedValueOnce([mkRun("from-ws-1")]);
    const { result, rerender } = renderHook(({ ws }) => useWorkspaceRuns(ws), {
      initialProps: { ws: WORKSPACE },
    });
    await waitFor(() => expect(result.current.runs.map((r) => r.id)).toEqual(["from-ws-1"]));

    let resolveSecond: (value: WorkspaceAutomationRun[]) => void = () => {};
    mockList.mockImplementationOnce(
      () =>
        new Promise<WorkspaceAutomationRun[]>((resolve) => {
          resolveSecond = resolve;
        }),
    );
    rerender({ ws: "ws-2" });

    // Mid-flight: the old workspace's rows must already be gone.
    await waitFor(() => expect(result.current.runs).toEqual([]));

    await act(async () => {
      resolveSecond([mkRun("from-ws-2")]);
    });
    expect(result.current.runs.map((r) => r.id)).toEqual(["from-ws-2"]);
  });

  it("ignores a response that lands after the workspace goes away", async () => {
    // The no-workspace branch used to return without claiming a request id, so
    // an in-flight response repopulated a feed that should be empty.
    let resolveFirst: (value: WorkspaceAutomationRun[]) => void = () => {};
    mockList.mockImplementationOnce(
      () =>
        new Promise<WorkspaceAutomationRun[]>((resolve) => {
          resolveFirst = resolve;
        }),
    );

    const { result, rerender } = renderHook(
      ({ ws }: { ws: string | undefined }) => useWorkspaceRuns(ws),
      { initialProps: { ws: WORKSPACE as string | undefined } },
    );

    rerender({ ws: undefined });
    await act(async () => {
      resolveFirst([mkRun("stale")]);
    });

    expect(result.current.runs).toEqual([]);
  });

  it("keeps the rows on screen while a same-workspace refresh is in flight", async () => {
    // The gap Codex named: clearing on every refresh, not just on a switch,
    // would still pass every other test here while yanking the feed out from
    // under a reader who pressed refresh.
    mockList.mockResolvedValueOnce([mkRun("r1")]);
    const { result } = renderHook(() => useWorkspaceRuns(WORKSPACE));
    await waitFor(() => expect(result.current.runs.map((r) => r.id)).toEqual(["r1"]));

    let resolveRefresh: (value: WorkspaceAutomationRun[]) => void = () => {};
    mockList.mockImplementationOnce(
      () =>
        new Promise<WorkspaceAutomationRun[]>((resolve) => {
          resolveRefresh = resolve;
        }),
    );
    act(() => result.current.refresh());

    // Mid-flight, the previous rows are still there.
    expect(result.current.runs.map((r) => r.id)).toEqual(["r1"]);

    await act(async () => {
      resolveRefresh([mkRun("r2")]);
    });
    expect(result.current.runs.map((r) => r.id)).toEqual(["r2"]);
  });

  it("reports loading from the moment the workspace changes", async () => {
    // The fetch is issued from a passive effect, so there is a render with no
    // rows and no request in flight. Reporting "not loading" there flashes the
    // empty state before the request even starts.
    mockList.mockResolvedValueOnce([mkRun("r1")]);
    const { result, rerender } = renderHook(({ ws }) => useWorkspaceRuns(ws), {
      initialProps: { ws: WORKSPACE },
    });
    await waitFor(() => expect(result.current.loading).toBe(false));

    rerender({ ws: "ws-2" });

    expect(result.current.loading).toBe(true);
  });

  it("refetches when the workspace changes", async () => {
    const { result, rerender } = renderHook(({ ws }) => useWorkspaceRuns(ws), {
      initialProps: { ws: WORKSPACE },
    });
    await waitFor(() => expect(result.current.loading).toBe(false));

    rerender({ ws: "ws-2" });

    await waitFor(() => expect(mockList).toHaveBeenCalledTimes(2));
    expect(mockList.mock.calls[1][0]).toBe("ws-2");
  });
});
