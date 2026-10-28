import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { TaskNavigationCycleDeadlineError } from "./task-navigation-recovery-clock";
import {
  createTaskNavigationReads,
  isTemporaryTaskNavigationError,
  type TaskNavigationIdentity,
} from "./task-navigation-reads";
import { identity, temporaryNetworkFailure } from "./task-navigation-reads.test-helpers";

const apiMocks = vi.hoisted(() => ({
  fetchTask: vi.fn(),
  listTaskSessions: vi.fn(),
}));

vi.mock("@/lib/api", () => apiMocks);

afterEach(() => {
  vi.useRealTimers();
});

describe("task navigation retry timing", () => {
  // @covers AC-PLATFORM-INTERACTIVE-READS-005.2
  it("shares four bounded automatic retries at two, five, ten, and ten seconds", async () => {
    vi.useFakeTimers();
    const failure = temporaryNetworkFailure;
    const load = vi
      .fn<(taskId: string) => Promise<TaskNavigationIdentity>>()
      .mockImplementationOnce(failure)
      .mockImplementationOnce(failure)
      .mockImplementationOnce(failure)
      .mockImplementationOnce(failure)
      .mockResolvedValueOnce(identity("task-1"));
    const reads = createTaskNavigationReads(load);
    const generation = reads.beginNavigation({}, "/t/task-1");
    const first = reads.read("task-1", generation);
    const foreground = reads.read("task-1", generation, { refresh: true });

    expect(foreground).toBe(first);
    await vi.advanceTimersByTimeAsync(0);
    expect(load).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1_999);
    expect(load).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(load).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(4_999);
    expect(load).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(1);
    expect(load).toHaveBeenCalledTimes(3);
    await vi.advanceTimersByTimeAsync(9_999);
    expect(load).toHaveBeenCalledTimes(3);
    await vi.advanceTimersByTimeAsync(1);
    expect(load).toHaveBeenCalledTimes(4);
    await vi.advanceTimersByTimeAsync(9_999);
    expect(load).toHaveBeenCalledTimes(4);
    await vi.advanceTimersByTimeAsync(1);
    await expect(first).resolves.toMatchObject({ task: { id: "task-1" } });
    expect(load).toHaveBeenCalledTimes(5);
  });

  // @covers AC-PLATFORM-INTERACTIVE-READS-005.8
  it("recovers automatically after a thirteen-second persistence outage", async () => {
    vi.useFakeTimers();
    const startedAt = Date.now();
    const load = vi.fn((taskId: string) => {
      if (Date.now() - startedAt < 13_000) return temporaryNetworkFailure();
      return Promise.resolve(identity(taskId));
    });
    const reads = createTaskNavigationReads(load);
    const generation = reads.beginNavigation({}, "/t/task-1");
    const result = reads.read("task-1", generation);

    await vi.advanceTimersByTimeAsync(13_000);
    expect(load).toHaveBeenCalledTimes(3);
    expect(reads.getSnapshot("task-1", generation)).toMatchObject({
      phase: "retrying",
      temporary: true,
    });
    await vi.advanceTimersByTimeAsync(4_000);

    await expect(result).resolves.toMatchObject({ task: { id: "task-1" } });
    expect(load).toHaveBeenCalledTimes(4);
    expect(reads.getSnapshot("task-1", generation)).toMatchObject({
      phase: "succeeded",
      temporary: false,
    });
  });

  // @covers AC-PLATFORM-INTERACTIVE-READS-005.8
  it("recovers at the fifteen-second boundary without another user event", async () => {
    vi.useFakeTimers();
    const startedAt = Date.now();
    const load = vi.fn((taskId: string) => {
      if (Date.now() - startedAt < 15_000) return temporaryNetworkFailure();
      return Promise.resolve(identity(taskId));
    });
    const reads = createTaskNavigationReads(load);
    const generation = reads.beginNavigation({}, "/t/task-1");
    const result = reads.read("task-1", generation);

    await vi.advanceTimersByTimeAsync(15_000);
    expect(load).toHaveBeenCalledTimes(3);
    await vi.advanceTimersByTimeAsync(2_000);

    await expect(result).resolves.toMatchObject({ task: { id: "task-1" } });
    expect(load).toHaveBeenCalledTimes(4);
  });
});

describe("task navigation retry boundaries", () => {
  // @covers AC-PLATFORM-INTERACTIVE-READS-005.7
  it("preserves the most recent temporary error during a pending retry", async () => {
    vi.useFakeTimers();
    const error = new ApiError("busy", 503, { code: "persistence_unavailable" });
    const load = vi
      .fn<(taskId: string) => Promise<TaskNavigationIdentity>>()
      .mockRejectedValueOnce(error)
      .mockImplementationOnce(() => new Promise(() => {}));
    const reads = createTaskNavigationReads(load);
    const generation = reads.beginNavigation({}, "/t/task-1");
    const result = reads.read("task-1", generation);
    const outcome = result.catch((error: unknown) => error);

    await vi.advanceTimersByTimeAsync(0);
    expect(reads.getSnapshot("task-1", generation)).toMatchObject({
      phase: "retrying",
      error,
      temporary: true,
      attempt: 1,
    });
    await vi.advanceTimersByTimeAsync(2_000);
    expect(reads.getSnapshot("task-1", generation)).toMatchObject({
      phase: "retrying",
      error,
      temporary: true,
      attempt: 1,
    });
    reads.dispose();
    expect(await outcome).toMatchObject({ name: "AbortError" });
  });

  // @covers AC-PLATFORM-INTERACTIVE-READS-005.10
  it("stops within the budget when Retry-After exceeds the remaining time", async () => {
    vi.useFakeTimers();
    const load = vi
      .fn<(taskId: string) => Promise<TaskNavigationIdentity>>()
      .mockRejectedValue(new ApiError("busy", 503, { code: "persistence_unavailable" }, 60));
    const reads = createTaskNavigationReads(load);
    const generation = reads.beginNavigation({}, "/t/task-1");
    const result = reads.read("task-1", generation);
    const failure = result.catch((error: unknown) => error);

    await vi.advanceTimersByTimeAsync(29_999);
    expect(load).toHaveBeenCalledTimes(1);
    expect(reads.getSnapshot("task-1", generation).phase).toBe("retrying");
    await vi.advanceTimersByTimeAsync(1);

    expect(await failure).toBeInstanceOf(TaskNavigationCycleDeadlineError);
    expect(load).toHaveBeenCalledTimes(1);
    expect(reads.getSnapshot("task-1", generation)).toMatchObject({
      phase: "failed",
      temporary: true,
    });
  });
});

describe("task navigation retry exhaustion", () => {
  it("honors Retry-After and allows one foreground recovery after exhaustion", async () => {
    vi.useFakeTimers();
    const load = vi
      .fn<(taskId: string) => Promise<TaskNavigationIdentity>>()
      .mockRejectedValueOnce(new ApiError("busy", 503, { code: "persistence_unavailable" }, 8))
      .mockRejectedValueOnce(new ApiError("busy", 503, { code: "persistence_unavailable" }))
      .mockRejectedValueOnce(new ApiError("busy", 503, { code: "persistence_unavailable" }))
      .mockRejectedValueOnce(new ApiError("busy", 503, { code: "persistence_unavailable" }))
      .mockResolvedValueOnce(identity("task-1"));
    const reads = createTaskNavigationReads(load);
    const generation = reads.beginNavigation({}, "/t/task-1");
    const initial = reads.read("task-1", generation);
    const failed = expect(initial).rejects.toBeInstanceOf(TaskNavigationCycleDeadlineError);

    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(7_999);
    expect(load).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    await vi.advanceTimersByTimeAsync(5_000);
    await vi.advanceTimersByTimeAsync(10_000);
    await vi.advanceTimersByTimeAsync(7_000);
    await failed;
    expect(load).toHaveBeenCalledTimes(4);

    await expect(reads.read("task-1", generation, { refresh: true })).rejects.toBeInstanceOf(
      TaskNavigationCycleDeadlineError,
    );
    expect(load).toHaveBeenCalledTimes(4);

    const foregroundEpisode = {};
    const foreground = reads.read("task-1", generation, {
      refresh: true,
      foregroundRecoveryEpisode: foregroundEpisode,
    });
    const joinedForeground = reads.read("task-1", generation, {
      refresh: true,
      foregroundRecoveryEpisode: foregroundEpisode,
    });
    expect(joinedForeground).toBe(foreground);
    await vi.advanceTimersByTimeAsync(0);
    await expect(foreground).resolves.toMatchObject({
      task: { id: "task-1" },
    });
    expect(load).toHaveBeenCalledTimes(5);
    await expect(
      reads.read("task-1", generation, {
        refresh: true,
        foregroundRecoveryEpisode: foregroundEpisode,
      }),
    ).resolves.toMatchObject({ task: { id: "task-1" } });
    expect(load).toHaveBeenCalledTimes(5);
  });

  it("does not restart exhausted permanent failures during foreground recovery", async () => {
    const load = vi.fn().mockRejectedValue(new ApiError("unauthorized", 401, null));
    const reads = createTaskNavigationReads(load);
    const generation = reads.beginNavigation({}, "/t/task-1");
    await expect(reads.read("task-1", generation)).rejects.toMatchObject({ status: 401 });

    await expect(
      reads.read("task-1", generation, {
        refresh: true,
        foregroundRecoveryEpisode: {},
      }),
    ).rejects.toMatchObject({ status: 401 });
    expect(load).toHaveBeenCalledTimes(1);
  });
});

describe("task navigation retry policy", () => {
  it("does not retry permanent, parse, authentication, or abort failures", async () => {
    expect(isTemporaryTaskNavigationError(new TypeError("offline"))).toBe(true);
    expect(isTemporaryTaskNavigationError(new TaskNavigationCycleDeadlineError())).toBe(true);
    expect(isTemporaryTaskNavigationError(new ApiError("busy", 429, null))).toBe(true);
    expect(isTemporaryTaskNavigationError(new ApiError("busy", 502, null))).toBe(true);
    expect(isTemporaryTaskNavigationError(new ApiError("busy", 504, null))).toBe(true);
    expect(
      isTemporaryTaskNavigationError(
        new ApiError("busy", 500, { code: "persistence_unavailable" }),
      ),
    ).toBe(true);
    expect(
      isTemporaryTaskNavigationError(
        new ApiError("busy", 503, { code: "persistence_unavailable" }),
      ),
    ).toBe(true);
    for (const error of [
      new ApiError("missing", 404, null),
      new ApiError("unauthorized", 401, null),
      new ApiError("other server failure", 500, { error_code: "persistence_unavailable" }),
      new SyntaxError("bad JSON"),
      Object.assign(new Error("cancelled"), { name: "AbortError" }),
    ]) {
      expect(isTemporaryTaskNavigationError(error)).toBe(false);
    }

    const load = vi.fn().mockRejectedValue(new ApiError("missing", 404, null));
    const reads = createTaskNavigationReads(load);
    const generation = reads.beginNavigation({}, "/t/task-1");
    await expect(reads.read("task-1", generation)).rejects.toMatchObject({ status: 404 });
    expect(load).toHaveBeenCalledTimes(1);
  });

  it("suspends a scheduled retry while the document is hidden", async () => {
    vi.useFakeTimers();
    const visibility = Object.getOwnPropertyDescriptor(document, "visibilityState");
    try {
      Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
      const load = vi
        .fn<(taskId: string) => Promise<TaskNavigationIdentity>>()
        .mockRejectedValueOnce(new TypeError("offline"))
        .mockResolvedValueOnce(identity("task-1"));
      const reads = createTaskNavigationReads(load);
      const generation = reads.beginNavigation({}, "/t/task-1");
      const result = reads.read("task-1", generation);
      await vi.advanceTimersByTimeAsync(0);
      await vi.advanceTimersByTimeAsync(1_000);
      Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
      document.dispatchEvent(new Event("visibilitychange"));
      await vi.advanceTimersByTimeAsync(5_000);
      expect(load).toHaveBeenCalledTimes(1);
      Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
      document.dispatchEvent(new Event("visibilitychange"));
      await vi.advanceTimersByTimeAsync(999);
      expect(load).toHaveBeenCalledTimes(1);
      await vi.advanceTimersByTimeAsync(1);
      await expect(result).resolves.toMatchObject({ task: { id: "task-1" } });
    } finally {
      Object.defineProperty(
        document,
        "visibilityState",
        visibility ?? { configurable: true, value: "visible" },
      );
    }
  });
});

describe("task navigation attempt deadline", () => {
  // @covers AC-PLATFORM-INTERACTIVE-READS-005.2
  it("stops a request that never settles at the thirty-second cycle deadline", async () => {
    vi.useFakeTimers();
    const requestSignals: AbortSignal[] = [];
    const load = vi.fn((_taskId: string, signal: AbortSignal) => {
      requestSignals.push(signal);
      return new Promise<TaskNavigationIdentity>((_resolve, reject) => {
        signal.addEventListener(
          "abort",
          () => reject(Object.assign(new Error("request aborted"), { name: "AbortError" })),
          { once: true },
        );
      });
    });
    const reads = createTaskNavigationReads(load);
    const generation = reads.beginNavigation({}, "/t/task-1");
    const result = reads.read("task-1", generation);
    const outcome = result.catch((error: unknown) => error);

    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(requestSignals[0]?.aborted).toBe(true);
    expect(reads.getSnapshot("task-1", generation).phase).toBe("retrying");

    await vi.advanceTimersByTimeAsync(12_000);
    expect(load).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(5_000);
    expect(load).toHaveBeenCalledTimes(3);
    await vi.advanceTimersByTimeAsync(3_000);

    expect(await outcome).toBeInstanceOf(TaskNavigationCycleDeadlineError);
    expect(requestSignals.every((signal) => signal.aborted)).toBe(true);
    expect(reads.getSnapshot("task-1", generation)).toMatchObject({
      phase: "failed",
      temporary: true,
      attempt: 2,
    });
  });
});
