import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RunRead } from "@/lib/api/domains/coordinator-autonomy-api";

const getRun = vi.fn();
vi.mock("@/lib/api/domains/coordinator-autonomy-api", () => ({
  getRun: (...a: unknown[]) => getRun(...a),
}));

import {
  ensureWakeRun,
  getWakeRun,
  refreshOpenWakeRun,
  resetWakeRunStore,
  subscribeWakeRuns,
  wakeRunKey,
} from "./wake-run-store";

function run(over: Partial<RunRead> = {}): RunRead {
  return {
    id: "t1",
    coordinator_id: "c1",
    conversation_task_id: "x",
    session_id: "s",
    started_at: "2026-09-30T10:00:00Z",
    finished_at: null,
    outcome: null,
    wake_count: 2,
    denied_permissions: 0,
    cost_subcents: null,
    stop_requested_at: null,
    stop_state: null,
    wakes: [],
    ...over,
  };
}

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const KEY = wakeRunKey("c1", "t1");
const flush = () => new Promise((r) => setTimeout(r, 0));

beforeEach(() => {
  resetWakeRunStore();
  getRun.mockReset();
});
afterEach(() => resetWakeRunStore());

describe("wake run store", () => {
  it("reads once per coordinator and turn id, however many surfaces ask", async () => {
    getRun.mockResolvedValue(run({ outcome: "completed" }));
    ensureWakeRun("w1", "c1", "t1");
    ensureWakeRun("w1", "c1", "t1");
    expect(getWakeRun(KEY)?.status).toBe("loading");
    await flush();
    ensureWakeRun("w1", "c1", "t1");
    expect(getRun).toHaveBeenCalledTimes(1);
    expect(getWakeRun(KEY)?.status).toBe("loaded");
  });

  it("keys by coordinator id as well as turn id", async () => {
    getRun.mockResolvedValue(run());
    ensureWakeRun("w1", "c1", "t1");
    ensureWakeRun("w1", "c2", "t1");
    expect(getRun).toHaveBeenCalledTimes(2);
  });

  it("never re-reads a settled run", async () => {
    getRun.mockResolvedValue(run({ outcome: "completed" }));
    ensureWakeRun("w1", "c1", "t1");
    await flush();
    refreshOpenWakeRun("c1", "t1");
    expect(getRun).toHaveBeenCalledTimes(1);
  });

  it("re-reads an open run and a failed re-read keeps the loaded data", async () => {
    getRun.mockResolvedValueOnce(run());
    ensureWakeRun("w1", "c1", "t1");
    await flush();
    getRun.mockRejectedValueOnce(new Error("down"));
    refreshOpenWakeRun("c1", "t1");
    await flush();
    expect(getRun).toHaveBeenCalledTimes(2);
    expect(getWakeRun(KEY)?.status).toBe("loaded");
    getRun.mockResolvedValueOnce(run({ outcome: "completed", wake_count: 3 }));
    refreshOpenWakeRun("c1", "t1");
    await flush();
    expect(getWakeRun(KEY)?.run?.wake_count).toBe(3);
  });

  it("falls to unavailable only on a failed first read, and does not retry it", async () => {
    getRun.mockRejectedValue(new Error("404"));
    ensureWakeRun("w1", "c1", "t1");
    await flush();
    expect(getWakeRun(KEY)?.status).toBe("unavailable");
    refreshOpenWakeRun("c1", "t1");
    ensureWakeRun("w1", "c1", "t1");
    expect(getRun).toHaveBeenCalledTimes(1);
  });

  it("re-reads while the first read is in flight and drops the older response", async () => {
    const first = deferred<RunRead>();
    const second = deferred<RunRead>();
    getRun.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    ensureWakeRun("w1", "c1", "t1");
    refreshOpenWakeRun("c1", "t1");
    second.resolve(run({ wake_count: 5, outcome: "completed" }));
    await flush();
    first.resolve(run({ wake_count: 1 }));
    await flush();
    expect(getWakeRun(KEY)?.run?.wake_count).toBe(5);
  });

  it("notifies subscribers and stops after unsubscribe", async () => {
    getRun.mockResolvedValue(run());
    const listener = vi.fn();
    const off = subscribeWakeRuns(listener);
    ensureWakeRun("w1", "c1", "t1");
    await flush();
    expect(listener).toHaveBeenCalledTimes(2);
    off();
    resetWakeRunStore();
    expect(listener).toHaveBeenCalledTimes(2);
  });
});
