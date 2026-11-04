import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { TaskNavigationCycleDeadlineError } from "@/lib/state/task-navigation-recovery-clock";
import {
  SIDEBAR_RECOVERY_MAX_ATTEMPTS,
  startSidebarPageRecoveryCycle,
} from "./sidebar-task-page-recovery";

function temporaryFailure(retryAfterSeconds?: number) {
  return new ApiError("busy", 503, { code: "persistence_unavailable" }, retryAfterSeconds);
}

function setVisibility(value: "visible" | "hidden") {
  Object.defineProperty(document, "visibilityState", { configurable: true, value });
  document.dispatchEvent(new Event("visibilitychange"));
}

afterEach(() => {
  vi.useRealTimers();
  setVisibility("visible");
});

describe("sidebar page recovery cycle", () => {
  it("obeys the shared delays and recovers within the outage", async () => {
    vi.useFakeTimers();
    const attempt = vi
      .fn()
      .mockRejectedValueOnce(temporaryFailure())
      .mockRejectedValueOnce(temporaryFailure())
      .mockRejectedValueOnce(temporaryFailure())
      .mockRejectedValueOnce(temporaryFailure())
      .mockResolvedValueOnce("recovered");
    const cycle = startSidebarPageRecoveryCycle({ attempt });
    const result = cycle.promise;

    await vi.advanceTimersByTimeAsync(0);
    expect(attempt).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(2_000);
    expect(attempt).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(5_000);
    expect(attempt).toHaveBeenCalledTimes(3);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(attempt).toHaveBeenCalledTimes(4);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(attempt).toHaveBeenCalledTimes(5);
    await expect(result).resolves.toBe("recovered");
  });
});

describe("sidebar page recovery exhaustion", () => {
  it("makes at most five attempts and rejects the last temporary error", async () => {
    vi.useFakeTimers();
    const failure = temporaryFailure();
    const attempt = vi.fn().mockRejectedValue(failure);
    const cycle = startSidebarPageRecoveryCycle({ attempt });
    const result = cycle.promise.catch((error: unknown) => error);

    await vi.advanceTimersByTimeAsync(120_000);
    await expect(result).resolves.toBe(failure);
    expect(attempt).toHaveBeenCalledTimes(SIDEBAR_RECOVERY_MAX_ATTEMPTS);
  });
});

describe("sidebar page recovery bounds", () => {
  it("stops without another attempt when the visible budget is exhausted", async () => {
    vi.useFakeTimers();
    const signals: AbortSignal[] = [];
    const attempt = vi.fn((signal: AbortSignal) => {
      signals.push(signal);
      return new Promise<string>(() => {});
    });
    const cycle = startSidebarPageRecoveryCycle({ attempt });
    const result = cycle.promise.catch((error: unknown) => error);

    await vi.advanceTimersByTimeAsync(29_999);
    expect(attempt).toHaveBeenCalledTimes(3);
    await vi.advanceTimersByTimeAsync(1);
    await expect(result).resolves.toBeInstanceOf(TaskNavigationCycleDeadlineError);
    expect(attempt).toHaveBeenCalledTimes(3);
    expect(signals.every((signal) => signal.aborted)).toBe(true);
    await vi.advanceTimersByTimeAsync(120_000);
    expect(attempt).toHaveBeenCalledTimes(3);
  });

  it("fails each attempt at its ten-second deadline and keeps the cycle temporary", async () => {
    vi.useFakeTimers();
    const attempt = vi.fn((signal: AbortSignal) => {
      expect(signal.aborted).toBe(false);
      return new Promise<string>(() => {});
    });
    const cycle = startSidebarPageRecoveryCycle({ attempt });
    const result = cycle.promise.catch((error: unknown) => error);

    await vi.advanceTimersByTimeAsync(9_999);
    expect(attempt).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(2_001);
    expect(attempt).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(18_000);
    await expect(result).resolves.toBeInstanceOf(TaskNavigationCycleDeadlineError);
    expect(attempt).toHaveBeenCalledTimes(3);
  });
});

describe("sidebar page recovery retry timing", () => {
  it("honors a server retry delay without extending the cycle", async () => {
    vi.useFakeTimers();
    const attempt = vi
      .fn()
      .mockRejectedValueOnce(temporaryFailure(12))
      .mockRejectedValueOnce(temporaryFailure())
      .mockRejectedValueOnce(temporaryFailure())
      .mockRejectedValueOnce(temporaryFailure())
      .mockRejectedValueOnce(temporaryFailure());
    const cycle = startSidebarPageRecoveryCycle({ attempt });
    const result = cycle.promise.catch((error: unknown) => error);

    await vi.advanceTimersByTimeAsync(0);
    expect(attempt).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(11_999);
    expect(attempt).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(attempt).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(18_000);
    await expect(result).resolves.toBeInstanceOf(TaskNavigationCycleDeadlineError);
    expect(attempt).toHaveBeenCalledTimes(4);
  });

  it("pauses the retry delay and budget while the tab is hidden", async () => {
    vi.useFakeTimers();
    setVisibility("visible");
    const attempt = vi
      .fn()
      .mockRejectedValueOnce(temporaryFailure())
      .mockResolvedValueOnce("recovered");
    const cycle = startSidebarPageRecoveryCycle({ attempt });
    const result = cycle.promise;

    await vi.advanceTimersByTimeAsync(0);
    expect(attempt).toHaveBeenCalledTimes(1);
    setVisibility("hidden");
    await vi.advanceTimersByTimeAsync(20_000);
    expect(attempt).toHaveBeenCalledTimes(1);
    setVisibility("visible");
    await vi.advanceTimersByTimeAsync(1_999);
    expect(attempt).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(attempt).toHaveBeenCalledTimes(2);
    await expect(result).resolves.toBe("recovered");
  });

  it("does not retry permanent failures", async () => {
    vi.useFakeTimers();
    const permanent = new ApiError("refused", 403, { error: "denied" });
    const attempt = vi.fn().mockRejectedValue(permanent);
    const cycle = startSidebarPageRecoveryCycle({ attempt });
    const result = cycle.promise.catch((error: unknown) => error);

    await vi.advanceTimersByTimeAsync(0);
    await expect(result).resolves.toBe(permanent);
    expect(attempt).toHaveBeenCalledTimes(1);
  });

  it("cancels the pending attempt and schedules nothing else", async () => {
    vi.useFakeTimers();
    const signals: AbortSignal[] = [];
    const attempt = vi.fn((signal: AbortSignal) => {
      signals.push(signal);
      return new Promise<string>((_resolve, reject) => {
        signal.addEventListener("abort", () =>
          reject(Object.assign(new Error("aborted"), { name: "AbortError" })),
        );
      });
    });
    const cycle = startSidebarPageRecoveryCycle({ attempt });
    const result = cycle.promise.catch((error: unknown) => error);

    await vi.advanceTimersByTimeAsync(0);
    cycle.abort();
    await expect(result).resolves.toMatchObject({ name: "AbortError" });
    await vi.advanceTimersByTimeAsync(120_000);
    expect(attempt).toHaveBeenCalledTimes(1);
    expect(signals[0]?.aborted).toBe(true);
  });
});
