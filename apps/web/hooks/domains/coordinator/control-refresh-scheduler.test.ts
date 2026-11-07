import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  createControlRefreshScheduler,
  type ControlRefreshOutcome,
} from "./control-refresh-scheduler";

type Pending = { resolve: (outcome: ControlRefreshOutcome) => void; signal: AbortSignal };

function harness(options: { isVisible?: () => boolean } = {}) {
  const calls: Pending[] = [];
  const retries: Array<number | null> = [];
  const scheduler = createControlRefreshScheduler({
    run: (signal) =>
      new Promise<ControlRefreshOutcome>((resolve) => calls.push({ resolve, signal })),
    onRetryScheduled: (at) => retries.push(at),
    ...options,
  });
  return { calls, retries, scheduler };
}

async function settle(call: Pending, outcome: ControlRefreshOutcome = "ok") {
  call.resolve(outcome);
  await vi.advanceTimersByTimeAsync(0);
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(Date.parse("2026-10-20T09:00:00Z"));
});
afterEach(() => vi.useRealTimers());

describe("control refresh scheduler", () => {
  it("never aborts a read in flight and merges every request during it into one follow-up", async () => {
    const { calls, scheduler } = harness();
    const first = scheduler.request();
    const second = scheduler.request();
    const third = scheduler.request();
    expect(calls).toHaveLength(1);
    await settle(calls[0]);
    await first;
    expect(calls[0].signal.aborted).toBe(false);
    expect(calls).toHaveLength(2);
    await settle(calls[1]);
    await Promise.all([second, third]);
    expect(calls).toHaveLength(2);
    scheduler.dispose();
  });

  it("settles a burst of live events into one read after two quiet seconds", async () => {
    const { calls, scheduler } = harness();
    for (let index = 0; index < 5; index += 1) {
      scheduler.event();
      await vi.advanceTimersByTimeAsync(500);
    }
    expect(calls).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(1_500);
    expect(calls).toHaveLength(1);
    scheduler.dispose();
  });

  it("reads at most once per ten seconds while events never stop", async () => {
    const { calls, scheduler } = harness();
    for (let index = 0; index < 30; index += 1) {
      scheduler.event();
      await vi.advanceTimersByTimeAsync(1_000);
      if (calls.length > 0 && calls.at(-1)) await settle(calls.at(-1)!);
    }
    // 30 seconds of one event per second: reads at 10 s, 20 s and 30 s.
    expect(calls.length).toBeLessThanOrEqual(3);
    expect(calls.length).toBeGreaterThanOrEqual(2);
    scheduler.dispose();
  });
});

describe("control refresh scheduler timing", () => {
  it("re-reads every thirty seconds and restarts that clock after any read", async () => {
    const { calls, scheduler } = harness();
    void scheduler.request();
    await settle(calls[0]);
    await vi.advanceTimersByTimeAsync(29_999);
    expect(calls).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(calls).toHaveLength(2);
    scheduler.dispose();
  });

  it("skips a periodic read while the page is hidden", async () => {
    let visible = false;
    const { calls, scheduler } = harness({ isVisible: () => visible });
    void scheduler.request();
    await settle(calls[0]);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(calls).toHaveLength(1);
    visible = true;
    void scheduler.request();
    expect(calls).toHaveLength(2);
    scheduler.dispose();
  });

  it("does not read for live events while hidden; the foreground return reads once", async () => {
    let visible = false;
    const { calls, scheduler } = harness({ isVisible: () => visible });
    for (let index = 0; index < 5; index += 1) {
      scheduler.event();
      await vi.advanceTimersByTimeAsync(5_000);
    }
    expect(calls).toHaveLength(0);
    visible = true;
    void scheduler.request();
    void scheduler.request();
    expect(calls).toHaveLength(1);
    scheduler.dispose();
  });

  it("clears the retry time when a failed read is followed by a queued read", async () => {
    const { calls, retries, scheduler } = harness();
    void scheduler.request();
    void scheduler.request();
    await settle(calls[0], "retry");
    expect(calls).toHaveLength(2);
    expect(retries).toEqual([null]);
    scheduler.dispose();
  });

  it("backs off 5, 10, 20, 40 then 60 seconds after failures and resets on success", async () => {
    const { calls, retries, scheduler } = harness();
    void scheduler.request();
    const delays: number[] = [];
    for (const expected of [5_000, 10_000, 20_000, 40_000, 60_000, 60_000]) {
      const before = Date.now();
      await settle(calls.at(-1)!, "retry");
      delays.push((retries.at(-1) ?? 0) - before);
      await vi.advanceTimersByTimeAsync(expected);
    }
    expect(delays).toEqual([5_000, 10_000, 20_000, 40_000, 60_000, 60_000]);
    await settle(calls.at(-1)!, "ok");
    expect(retries.at(-1)).toBeNull();
    scheduler.dispose();
  });

  it("aborts the read in flight and stops scheduling when disposed", async () => {
    const { calls, scheduler } = harness();
    void scheduler.request();
    scheduler.event();
    scheduler.dispose();
    expect(calls[0].signal.aborted).toBe(true);
    await vi.advanceTimersByTimeAsync(120_000);
    expect(calls).toHaveLength(1);
  });
});
