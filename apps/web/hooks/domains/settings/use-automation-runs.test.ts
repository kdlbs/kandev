import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import type { AutomationRun } from "@/lib/types/automation";

import {
  AUTOMATION_ID,
  WORKSPACE_ID,
  runsStore,
  deferred,
  mkRun,
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

beforeEach(() => {
  runsStore.reset();
  vi.mocked(listAutomationRuns).mockReset();
  vi.mocked(deleteAutomationRun).mockReset();
  vi.mocked(deleteAllAutomationRuns).mockReset();
  vi.mocked(toast.error).mockReset();
});

describe("useAutomationRuns", () => {
  it("does not let a refresh started before a single-run delete resurrect the row", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    // The delete request stays pending until we manually resolve it below.
    const del = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAutomationRun).mockReturnValue(del.promise);
    // The refresh starts before the delete claims the mutation epoch and
    // resolves with the pre-delete list — the epoch guard must discard it.
    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise) // mount
      .mockResolvedValue([runX, runY]);

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.refresh(); // captures the pre-delete epoch
      result.current.deleteRun("run-x"); // claims the epoch
    });
    // The pre-delete refresh resolves with the full list — it must be
    // discarded (its captured epoch went stale when the delete claimed).
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-y"]);

    // The delete confirms server-side.
    await act(async () => {
      del.resolve({ deleted: true });
      await del.promise;
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-y"]);
  });

  it("keeps a run created after delete-all completed when reconciling the full list", async () => {
    const runNew = mkRun("run-new");
    setRuns(AUTOMATION_ID, [mkRun("run-x"), mkRun("run-y")]);

    const del = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAllAutomationRuns).mockReturnValue(del.promise);
    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise) // mount
      .mockResolvedValue([runNew]); // authoritative post-delete refresh

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns();
    });
    rerender();
    expect(result.current.runs).toEqual([]);

    // The success path must end with exactly the authoritative post-delete
    // list: a blanket clear would drop run-new, which was created after the
    // backend delete completed.
    await act(async () => {
      del.resolve({ deleted: true });
      await del.promise;
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-new"]);
  });

  it("passes workspaceId through to the delete-run and delete-all-runs API calls", async () => {
    setRuns(AUTOMATION_ID, [mkRun("run-x")]);
    vi.mocked(listAutomationRuns).mockReturnValue(Promise.withResolvers<AutomationRun[]>().promise);
    vi.mocked(deleteAutomationRun).mockResolvedValue({ deleted: true });
    vi.mocked(deleteAllAutomationRuns).mockResolvedValue({ deleted: true });

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteRun("run-x");
    });
    expect(deleteAutomationRun).toHaveBeenCalledWith("run-x", WORKSPACE_ID);

    // Let the per-run delete settle so the serialization gate reopens before
    // the next mutation.
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();
    expect(result.current.deleting).toBe(false);

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

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
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
    expect(result.current.deleting).toBe(false);
  });

  it("restores the full pre-clear snapshot if both delete-all and the recovery refresh fail", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    vi.mocked(listAutomationRuns)
      .mockResolvedValueOnce([runX, runY])
      .mockRejectedValueOnce(new Error("network down"));
    vi.mocked(deleteAllAutomationRuns).mockRejectedValue(new Error("delete-all failed"));

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
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
    expect(result.current.deleting).toBe(false);
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

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
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
    expect(result.current.deleting).toBe(false);
  });

  it("shows a toast and reverts to the server list when deleteAllRuns fails but the recovery refresh succeeds", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    vi.mocked(listAutomationRuns).mockResolvedValue([runX, runY]);
    vi.mocked(deleteAllAutomationRuns).mockRejectedValue(new Error("delete-all failed"));

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
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
    expect(result.current.deleting).toBe(false);
  });
});
