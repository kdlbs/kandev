import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { createAppStore } from "@/lib/state/store";
import { TaskNavigationCycleDeadlineError } from "./task-navigation-recovery-clock";
import {
  beginTaskNavigation,
  createTaskNavigationReads,
  readTaskNavigationIdentity,
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

describe("task navigation attempt cancellation", () => {
  it("aborts each failed attempt's pending session request before retrying", async () => {
    vi.useFakeTimers();
    const sessionSignals: AbortSignal[] = [];
    apiMocks.fetchTask.mockImplementation((_id: string, options: { init?: RequestInit }) => {
      const priorSignal = sessionSignals.at(-1);
      if (priorSignal) expect(priorSignal.aborted).toBe(true);
      expect(options.init?.signal?.aborted).toBe(false);
      return Promise.reject(new ApiError("busy", 503, { code: "persistence_unavailable" }));
    });
    apiMocks.listTaskSessions.mockImplementation((_id: string, options: { init?: RequestInit }) => {
      const signal = options.init?.signal as AbortSignal;
      sessionSignals.push(signal);
      return new Promise((_, reject) => {
        signal.addEventListener("abort", () =>
          reject(Object.assign(new Error("aborted"), { name: "AbortError" })),
        );
      });
    });
    const store = createAppStore();
    const generation = beginTaskNavigation(store, {}, "/t/task-a").generation;
    const result = readTaskNavigationIdentity(store, "task-a", { generation });
    const failure = result.catch((error: unknown) => error);

    await vi.advanceTimersByTimeAsync(0);
    expect(sessionSignals[0]?.aborted).toBe(true);
    await vi.advanceTimersByTimeAsync(2_000);
    expect(apiMocks.fetchTask).toHaveBeenCalledTimes(2);
    expect(sessionSignals[1]?.aborted).toBe(true);
    await vi.advanceTimersByTimeAsync(5_000);
    await vi.advanceTimersByTimeAsync(10_000);
    await vi.advanceTimersByTimeAsync(10_000);
    await expect(failure).resolves.toMatchObject({ status: 503 });

    expect(apiMocks.fetchTask).toHaveBeenCalledTimes(5);
    expect(sessionSignals).toHaveLength(5);
    expect(sessionSignals.every((signal) => signal.aborted)).toBe(true);
  });
});

describe("task navigation attempt cancellation fallback", () => {
  it("does not downgrade an aborted session request to the empty-session fallback", async () => {
    let taskRequestSignal: AbortSignal | undefined;
    let sessionRequestSignal: AbortSignal | undefined;
    apiMocks.fetchTask.mockImplementation((_id: string, options: { init?: RequestInit }) => {
      taskRequestSignal = options.init?.signal ?? undefined;
      return new Promise((_, reject) => {
        taskRequestSignal?.addEventListener("abort", () =>
          reject(Object.assign(new Error("aborted"), { name: "AbortError" })),
        );
      });
    });
    apiMocks.listTaskSessions.mockImplementation((_id: string, options: { init?: RequestInit }) => {
      sessionRequestSignal = options.init?.signal ?? undefined;
      return new Promise((_, reject) => {
        sessionRequestSignal?.addEventListener("abort", () =>
          reject(Object.assign(new Error("aborted"), { name: "AbortError" })),
        );
      });
    });
    const store = createAppStore();
    const owner = {};
    const context = beginTaskNavigation(store, owner, "/t/task-a");
    const result = readTaskNavigationIdentity(store, "task-a", { context });
    const outcome = result.then(
      () => "resolved",
      (error) => error,
    );
    beginTaskNavigation(store, owner, "/t/task-b");

    await Promise.resolve();
    expect(taskRequestSignal?.aborted).toBe(true);
    expect(sessionRequestSignal).toBeDefined();
    expect(sessionRequestSignal).not.toBe(taskRequestSignal);
    await expect(outcome).resolves.toMatchObject({ name: "AbortError" });
    await Promise.resolve();
    expect(sessionRequestSignal?.aborted).toBe(true);
  });
});

describe("task navigation visible-time budget", () => {
  // @covers AC-PLATFORM-INTERACTIVE-READS-005.9
  it("keeps the cycle budget and retry delays paused while the document is hidden", async () => {
    vi.useFakeTimers();
    const visibility = Object.getOwnPropertyDescriptor(document, "visibilityState");
    try {
      Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
      const load = vi
        .fn<(taskId: string) => Promise<TaskNavigationIdentity>>()
        .mockImplementationOnce(temporaryNetworkFailure)
        .mockImplementationOnce(temporaryNetworkFailure)
        .mockImplementationOnce(temporaryNetworkFailure)
        .mockImplementationOnce(temporaryNetworkFailure)
        .mockResolvedValueOnce(identity("task-1"));
      const reads = createTaskNavigationReads(load);
      const generation = reads.beginNavigation({}, "/t/task-1");
      const result = reads.read("task-1", generation);
      await vi.advanceTimersByTimeAsync(0);
      expect(load).toHaveBeenCalledTimes(1);

      Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
      document.dispatchEvent(new Event("visibilitychange"));
      await vi.advanceTimersByTimeAsync(10 * 60_000);
      expect(load).toHaveBeenCalledTimes(1);

      Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
      document.dispatchEvent(new Event("visibilitychange"));
      await vi.advanceTimersByTimeAsync(2_000);
      expect(load).toHaveBeenCalledTimes(2);
      await vi.advanceTimersByTimeAsync(5_000);
      expect(load).toHaveBeenCalledTimes(3);
      await vi.advanceTimersByTimeAsync(10_000);
      expect(load).toHaveBeenCalledTimes(4);
      await vi.advanceTimersByTimeAsync(10_000);

      await expect(result).resolves.toMatchObject({ task: { id: "task-1" } });
      expect(load).toHaveBeenCalledTimes(5);
    } finally {
      Object.defineProperty(
        document,
        "visibilityState",
        visibility ?? { configurable: true, value: "visible" },
      );
    }
  });
});

describe("task navigation delayed retry accounting", () => {
  // @covers AC-PLATFORM-INTERACTIVE-READS-005.9
  it("keeps a later retry delay whole when the document hides after earlier visible time", async () => {
    vi.useFakeTimers();
    const visibility = Object.getOwnPropertyDescriptor(document, "visibilityState");
    try {
      Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
      let calls = 0;
      const load = vi.fn((taskId: string) => {
        calls++;
        if (calls === 1) return temporaryNetworkFailure();
        if (calls === 2) {
          return new Promise<TaskNavigationIdentity>((_resolve, reject) => {
            setTimeout(() => reject(new TypeError("network unavailable")), 2_000);
          });
        }
        return Promise.resolve(identity(taskId));
      });
      const reads = createTaskNavigationReads(load);
      const generation = reads.beginNavigation({}, "/t/task-1");
      const result = reads.read("task-1", generation);

      await vi.advanceTimersByTimeAsync(0);
      await vi.advanceTimersByTimeAsync(2_000);
      await vi.advanceTimersByTimeAsync(2_000);
      await vi.advanceTimersByTimeAsync(1_000);
      Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
      document.dispatchEvent(new Event("visibilitychange"));
      await vi.advanceTimersByTimeAsync(60_000);
      Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
      document.dispatchEvent(new Event("visibilitychange"));
      await vi.advanceTimersByTimeAsync(3_999);
      expect(load).toHaveBeenCalledTimes(2);

      await vi.advanceTimersByTimeAsync(1);
      await expect(result).resolves.toMatchObject({ task: { id: "task-1" } });
      expect(load).toHaveBeenCalledTimes(3);
    } finally {
      Object.defineProperty(
        document,
        "visibilityState",
        visibility ?? { configurable: true, value: "visible" },
      );
    }
  });
});

describe("task navigation cycle deadline", () => {
  // @covers AC-PLATFORM-INTERACTIVE-READS-005.10
  it("aborts an in-flight attempt at the deadline and rejects a late success", async () => {
    vi.useFakeTimers();
    let resolveRead: ((value: TaskNavigationIdentity) => void) | undefined;
    const load = vi.fn(
      (_taskId: string, signal: AbortSignal) =>
        new Promise<TaskNavigationIdentity>((resolve, reject) => {
          resolveRead = resolve;
          signal.addEventListener(
            "abort",
            () => reject(Object.assign(new Error("aborted"), { name: "AbortError" })),
            { once: true },
          );
        }),
    );
    const reads = createTaskNavigationReads(load);
    const generation = reads.beginNavigation({}, "/t/task-1");
    const result = reads.read("task-1", generation);
    const outcome = result.catch((error: unknown) => error);

    await vi.advanceTimersByTimeAsync(30_000);
    expect(await outcome).toBeInstanceOf(TaskNavigationCycleDeadlineError);
    expect(reads.getSnapshot("task-1", generation)).toMatchObject({
      phase: "failed",
      temporary: true,
    });

    resolveRead?.(identity("task-1"));
    await vi.advanceTimersByTimeAsync(0);
    expect(reads.getSnapshot("task-1", generation)).toMatchObject({
      phase: "failed",
      temporary: true,
    });
    expect(load).toHaveBeenCalledTimes(3);
  });

  it("starts one new bounded cycle on manual recovery after exhaustion", async () => {
    vi.useFakeTimers();
    const load = vi
      .fn<(taskId: string) => Promise<TaskNavigationIdentity>>()
      .mockImplementationOnce(temporaryNetworkFailure)
      .mockImplementationOnce(temporaryNetworkFailure)
      .mockImplementationOnce(temporaryNetworkFailure)
      .mockImplementationOnce(temporaryNetworkFailure)
      .mockImplementationOnce(temporaryNetworkFailure)
      .mockResolvedValueOnce(identity("task-1"));
    const reads = createTaskNavigationReads(load);
    const generation = reads.beginNavigation({}, "/t/task-1");
    const first = reads.read("task-1", generation);
    const failure = first.catch((error: unknown) => error);

    await vi.advanceTimersByTimeAsync(27_000);
    expect(await failure).toBeInstanceOf(TypeError);
    expect(load).toHaveBeenCalledTimes(5);

    const retry = reads.read("task-1", generation, { refresh: true, manualRetry: true });
    await vi.advanceTimersByTimeAsync(0);
    await expect(retry).resolves.toMatchObject({ task: { id: "task-1" } });
    expect(load).toHaveBeenCalledTimes(6);
  });

  it("stops automatic retries after navigation replaces the read scope", async () => {
    vi.useFakeTimers();
    const load = vi
      .fn<(taskId: string) => Promise<TaskNavigationIdentity>>()
      .mockImplementationOnce(temporaryNetworkFailure);
    const reads = createTaskNavigationReads(load);
    const owner = {};
    const oldGeneration = reads.beginNavigation(owner, "/t/task-1");
    const oldRead = reads.read("task-1", oldGeneration);
    const outcome = oldRead.catch((error: unknown) => error);
    await vi.advanceTimersByTimeAsync(0);
    expect(load).toHaveBeenCalledTimes(1);

    reads.beginNavigation(owner, "/t/task-2");
    await vi.advanceTimersByTimeAsync(60_000);

    expect(await outcome).toMatchObject({ name: "AbortError" });
    expect(load).toHaveBeenCalledTimes(1);
  });
});
