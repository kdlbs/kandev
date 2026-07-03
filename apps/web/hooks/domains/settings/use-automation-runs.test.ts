import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import type { AutomationRun } from "@/lib/types/automation";

type MockState = {
  automationRuns: {
    byAutomationId: Record<string, AutomationRun[]>;
    loading: Record<string, boolean>;
  };
};

let mockState: MockState = { automationRuns: { byAutomationId: {}, loading: {} } };

function setRuns(automationId: string, runs: AutomationRun[]) {
  mockState = {
    automationRuns: {
      ...mockState.automationRuns,
      byAutomationId: { ...mockState.automationRuns.byAutomationId, [automationId]: runs },
    },
  };
}

const storeActions = {
  setAutomationRuns: (automationId: string, runs: AutomationRun[]) => setRuns(automationId, runs),
  setAutomationRunsLoading: (automationId: string, loading: boolean) => {
    mockState = {
      automationRuns: {
        ...mockState.automationRuns,
        loading: { ...mockState.automationRuns.loading, [automationId]: loading },
      },
    };
  },
  removeAutomationRun: (automationId: string, runId: string) => {
    const runs = mockState.automationRuns.byAutomationId[automationId] ?? [];
    setRuns(
      automationId,
      runs.filter((r) => r.id !== runId),
    );
  },
  clearAutomationRuns: (automationId: string) => setRuns(automationId, []),
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState & typeof storeActions) => unknown) =>
    selector({ ...mockState, ...storeActions }),
}));

vi.mock("@/lib/api/domains/automation-api", () => ({
  listAutomationRuns: vi.fn(),
  deleteAutomationRun: vi.fn(),
  deleteAllAutomationRuns: vi.fn(),
}));

import {
  listAutomationRuns,
  deleteAutomationRun,
  deleteAllAutomationRuns,
} from "@/lib/api/domains/automation-api";
import { useAutomationRuns } from "./use-automation-runs";

const AUTOMATION_ID = "auto-1";
const WORKSPACE_ID = "ws-1";

function mkRun(id: string): AutomationRun {
  return {
    id,
    automation_id: AUTOMATION_ID,
    trigger_id: "trig-1",
    trigger_type: "scheduled",
    task_id: "",
    status: "skipped",
    dedup_key: "",
    trigger_data: {},
    error_message: "",
    created_at: new Date().toISOString(),
  };
}

/** A promise plus its resolve function, for controlling when a mocked async
 * call settles relative to other events in a test. */
function deferred<T>() {
  const { promise, resolve } = Promise.withResolvers<T>();
  return { promise, resolve };
}

beforeEach(() => {
  mockState = { automationRuns: { byAutomationId: {}, loading: {} } };
  vi.mocked(listAutomationRuns).mockReset();
  vi.mocked(deleteAutomationRun).mockReset();
  vi.mocked(deleteAllAutomationRuns).mockReset();
});

describe("useAutomationRuns", () => {
  it("re-applies the optimistic removal if an in-flight refresh resurrects the row before delete confirms", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    // The delete request stays pending until we manually resolve it below,
    // so we can interleave a "stale refresh" in between.
    const del = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAutomationRun).mockReturnValue(del.promise);
    // A concurrent refresh() resolves with the pre-delete list — as if the
    // list request was already in flight when the delete was fired. The
    // hook's own mount-effect fetch (first call) is left pending so it
    // doesn't confound the explicit refresh() below.
    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise)
      .mockResolvedValue([runX, runY]);

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteRun("run-x");
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-y"]);

    // The in-flight refresh resolves and overwrites the store with the
    // stale full list, resurrecting run-x — this is the race being guarded
    // against, reproduced here explicitly.
    await act(async () => {
      await result.current.refresh();
    });
    rerender();
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);

    // The delete now confirms server-side. Without re-applying the removal
    // on success, run-x would stay resurrected until the next full refresh.
    await act(async () => {
      del.resolve({ deleted: true });
      await del.promise;
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-y"]);
  });

  it("re-applies the optimistic clear if an in-flight refresh resurrects rows before delete-all confirms", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    const del = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAllAutomationRuns).mockReturnValue(del.promise);
    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise)
      .mockResolvedValue([runX, runY]);

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns();
    });
    rerender();
    expect(result.current.runs).toEqual([]);

    await act(async () => {
      await result.current.refresh();
    });
    rerender();
    expect(result.current.runs).toHaveLength(2);

    await act(async () => {
      del.resolve({ deleted: true });
      await del.promise;
    });
    rerender();
    expect(result.current.runs).toEqual([]);
  });

  it("passes workspaceId through to the delete-run and delete-all-runs API calls", () => {
    setRuns(AUTOMATION_ID, [mkRun("run-x")]);
    vi.mocked(listAutomationRuns).mockReturnValue(Promise.withResolvers<AutomationRun[]>().promise);
    vi.mocked(deleteAutomationRun).mockResolvedValue({ deleted: true });
    vi.mocked(deleteAllAutomationRuns).mockResolvedValue({ deleted: true });

    const { result } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteRun("run-x");
    });
    expect(deleteAutomationRun).toHaveBeenCalledWith("run-x", WORKSPACE_ID);

    act(() => {
      result.current.deleteAllRuns();
    });
    expect(deleteAllAutomationRuns).toHaveBeenCalledWith(AUTOMATION_ID, WORKSPACE_ID);
  });
});
