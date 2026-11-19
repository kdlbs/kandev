import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { sessionId, taskId } from "@/lib/types/http";
import type { QueueStatus } from "@/lib/state/slices/session/types";
import { useQueueRefetch } from "./use-queue-refetch";
import { useQueueAutomationActions } from "./use-queue-mutations";

const api = vi.hoisted(() => ({
  getQueueStatus: vi.fn(),
  setQueueAutoMerge: vi.fn(),
  setQueueAutoRun: vi.fn(),
  sendQueuedNow: vi.fn(),
}));
vi.mock("@/lib/api/domains/queue-api", () => api);
const identity = { task_id: "task", session_id: "session", session_incarnation_id: "inc" };
function status(generation: number, enabled = true): QueueStatus {
  return {
    ...identity,
    entries: [],
    count: 0,
    max: 10,
    status_epoch: "boot",
    status_generation: generation,
    auto_merge_available: true,
    auto_merge_enabled: enabled,
    auto_merge_source: "session",
    auto_merge_revision: generation,
    merge_enabled: true,
  };
}
function harness() {
  const store = createAppStore();
  store.getState().setTaskSession({
    id: sessionId("session"),
    task_id: taskId("task"),
    state: "RUNNING",
    started_at: "2026-11-05T00:00:00Z",
    updated_at: "2026-11-05T00:00:00Z",
    queue_incarnation_id: "inc",
  });
  const hook = renderHook(() => {
    const { setQueueEntries, beginQueueOperation, finishQueueOperation } = store.getState();
    const { refetch } = useQueueRefetch(
      identity,
      setQueueEntries,
      beginQueueOperation,
      finishQueueOperation,
    );
    return {
      refetch,
      ...useQueueAutomationActions({
        identity,
        beginQueueOperation,
        finishQueueOperation,
        refetch,
      }),
    };
  });
  return { store, ...hook };
}
beforeEach(() => vi.resetAllMocks());
afterEach(cleanup);

describe("authoritative Auto-merge recovery", () => {
  it("projects an initial read, keeps the known preference while unavailable, and restores it on a newer read", async () => {
    const { store, result } = harness();
    api.getQueueStatus.mockResolvedValueOnce(status(1));
    await act(async () => result.current.refetch("session"));
    expect(store.getState().queue.metaBySessionId.session).toMatchObject({
      autoMergeEnabled: true,
      autoMergeAvailable: true,
      autoMergeRevision: 1,
    });
    const unavailable = status(2);
    delete unavailable.auto_merge_enabled;
    delete unavailable.auto_merge_source;
    delete unavailable.auto_merge_revision;
    unavailable.auto_merge_available = false;
    api.getQueueStatus.mockResolvedValueOnce(unavailable);
    await act(async () => result.current.refetch("session"));
    expect(store.getState().queue.metaBySessionId.session).toMatchObject({
      autoMergeEnabled: true,
      autoMergeAvailable: false,
      autoMergeRevision: 1,
    });
    api.getQueueStatus.mockResolvedValueOnce(status(3, false));
    await act(async () => result.current.refetch("session"));
    expect(store.getState().queue.metaBySessionId.session).toMatchObject({
      autoMergeEnabled: false,
      autoMergeAvailable: true,
      autoMergeRevision: 3,
    });
  });

  it("holds the shared mutation token through the authoritative refetch and ignores the mutation response tuple", async () => {
    const mutation = Promise.withResolvers<unknown>();
    const read = Promise.withResolvers<QueueStatus>();
    api.setQueueAutoMerge.mockReturnValueOnce(mutation.promise);
    api.getQueueStatus.mockReturnValueOnce(read.promise);
    const { store, result } = harness();
    let pending!: Promise<void>;
    act(() => {
      pending = result.current.setAutoMerge(true);
    });
    expect(store.getState().queue.activeOperationBySessionId.session).toBeDefined();
    await act(async () => result.current.setAutoRun(false));
    expect(api.setQueueAutoRun).not.toHaveBeenCalled();
    await act(async () =>
      mutation.resolve({ auto_merge_enabled: false, auto_merge_revision: 999 }),
    );
    expect(store.getState().queue.activeOperationBySessionId.session).toBeDefined();
    await act(async () => {
      read.resolve(status(7));
      await pending;
    });
    expect(store.getState().queue.activeOperationBySessionId.session).toBeUndefined();
    expect(store.getState().queue.metaBySessionId.session).toMatchObject({
      autoMergeEnabled: true,
      autoMergeRevision: 7,
    });
  });

  it("restores authoritative policy after a failed switch and releases the shared token before propagating its error", async () => {
    const failure = new Error("compaction rolled back");
    api.setQueueAutoMerge.mockRejectedValueOnce(failure);
    api.getQueueStatus.mockResolvedValueOnce(status(2, false));
    const { store, result } = harness();
    await act(async () => {
      await expect(result.current.setAutoMerge(true)).rejects.toBe(failure);
    });
    expect(store.getState().queue.metaBySessionId.session).toMatchObject({
      autoMergeEnabled: false,
      autoMergeAvailable: true,
    });
    expect(store.getState().queue.activeOperationBySessionId.session).toBeUndefined();
  });
});
