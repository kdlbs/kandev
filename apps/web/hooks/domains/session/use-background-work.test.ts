import { describe, expect, it, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import type { WorkloadRunObservation } from "@/lib/types/background-work";

const listBackgroundWorkloads = vi.fn();
const executeBackgroundAction = vi.fn();

vi.mock("@/lib/api/domains/background-work-api", () => ({
  listBackgroundWorkloads: (...args: unknown[]) => listBackgroundWorkloads(...args),
  executeBackgroundAction: (...args: unknown[]) => executeBackgroundAction(...args),
}));

let storeState: Record<string, unknown>;
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

  it("fetches workloads on mount for a session", async () => {
    const mockWorkloads: WorkloadRunObservation[] = [
      {
        work_id: "work-1",
        title: "Build Watcher",
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

    renderHook(() => useBackgroundWork("session-1"));

    await waitFor(() => {
      expect(listBackgroundWorkloads).toHaveBeenCalledWith("session-1");
      expect(setBackgroundWorkloads).toHaveBeenCalledWith("session-1", mockWorkloads);
    });
  });
});

describe("useBackgroundWork calculations", () => {
  it("calculates counts and active workload correctly", () => {
    const mockWorkloads: WorkloadRunObservation[] = [
      {
        work_id: "work-1",
        title: "Build Watcher",
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
        "session-1": mockWorkloads,
      },
      activeWorkIdBySessionId: {
        "session-1": "work-2",
      },
      loadingBySessionId: {},
    };

    const { result } = renderHook(() => useBackgroundWork("session-1"));

    expect(result.current.totalCount).toBe(2);
    expect(result.current.runningCount).toBe(1);
    expect(result.current.waitingCount).toBe(1);
    expect(result.current.hasActiveWork).toBe(true);
    expect(result.current.activeWorkload?.work_id).toBe("work-2");
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
        "session-1": [
          {
            work_id: "work-1",
            title: "Build Watcher",
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
        "session-1": "work-1",
      },
      loadingBySessionId: {},
    };

    const { result } = renderHook(() => useBackgroundWork("session-1"));

    let actionRes;
    await act(async () => {
      actionRes = await result.current.executeAction({ action: "stop" });
    });

    expect(executeBackgroundAction).toHaveBeenCalledWith("session-1", {
      work_id: "work-1",
      action: "stop",
    });
    expect(actionRes).toEqual({ success: true, work_id: "work-1", action: "stop" });
  });
});
