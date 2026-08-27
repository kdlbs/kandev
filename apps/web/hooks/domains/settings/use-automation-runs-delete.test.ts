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

const BATCH_DELETE_ERROR = "batch delete failed";
const NETWORK_DOWN = "network down";

beforeEach(() => {
  runsStore.reset();
  vi.mocked(listAutomationRuns).mockReset();
  vi.mocked(deleteAutomationRun).mockReset();
  vi.mocked(deleteAllAutomationRuns).mockReset();
  vi.mocked(toast.error).mockReset();
});

describe("useAutomationRuns - status-scoped delete-all", () => {
  it("deletes exactly the given runs through the per-run API and removes them optimistically", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    const runZ = mkRun("run-z");
    setRuns(AUTOMATION_ID, [runX, runY, runZ]);

    vi.mocked(listAutomationRuns).mockReturnValue(Promise.withResolvers<AutomationRun[]>().promise);
    vi.mocked(deleteAutomationRun).mockResolvedValue({ deleted: true });

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns(["run-x", "run-z"]);
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-y"]);
    expect(deleteAutomationRun).toHaveBeenCalledTimes(2);
    expect(deleteAutomationRun).toHaveBeenCalledWith("run-x", WORKSPACE_ID);
    expect(deleteAutomationRun).toHaveBeenCalledWith("run-z", WORKSPACE_ID);
    expect(deleteAllAutomationRuns).not.toHaveBeenCalled();
  });

  it("keeps a run created after the deletes completed when reconciling the scoped view", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    const runNew = mkRun("run-new");
    setRuns(AUTOMATION_ID, [runX, runY]);

    const del = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAutomationRun).mockReturnValue(del.promise);
    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise) // mount
      .mockResolvedValue([runY, runNew]); // authoritative post-delete refresh

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns(["run-x"]);
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-y"]);

    // The success path must end with exactly the authoritative post-delete
    // list: a blanket re-clear would drop run-new, which was created after
    // the deletes completed server-side.
    await act(async () => {
      del.resolve({ deleted: true });
      await del.promise;
    });
    rerender();
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-new", "run-y"]);
  });
});

describe("useAutomationRuns - status-scoped delete-all failure recovery", () => {
  it("shows one aggregated toast and reverts to the server list when any delete fails", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    vi.mocked(listAutomationRuns).mockResolvedValue([runX, runY]);
    vi.mocked(deleteAutomationRun).mockRejectedValue(new Error(BATCH_DELETE_ERROR));

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    await act(async () => {});
    rerender();

    await act(async () => {
      result.current.deleteAllRuns(["run-x", "run-y"]);
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();

    // One toast for the whole batch, not one per failed delete.
    expect(toast.error).toHaveBeenCalledTimes(1);
    expect(toast.error).toHaveBeenCalledWith(BATCH_DELETE_ERROR);
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);
    // The serialization slot reopens once the recovery settles.
    expect(result.current.deleting).toBe(false);
  });

  it("restores the pre-delete snapshot if a delete fails and the recovery refresh also fails", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    vi.mocked(listAutomationRuns)
      .mockResolvedValueOnce([runX, runY])
      .mockRejectedValueOnce(new Error(NETWORK_DOWN));
    vi.mocked(deleteAutomationRun).mockRejectedValue(new Error(BATCH_DELETE_ERROR));

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    await act(async () => {});
    rerender();

    await act(async () => {
      result.current.deleteAllRuns(["run-x"]);
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();

    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);
    expect(toast.error).toHaveBeenCalledTimes(2); // delete failure + could-not-refresh
    // The serialization slot reopens even when the recovery refresh fails.
    expect(result.current.deleting).toBe(false);
  });

  it("treats an empty id list as a no-op", () => {
    setRuns(AUTOMATION_ID, [mkRun("run-x")]);
    vi.mocked(listAutomationRuns).mockReturnValue(Promise.withResolvers<AutomationRun[]>().promise);

    const { result } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns([]);
    });
    expect(result.current.runs).toHaveLength(1);
    expect(deleteAutomationRun).not.toHaveBeenCalled();
    expect(deleteAllAutomationRuns).not.toHaveBeenCalled();
  });
});

describe("useAutomationRuns - scoped delete-all stale list responses", () => {
  it("discards a refresh that started before a scoped delete-all and resolves after it succeeds", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    const stale = deferred<AutomationRun[]>();
    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise) // mount
      .mockReturnValueOnce(stale.promise) // refresh started before the delete
      .mockResolvedValue([runY]); // authoritative post-delete refresh
    vi.mocked(deleteAutomationRun).mockResolvedValue({ deleted: true });

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.refresh(); // in flight when the delete starts
      result.current.deleteAllRuns(["run-x"]); // succeeds immediately
    });
    // Let the delete's success re-removal land before the stale list resolves.
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-y"]);

    // The stale pre-delete list resolves after the delete confirmed — the
    // mutation guard must discard it instead of resurrecting run-x.
    await act(async () => {
      stale.resolve([runX, runY]);
      await stale.promise;
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-y"]);
  });

  it("ignores a refresh while a scoped delete-all is in flight", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise) // mount
      .mockResolvedValue([runY]); // authoritative post-delete refresh
    vi.mocked(deleteAutomationRun).mockResolvedValue({ deleted: true });

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns(["run-x"]);
      result.current.refresh(); // no-op while the delete owns the store
    });
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();
    // Only the mount fetch and the post-delete reconciliation ran; the
    // in-flight refresh was ignored at the hook level.
    expect(listAutomationRuns).toHaveBeenCalledTimes(2);
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-y"]);
  });
});
describe("useAutomationRuns - unscoped delete-all stale list responses", () => {
  it("ignores a refresh while an unscoped delete-all is in flight", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise) // mount
      .mockResolvedValue([]); // authoritative post-delete refresh
    vi.mocked(deleteAllAutomationRuns).mockResolvedValue({ deleted: true });

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns();
      result.current.refresh(); // no-op while the delete owns the store
    });
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();
    expect(listAutomationRuns).toHaveBeenCalledTimes(2); // mount + reconciliation
    expect(result.current.runs).toEqual([]);
  });
});
describe("useAutomationRuns - single-run delete invalidation", () => {
  it("ignores a refresh while a single-run delete is in flight", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    const del = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAutomationRun).mockReturnValue(del.promise);
    vi.mocked(listAutomationRuns).mockReturnValueOnce(
      Promise.withResolvers<AutomationRun[]>().promise,
    ); // mount

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteRun("run-x");
      result.current.refresh(); // no-op while the delete owns the store
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-y"]);

    await act(async () => {
      del.resolve({ deleted: true });
      await del.promise;
    });
    rerender();
    expect(listAutomationRuns).toHaveBeenCalledTimes(1); // mount only
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-y"]);
  });
});
describe("useAutomationRuns - loading flag", () => {
  it("keeps the loading flag until the latest list request settles", async () => {
    setRuns(AUTOMATION_ID, []);
    const first = deferred<AutomationRun[]>();
    const second = deferred<AutomationRun[]>();
    vi.mocked(listAutomationRuns)
      .mockResolvedValueOnce([]) // mount
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    await act(async () => {});
    rerender();
    expect(result.current.loading).toBe(false);

    act(() => {
      result.current.refresh(); // first request
      result.current.refresh(); // second, newer request
    });
    rerender();
    expect(result.current.loading).toBe(true);

    // The older request settling must not clear the newer request's loading.
    await act(async () => {
      first.resolve([mkRun("run-x")]);
      await first.promise;
    });
    rerender();
    expect(result.current.loading).toBe(true);

    await act(async () => {
      second.resolve([]);
      await second.promise;
    });
    rerender();
    expect(result.current.loading).toBe(false);
  });

  it("does not let an older list response overwrite a newer one", async () => {
    setRuns(AUTOMATION_ID, []);
    const older = deferred<AutomationRun[]>();
    const newer = deferred<AutomationRun[]>();
    vi.mocked(listAutomationRuns)
      .mockResolvedValueOnce([]) // mount
      .mockReturnValueOnce(older.promise)
      .mockReturnValueOnce(newer.promise);

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    await act(async () => {});
    rerender();

    act(() => {
      result.current.refresh(); // older request
      result.current.refresh(); // newer request
    });
    // The newer request resolves first with the freshest data.
    await act(async () => {
      newer.resolve([mkRun("run-new")]);
      await newer.promise;
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-new"]);

    // The older request resolves later with staler data — the latest-request
    // token must discard it instead of overwriting the newer result.
    await act(async () => {
      older.resolve([mkRun("run-old")]);
      await older.promise;
    });
    rerender();
    expect(result.current.runs.map((r) => r.id)).toEqual(["run-new"]);
  });
});
describe("useAutomationRuns - batch recovery timing", () => {
  it("does not let a refresh supersede an in-flight recovery", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    const recovery = deferred<AutomationRun[]>();
    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise) // mount
      .mockReturnValueOnce(recovery.promise); // recovery refresh
    vi.mocked(deleteAutomationRun).mockRejectedValue(new Error(BATCH_DELETE_ERROR));

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns(["run-x"]); // delete fails → recovery starts
      result.current.refresh(); // must be a no-op while the recovery is in flight
    });
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();
    // refresh() was ignored at the hook level: only the mount fetch and the
    // recovery ran, so the recovery's fallback cannot be superseded.
    expect(listAutomationRuns).toHaveBeenCalledTimes(2);

    // The recovery fails → the snapshot is restored, the slot is released,
    // and the failed delete's rows stay visible.
    await act(async () => {
      recovery.reject(new Error(NETWORK_DOWN));
      await recovery.promise.catch(() => {});
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);
    expect(toast.error).toHaveBeenCalledTimes(2); // delete failure + could-not-refresh
    expect(result.current.deleting).toBe(false);
  });

  it("does not recover from a partial failure until every delete in the batch settles", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    setRuns(AUTOMATION_ID, [runX, runY]);

    const slowDelete = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAutomationRun)
      .mockRejectedValueOnce(new Error(BATCH_DELETE_ERROR))
      .mockReturnValueOnce(slowDelete.promise);
    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise) // mount
      .mockResolvedValue([runX, runY]); // recovery refresh

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns(["run-x", "run-y"]);
    });
    // run-x rejects first while run-y is still pending — the recovery must
    // not run yet, or it would race the in-flight delete.
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();
    expect(listAutomationRuns).toHaveBeenCalledTimes(1); // mount fetch only
    expect(toast.error).not.toHaveBeenCalled();

    // run-y settles successfully; only now does the batch settle and recover.
    await act(async () => {
      slowDelete.resolve({ deleted: true });
      await slowDelete.promise;
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();
    expect(listAutomationRuns).toHaveBeenCalledTimes(2); // mount + recovery
    expect(toast.error).toHaveBeenCalledTimes(1);
    // The server list (both runs still present) wins over the optimistic
    // removal once every delete has settled.
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-x", "run-y"]);
  });
});
describe("useAutomationRuns - batch recovery fallback", () => {
  it("restores only the failed-delete rows when recovery fails after a partial batch", async () => {
    const runX = mkRun("run-x");
    const runY = mkRun("run-y");
    const runZ = mkRun("run-z");
    setRuns(AUTOMATION_ID, [runX, runY, runZ]);

    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise) // mount
      .mockRejectedValueOnce(new Error(NETWORK_DOWN)); // recovery fetch fails
    vi.mocked(deleteAutomationRun)
      .mockResolvedValueOnce({ deleted: true }) // run-x succeeds server-side
      .mockRejectedValueOnce(new Error(BATCH_DELETE_ERROR)); // run-y fails

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    await act(async () => {});
    rerender();

    await act(async () => {
      result.current.deleteAllRuns(["run-x", "run-y"]);
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();

    // run-x was deleted server-side and must NOT be resurrected by the
    // snapshot fallback; run-y failed and is restored; run-z was never
    // targeted and stays.
    expect(result.current.runs.map((r) => r.id).sort()).toEqual(["run-y", "run-z"]);
    expect(toast.error).toHaveBeenCalledTimes(2); // delete failure + could-not-refresh
    expect(result.current.deleting).toBe(false);
  });

  it("toasts when the post-delete reconciliation fetch fails", async () => {
    const runX = mkRun("run-x");
    setRuns(AUTOMATION_ID, [runX]);

    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise) // mount
      .mockRejectedValueOnce(new Error(NETWORK_DOWN)); // reconciliation fails
    vi.mocked(deleteAutomationRun).mockResolvedValue({ deleted: true });

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns(["run-x"]);
    });
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();

    // The optimistic removal stays (the delete succeeded), but the failed
    // reconciliation is surfaced and the serialization slot releases.
    expect(result.current.runs.map((r) => r.id)).toEqual([]);
    expect(toast.error).toHaveBeenCalledTimes(1); // could-not-refresh copy
    expect(result.current.deleting).toBe(false);
  });
});
describe("useAutomationRuns - serialized deletes", () => {
  it("ignores a second delete-all fired while one is still in flight", () => {
    setRuns(AUTOMATION_ID, [mkRun("run-x"), mkRun("run-y"), mkRun("run-z")]);
    const slow = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAutomationRun).mockReturnValue(slow.promise);
    vi.mocked(listAutomationRuns).mockReturnValue(Promise.withResolvers<AutomationRun[]>().promise);

    const { result } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns(["run-x", "run-y"]);
      // The second, overlapping mutation must be a no-op: the two delete
      // batches are serialized so their recoveries cannot race each other.
      result.current.deleteAllRuns(["run-z"]);
    });
    expect(deleteAutomationRun).toHaveBeenCalledTimes(2); // only the first batch
  });

  it("ignores a per-run delete fired while a delete-all is in flight", () => {
    setRuns(AUTOMATION_ID, [mkRun("run-x"), mkRun("run-y")]);
    const slow = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAutomationRun).mockReturnValue(slow.promise);
    // Mount fetch stays pending; the post-delete reconciliation settles so
    // A's endDelete (via the reconciliation settle) releases the slot.
    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise)
      .mockResolvedValue([]);

    const { result } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns(["run-x"]);
      result.current.deleteRun("run-y");
    });
    expect(deleteAutomationRun).toHaveBeenCalledTimes(1); // only the delete-all
  });

  it("shares the serialization slot across hook instances for the same automation", async () => {
    setRuns(AUTOMATION_ID, [mkRun("run-x"), mkRun("run-y")]);
    const slow = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAutomationRun).mockReturnValue(slow.promise);
    // Mount fetch stays pending; the post-delete reconciliation settles so
    // A's endDelete (via the reconciliation settle) releases the slot.
    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise)
      .mockResolvedValue([]);

    // Instance A starts a delete and "unmounts" (the editor navigated away).
    const first = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    act(() => {
      first.result.current.deleteAllRuns(["run-x"]);
    });
    first.unmount();

    // A remounted instance for the same automation must see the shared
    // in-flight slot and be gated, not start its own overlapping delete —
    // and its mount fetch must not supersede A's recovery either.
    const second = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));
    expect(second.result.current.deleting).toBe(true);
    expect(listAutomationRuns).toHaveBeenCalledTimes(1); // A's mount fetch only
    act(() => {
      second.result.current.deleteAllRuns(["run-y"]);
      second.result.current.deleteRun("run-y");
    });
    expect(deleteAutomationRun).toHaveBeenCalledTimes(1); // only A's batch

    // A's delete settles and its endDelete releases the shared slot.
    await act(async () => {
      slow.resolve({ deleted: true });
      await slow.promise;
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    second.rerender();
    expect(second.result.current.deleting).toBe(false);
  });

  it("exposes deleting while a delete runs and clears it once the batch settles", async () => {
    setRuns(AUTOMATION_ID, [mkRun("run-x")]);
    const slow = deferred<{ deleted: boolean }>();
    vi.mocked(deleteAutomationRun).mockReturnValue(slow.promise);
    // Mount fetch stays pending; the post-delete reconciliation settles so
    // the serialization slot (held until it does) can release.
    vi.mocked(listAutomationRuns)
      .mockReturnValueOnce(Promise.withResolvers<AutomationRun[]>().promise)
      .mockResolvedValue([]);

    const { result, rerender } = renderHook(() => useAutomationRuns(AUTOMATION_ID, WORKSPACE_ID));

    act(() => {
      result.current.deleteAllRuns(["run-x"]);
    });
    rerender();
    expect(result.current.deleting).toBe(true);

    await act(async () => {
      slow.resolve({ deleted: true });
      await slow.promise;
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    rerender();
    expect(result.current.deleting).toBe(false);
  });
});
