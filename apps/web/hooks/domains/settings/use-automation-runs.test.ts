import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, type ReactNode } from "react";
import { qk } from "@/lib/query/keys";
import type { AutomationRun } from "@/lib/types/automation";

let queryClient: QueryClient;

function setRuns(automationId: string, runs: AutomationRun[]) {
  queryClient.setQueryData(qk.automations.runs(automationId), runs);
}

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

vi.mock("@/lib/api/domains/automation-api", () => ({
  listAutomationRuns: vi.fn(),
  deleteAutomationRun: vi.fn(),
  deleteAllAutomationRuns: vi.fn(),
}));

import { toast } from "sonner";
import {
  listAutomationRuns,
  deleteAutomationRun,
  deleteAllAutomationRuns,
} from "@/lib/api/domains/automation-api";
import { useAutomationRuns } from "./use-automation-runs";

const AUTOMATION_ID = "auto-1";
const WORKSPACE_ID = "ws-1";

function wrapper({ children }: { children: ReactNode }) {
  return createElement(QueryClientProvider, { client: queryClient }, children);
}

function renderAutomationRunsHook() {
  return renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID), { wrapper });
}

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
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  vi.mocked(listAutomationRuns).mockReset();
  vi.mocked(deleteAutomationRun).mockReset();
  vi.mocked(deleteAllAutomationRuns).mockReset();
  vi.mocked(toast.error).mockReset();
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

    const { result, rerender } = renderAutomationRunsHook();

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

    const { result, rerender } = renderAutomationRunsHook();

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

    const { result } = renderAutomationRunsHook();

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

describe("useAutomationRuns - double-failure recovery", () => {
  it("restores the specific deleted run if both the delete and the recovery refresh fail", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    // Mount-effect fetch resolves immediately with the initial list so it
    // doesn't interfere with the delete-triggered recovery fetch below.
    vi.mocked(listAutomationRuns)
      .mockResolvedValueOnce([runX, runY])
      .mockRejectedValueOnce(new Error("network down"));
    vi.mocked(deleteAutomationRun).mockRejectedValue(new Error("delete failed"));

    const { result, rerender } = renderAutomationRunsHook();
    await act(async () => {});
    rerender();

    await act(async () => {
      result.current.deleteRun("run-x");
      // Flush the delete rejection and the subsequent revert-fetch rejection.
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();

    // Both the delete and the revert fetch failed — the store must not be
    // left permanently missing run-x. It should be restored from the
    // pre-delete snapshot rather than silently staying gone.
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);
  });

  it("restores the full pre-clear snapshot if both delete-all and the recovery refresh fail", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    vi.mocked(listAutomationRuns)
      .mockResolvedValueOnce([runX, runY])
      .mockRejectedValueOnce(new Error("network down"));
    vi.mocked(deleteAllAutomationRuns).mockRejectedValue(new Error("delete-all failed"));

    const { result, rerender } = renderAutomationRunsHook();
    await act(async () => {});
    rerender();

    await act(async () => {
      result.current.deleteAllRuns();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();

    // Both delete-all and the revert fetch failed — the store must not be
    // left permanently empty. It should be restored from the pre-clear
    // snapshot rather than silently staying cleared.
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);
  });
});

describe("useAutomationRuns - single-failure revert", () => {
  it("shows a toast and reverts to the server list when deleteRun fails but the recovery refresh succeeds", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    // Mount-effect fetch, then the delete-triggered recovery fetch, both
    // succeed with the server's authoritative (unchanged) list.
    vi.mocked(listAutomationRuns).mockResolvedValue([runX, runY]);
    vi.mocked(deleteAutomationRun).mockRejectedValue(new Error("delete failed"));

    const { result, rerender } = renderAutomationRunsHook();
    await act(async () => {});
    rerender();

    await act(async () => {
      result.current.deleteRun("run-x");
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();

    expect(toast.error).toHaveBeenCalledWith("delete failed");
    // The recovery refresh succeeded, so the store reflects the server's
    // authoritative list rather than the double-failure local-cache fallback.
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);
  });

  it("shows a toast and reverts to the server list when deleteAllRuns fails but the recovery refresh succeeds", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    vi.mocked(listAutomationRuns).mockResolvedValue([runX, runY]);
    vi.mocked(deleteAllAutomationRuns).mockRejectedValue(new Error("delete-all failed"));

    const { result, rerender } = renderAutomationRunsHook();
    await act(async () => {});
    rerender();

    await act(async () => {
      result.current.deleteAllRuns();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();

    expect(toast.error).toHaveBeenCalledWith("delete-all failed");
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);
  });
});
