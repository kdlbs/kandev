import { describe, expect, it, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import type { WorkloadRunObservation } from "@/lib/types/background-work";
import type { BackgroundWorkState } from "@/lib/state/slices/session-runtime/types";

const SESSION_ID = "session-1";
const BUILD_WATCHER = "Build Watcher";
const listBackgroundWorkloads = vi.fn();
const executeBackgroundAction = vi.fn();

vi.mock("@/lib/api/domains/background-work-api", () => ({
  listBackgroundWorkloads: (...args: unknown[]) => listBackgroundWorkloads(...args),
  executeBackgroundAction: (...args: unknown[]) => executeBackgroundAction(...args),
}));

let storeState: Record<string, unknown>;
const beginBackgroundWorkRead = vi.fn((sessionId: string) => {
  const backgroundWork = storeState.backgroundWork as BackgroundWorkState;
  const generation = backgroundWork.nextReadGeneration ?? 1;
  backgroundWork.nextReadGeneration = generation + 1;
  backgroundWork.readGenerationBySessionId ??= {};
  backgroundWork.readGenerationBySessionId[sessionId] = generation;
  return generation;
});
const setBackgroundWorkLoading = vi.fn((sessionId: string, loading: boolean) => {
  (storeState.backgroundWork as BackgroundWorkState).loadingBySessionId[sessionId] = loading;
});
const setBackgroundWorkloads = vi.fn((sessionId: string, workloads: WorkloadRunObservation[]) => {
  storeState.backgroundWork = {
    ...((storeState.backgroundWork as Record<string, unknown>) || {}),
    workloadsBySessionId: {
      ...((storeState.backgroundWork as Record<string, Record<string, unknown>>)
        ?.workloadsBySessionId || {}),
      [sessionId]: workloads,
    },
  };
});
const setActiveBackgroundWorkload = vi.fn((sessionId: string, workId: string) => {
  storeState.backgroundWork = {
    ...((storeState.backgroundWork as Record<string, unknown>) || {}),
    activeWorkIdBySessionId: {
      ...((storeState.backgroundWork as Record<string, Record<string, unknown>>)
        ?.activeWorkIdBySessionId || {}),
      [sessionId]: workId,
    },
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) => selector(storeState),
  useAppStoreApi: () => ({
    getState: () => ({
      ...storeState,
      beginBackgroundWorkRead,
      setBackgroundWorkLoading,
      setBackgroundWorkloads,
      setActiveBackgroundWorkload,
    }),
  }),
}));

import { useBackgroundWork } from "./use-background-work";

describe("useBackgroundWork fetching", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listBackgroundWorkloads.mockResolvedValue({ workloads: [] });
    storeState = {
      features: {
        agentBackgroundWork: true,
      },
      backgroundWork: {
        workloadsBySessionId: {},
        activeWorkIdBySessionId: {},
        loadingBySessionId: {},
      },
    };
  });

  it("reports initial loading until the first session read settles", async () => {
    let resolveList: ((value: { workloads: WorkloadRunObservation[] }) => void) | undefined;
    listBackgroundWorkloads.mockReturnValueOnce(
      new Promise<{ workloads: WorkloadRunObservation[] }>((resolve) => {
        resolveList = resolve;
      }),
    );

    const { result } = renderHook(() => useBackgroundWork(SESSION_ID));

    expect(result.current.isInitialLoading).toBe(true);
    expect(result.current.workloads).toEqual([]);

    await act(async () => {
      resolveList?.({
        workloads: [
          {
            work_id: "work-1",
            title: BUILD_WATCHER,
            kind: "shell",
            state: "running",
            capabilities: {
              discovery: "snapshot",
              output: "stream",
              transcript: false,
              parentage: false,
              reasoning_summary: false,
              attributable_usage: false,
            },
            revision: 1,
          },
        ],
      });
    });

    await waitFor(() => {
      expect(result.current.isInitialLoading).toBe(false);
    });
    expect(result.current.workloads.map((row) => row.work_id)).toEqual(["work-1"]);
    expect(result.current.loadError).toBe(false);
  });

  it("marks a failed first read instead of treating it as verified empty", async () => {
    listBackgroundWorkloads.mockRejectedValueOnce(new Error("read failed"));

    const { result } = renderHook(() => useBackgroundWork(SESSION_ID));

    await waitFor(() => {
      expect(result.current.isInitialLoading).toBe(false);
    });
    expect(result.current.loadError).toBe(true);
    expect(result.current.workloads).toEqual([]);
  });
});

describe("useBackgroundWork request generations", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    storeState = {
      features: {
        agentBackgroundWork: true,
      },
      backgroundWork: {
        workloadsBySessionId: {},
        activeWorkIdBySessionId: {},
        loadingBySessionId: {},
      },
    };
  });

  it("does not settle readiness from an older read across A-B-A session changes", async () => {
    const pending: Array<(value: { workloads: WorkloadRunObservation[] }) => void> = [];
    listBackgroundWorkloads.mockImplementation(
      () =>
        new Promise<{ workloads: WorkloadRunObservation[] }>((resolve) => {
          pending.push(resolve);
        }),
    );

    const { result, rerender } = renderHook(
      ({ sessionId }: { sessionId: string }) => useBackgroundWork(sessionId),
      { initialProps: { sessionId: "session-a" } },
    );
    await waitFor(() => expect(pending).toHaveLength(1));

    rerender({ sessionId: "session-b" });
    await waitFor(() => expect(pending).toHaveLength(2));

    rerender({ sessionId: "session-a" });
    await waitFor(() => expect(pending).toHaveLength(3));

    await act(async () => {
      pending[0]?.({ workloads: [] });
    });
    expect(result.current.isInitialLoading).toBe(true);

    await act(async () => {
      pending[2]?.({ workloads: [] });
    });
    await waitFor(() => expect(result.current.isInitialLoading).toBe(false));
  });

  it("does not report initial loading without a fetched session", () => {
    const { result } = renderHook(() => useBackgroundWork(null));
    expect(result.current.isInitialLoading).toBe(false);
  });

  it("fetches workloads on mount for a session", async () => {
    const mockWorkloads: WorkloadRunObservation[] = [
      {
        work_id: "work-1",
        title: BUILD_WATCHER,
        kind: "shell",
        state: "running",
        capabilities: {
          discovery: "snapshot",
          output: "stream",
          transcript: false,
          parentage: false,
          reasoning_summary: false,
          attributable_usage: false,
        },
        revision: 1,
      },
    ];
    listBackgroundWorkloads.mockResolvedValueOnce({ workloads: mockWorkloads });

    renderHook(() => useBackgroundWork(SESSION_ID));

    await waitFor(() => {
      expect(listBackgroundWorkloads).toHaveBeenCalledWith(SESSION_ID);
      expect(setBackgroundWorkloads).toHaveBeenCalledWith(SESSION_ID, mockWorkloads);
    });
  });
});

describe("useBackgroundWork calculations", () => {
  it("calculates counts and active workload correctly", () => {
    const mockWorkloads: WorkloadRunObservation[] = [
      {
        work_id: "work-1",
        title: BUILD_WATCHER,
        kind: "shell",
        state: "running",
        capabilities: {
          discovery: "snapshot",
          output: "stream",
          transcript: false,
          parentage: false,
          reasoning_summary: false,
          attributable_usage: false,
        },
        revision: 1,
      },
      {
        work_id: "work-2",
        title: "Subagent Task",
        kind: "subagent",
        state: "waiting",
        capabilities: {
          discovery: "snapshot",
          output: "stream",
          transcript: true,
          parentage: true,
          reasoning_summary: true,
          attributable_usage: true,
        },
        revision: 1,
      },
    ];

    storeState.backgroundWork = {
      workloadsBySessionId: {
        [SESSION_ID]: mockWorkloads,
      },
      activeWorkIdBySessionId: {
        [SESSION_ID]: "work-2",
      },
      loadingBySessionId: {},
    };

    const { result } = renderHook(() => useBackgroundWork(SESSION_ID));

    expect(result.current.totalCount).toBe(2);
    expect(result.current.runningCount).toBe(1);
    expect(result.current.waitingCount).toBe(1);
    expect(result.current.unknownCount).toBe(0);
    expect(result.current.totalJobCount).toBe(2);
    expect(result.current.runningWorkloads.map((w) => w.work_id)).toEqual(["work-1"]);
    expect(result.current.activeWorkloads.map((w) => w.work_id)).toEqual(["work-1", "work-2"]);
    expect(result.current.historyWorkloads).toEqual([]);
    expect(result.current.hasActiveWork).toBe(true);
    expect(result.current.activeWorkload?.work_id).toBe("work-2");
  });
});

describe("useBackgroundWork history views", () => {
  it("counts unknown activity separately and keeps terminal history", () => {
    storeState.backgroundWork = {
      workloadsBySessionId: {
        [SESSION_ID]: [
          {
            work_id: "work-1",
            title: "CI poller",
            kind: "shell",
            state: "unknown",
            capabilities: {
              discovery: "events_only",
              output: "none",
              transcript: false,
              parentage: false,
              reasoning_summary: false,
              attributable_usage: false,
            },
            revision: 1,
          },
          {
            work_id: "work-2",
            title: "Local tests",
            kind: "shell",
            state: "completed",
            capabilities: {
              discovery: "events_only",
              output: "none",
              transcript: false,
              parentage: false,
              reasoning_summary: false,
              attributable_usage: false,
            },
            revision: 2,
          },
        ],
      },
      activeWorkIdBySessionId: {},
      loadingBySessionId: {},
    };

    const { result } = renderHook(() => useBackgroundWork(SESSION_ID));

    expect(result.current.unknownCount).toBe(1);
    expect(result.current.runningCount).toBe(0);
    expect(result.current.totalCount).toBe(1);
    expect(result.current.totalJobCount).toBe(2);
    expect(result.current.runningWorkloads).toEqual([]);
    expect(result.current.activeWorkloads).toEqual([]);
    expect(result.current.historyWorkloads.map((w) => w.work_id)).toEqual(["work-1", "work-2"]);
    expect(result.current.hasActiveWork).toBe(true);
    expect(result.current.hasHistory).toBe(true);
  });
});

describe("useBackgroundWork actions", () => {
  it("executes background action with target workload", async () => {
    executeBackgroundAction.mockResolvedValueOnce({
      success: true,
      work_id: "work-1",
      action: "stop",
    });

    storeState.backgroundWork = {
      workloadsBySessionId: {
        [SESSION_ID]: [
          {
            work_id: "work-1",
            title: BUILD_WATCHER,
            kind: "shell",
            state: "running",
            capabilities: {
              discovery: "snapshot",
              output: "stream",
              transcript: false,
              parentage: false,
              reasoning_summary: false,
              attributable_usage: false,
            },
            revision: 1,
          },
        ],
      },
      activeWorkIdBySessionId: {
        [SESSION_ID]: "work-1",
      },
      loadingBySessionId: {},
    };

    const { result } = renderHook(() => useBackgroundWork(SESSION_ID));

    let actionRes;
    await act(async () => {
      actionRes = await result.current.executeAction({ action: "stop" });
    });

    expect(executeBackgroundAction).toHaveBeenCalledWith(SESSION_ID, {
      work_id: "work-1",
      action: "stop",
    });
    expect(actionRes).toEqual({ success: true, work_id: "work-1", action: "stop" });
  });
});
