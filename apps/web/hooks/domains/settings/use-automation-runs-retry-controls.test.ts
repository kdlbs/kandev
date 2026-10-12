import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AutomationRun, RetryHistoryMode } from "@/lib/types/automation";
import {
  AUTOMATION_ID,
  WORKSPACE_ID,
  deferred,
  mkRun,
  runsStore,
  setRuns,
} from "./use-automation-runs.test-utils";

vi.mock("@/components/state-provider", async () => {
  const { runsStore, mockStoreApi } = await import("./use-automation-runs.test-utils");
  return {
    useAppStore: (selector: (s: unknown) => unknown) => selector(runsStore.get()),
    useAppStoreApi: () => mockStoreApi,
  };
});
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/lib/api/domains/automation-api", () => ({
  listAutomationRuns: vi.fn(),
  listAutomationRunPage: vi.fn(),
  listAutomationRetryHistory: vi.fn(),
  deleteAutomationRun: vi.fn(),
  deleteAllAutomationRuns: vi.fn(),
  stopAutomationRun: vi.fn(),
}));

import { toast } from "sonner";
import {
  deleteAllAutomationRuns,
  deleteAutomationRun,
  listAutomationRetryHistory,
  listAutomationRunPage,
  listAutomationRuns,
  stopAutomationRun,
} from "@/lib/api/domains/automation-api";
import { useAutomationRuns } from "./use-automation-runs";

beforeEach(() => {
  runsStore.reset();
  vi.resetAllMocks();
  vi.mocked(listAutomationRuns).mockResolvedValue([]);
  vi.mocked(listAutomationRunPage).mockResolvedValue({ items: [], next_cursor: "" });
  vi.mocked(listAutomationRetryHistory).mockResolvedValue({
    scope: AUTOMATION_ID,
    items: [],
    high_water_mark: "watermark",
  });
  vi.mocked(deleteAutomationRun).mockResolvedValue({ deleted: true });
  vi.mocked(deleteAllAutomationRuns).mockResolvedValue({ deleted: true });
});

describe("retry history mutation ownership", () => {
  it.each(["all", "scoped", "failed-single", "failed-all"])(
    "retains timeline mode after %s deletion",
    async (kind) => {
      const { result, rerender } = renderHook(() =>
        useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID, "timeline"),
      );
      await act(async () => {});
      setRuns(AUTOMATION_ID, [mkRun("old")]);
      rerender();
      const attempt = mkRun("retry-attempt");
      vi.mocked(listAutomationRetryHistory).mockResolvedValue({
        scope: AUTOMATION_ID,
        items: [{ retry_group_id: "group", trigger_ids: [], attempts: [attempt], completed: true }],
        high_water_mark: "watermark",
      });
      if (kind === "failed-single")
        vi.mocked(deleteAutomationRun).mockRejectedValue(new Error("delete failed"));
      if (kind === "failed-all")
        vi.mocked(deleteAllAutomationRuns).mockRejectedValue(new Error("delete failed"));
      await act(async () => {
        if (kind === "scoped") result.current.deleteAllRuns(["old"]);
        else if (kind === "failed-single") result.current.deleteRun("old");
        else result.current.deleteAllRuns();
      });
      rerender();
      expect(result.current.runs.map((run) => run.id)).toEqual(["retry-attempt"]);
      expect(listAutomationRuns).not.toHaveBeenCalled();
    },
  );

  it("supersedes an in-flight attempts request when timeline is selected", async () => {
    const old = deferred<AutomationRun[]>();
    vi.mocked(listAutomationRuns).mockReturnValue(old.promise);
    vi.mocked(listAutomationRunPage).mockResolvedValue({
      items: [mkRun("new-mode")],
      next_cursor: "",
    });
    const { result, rerender } = renderHook(
      ({ mode }: { mode: RetryHistoryMode }) =>
        useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID, mode),
      { initialProps: { mode: "attempts" } },
    );
    rerender({ mode: "timeline" });
    await act(async () => {});
    await act(async () => {
      old.resolve([mkRun("stale-mode")]);
    });
    rerender({ mode: "timeline" });
    expect(result.current.runs.map((run) => run.id)).toEqual(["new-mode"]);
  });
});

describe("retry Stop control", () => {
  it("remains busy until every distinct pending Stop settles", async () => {
    const older = deferred<Awaited<ReturnType<typeof stopAutomationRun>>>();
    const newer = deferred<Awaited<ReturnType<typeof stopAutomationRun>>>();
    vi.mocked(stopAutomationRun).mockImplementation((_automationId, runId) =>
      runId === "older" ? older.promise : newer.promise,
    );
    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    let first: Promise<void>;
    let second: Promise<void>;
    act(() => {
      first = result.current.stopRun("older");
      second = result.current.stopRun("newer");
    });
    await act(async () => {
      newer.resolve({ run_id: "newer", status: "cancelled" });
      await second!;
    });
    rerender();
    expect(result.current.stopping).toBe(true);
    await act(async () => {
      older.resolve({ run_id: "older", status: "cancelled" });
      await first!;
    });
    rerender();
    expect(result.current.stopping).toBe(false);
  });

  it("catches failure, reports it, and clears busy state after duplicate submissions", async () => {
    const pending = deferred<Awaited<ReturnType<typeof stopAutomationRun>>>();
    vi.mocked(stopAutomationRun).mockReturnValue(pending.promise);
    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    let first: Promise<void>;
    let duplicate: Promise<void>;
    act(() => {
      first = result.current.stopRun("run");
      duplicate = result.current.stopRun("run");
    });
    rerender();
    expect(stopAutomationRun).toHaveBeenCalledTimes(1);
    expect(result.current.stopping).toBe(true);
    await act(async () => {
      pending.reject(new Error("stop failed"));
      await expect(first!).resolves.toBeUndefined();
      await expect(duplicate!).resolves.toBeUndefined();
    });
    rerender();
    expect(toast.error).toHaveBeenCalledTimes(1);
    expect(result.current.stopping).toBe(false);
  });

  it("guards the same Stop across consumers of the same store", async () => {
    const pending = deferred<Awaited<ReturnType<typeof stopAutomationRun>>>();
    vi.mocked(stopAutomationRun).mockReturnValue(pending.promise);
    const first = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    const second = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    let stopped: Promise<void>;
    act(() => {
      stopped = first.result.current.stopRun("run");
      void second.result.current.stopRun("run");
    });
    expect(stopAutomationRun).toHaveBeenCalledTimes(1);
    await act(async () => {
      pending.resolve({ run_id: "run", status: "cancelled" });
      await stopped!;
    });
  });
});
