import { afterEach, describe, expect, it, vi } from "vitest";
import {
  TASK_NAVIGATION_VISIBLE_BUDGET_MS,
  TaskNavigationCycleDeadlineError,
  TaskNavigationRecoveryClock,
} from "./task-navigation-recovery-clock";

afterEach(() => {
  vi.useRealTimers();
});

function abortableWait(signal: AbortSignal): Promise<never> {
  return new Promise((_, reject) => {
    signal.addEventListener(
      "abort",
      () => reject(Object.assign(new Error("aborted"), { name: "AbortError" })),
      { once: true },
    );
  });
}

describe("task navigation recovery clock", () => {
  it("waits for a visible retry delay and reports the remaining budget", async () => {
    vi.useFakeTimers();
    const controller = new AbortController();
    const onDeadline = vi.fn();
    const clock = new TaskNavigationRecoveryClock({ onDeadline });
    clock.start();
    const wait = clock.wait(2_000, controller.signal);

    await vi.advanceTimersByTimeAsync(1_999);
    expect(clock.expired).toBe(false);
    expect(clock.remainingVisibleMs()).toBe(TASK_NAVIGATION_VISIBLE_BUDGET_MS - 1_999);
    await vi.advanceTimersByTimeAsync(1);
    await expect(wait).resolves.toBeUndefined();
    expect(onDeadline).not.toHaveBeenCalled();
    clock.dispose();
  });

  it("pauses both the retry delay and the cycle budget while hidden", async () => {
    vi.useFakeTimers();
    let visible = true;
    let notify: () => void = () => {};
    const unsubscribe = vi.fn();
    const clock = new TaskNavigationRecoveryClock({
      onDeadline: vi.fn(),
      visibility: {
        isVisible: () => visible,
        subscribe: (listener) => {
          notify = listener;
          return unsubscribe;
        },
      },
    });
    const controller = new AbortController();
    clock.start();
    const wait = clock.wait(2_000, controller.signal);

    await vi.advanceTimersByTimeAsync(1_000);
    visible = false;
    notify();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(clock.remainingVisibleMs()).toBe(TASK_NAVIGATION_VISIBLE_BUDGET_MS - 1_000);

    visible = true;
    notify();
    await vi.advanceTimersByTimeAsync(999);
    expect(clock.remainingVisibleMs()).toBe(TASK_NAVIGATION_VISIBLE_BUDGET_MS - 1_999);
    await vi.advanceTimersByTimeAsync(1);
    await expect(wait).resolves.toBeUndefined();

    clock.dispose();
    expect(unsubscribe).toHaveBeenCalledOnce();
  });
});

describe("task navigation recovery clock deadlines", () => {
  it("expires at the visible-time budget and rejects a pending delay", async () => {
    vi.useFakeTimers();
    const onDeadline = vi.fn();
    const clock = new TaskNavigationRecoveryClock({ onDeadline });
    const controller = new AbortController();
    clock.start();
    const wait = clock.wait(60_000, controller.signal);
    const outcome = wait.catch((error: unknown) => error);

    await vi.advanceTimersByTimeAsync(TASK_NAVIGATION_VISIBLE_BUDGET_MS - 1);
    expect(clock.expired).toBe(false);
    await vi.advanceTimersByTimeAsync(1);

    expect(clock.expired).toBe(true);
    expect(clock.remainingVisibleMs()).toBe(0);
    expect(await outcome).toBeInstanceOf(TaskNavigationCycleDeadlineError);
    expect(onDeadline).toHaveBeenCalledOnce();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(onDeadline).toHaveBeenCalledOnce();
    clock.dispose();
  });

  it("rejects a pending delay when navigation disposes the clock", async () => {
    vi.useFakeTimers();
    const clock = new TaskNavigationRecoveryClock({ onDeadline: vi.fn() });
    const controller = new AbortController();
    clock.start();
    const wait = clock.wait(5_000, controller.signal);
    const outcome = wait.catch((error: unknown) => error);

    clock.dispose();

    expect(await outcome).toMatchObject({ name: "AbortError" });
    await vi.advanceTimersByTimeAsync(60_000);
    expect(clock.expired).toBe(false);
  });

  it("rejects promptly when the cycle is already exhausted", async () => {
    vi.useFakeTimers();
    const clock = new TaskNavigationRecoveryClock({ onDeadline: vi.fn() });
    const controller = new AbortController();
    clock.start();
    await vi.advanceTimersByTimeAsync(TASK_NAVIGATION_VISIBLE_BUDGET_MS);

    await expect(clock.wait(2_000, controller.signal)).rejects.toBeInstanceOf(
      TaskNavigationCycleDeadlineError,
    );
    clock.dispose();
  });

  it("keeps an in-flight read to its own deadline while the tab is hidden", async () => {
    vi.useFakeTimers();
    let visible = true;
    let notify: () => void = () => {};
    const clock = new TaskNavigationRecoveryClock({
      onDeadline: vi.fn(),
      visibility: {
        isVisible: () => visible,
        subscribe: (listener) => ((notify = listener), () => {}),
      },
    });
    clock.start();
    const controller = new AbortController();
    const read = abortableWait(controller.signal);
    const outcome = read.catch((error: unknown) => error);

    visible = false;
    notify();
    await vi.advanceTimersByTimeAsync(10_000);
    controller.abort();

    expect(await outcome).toMatchObject({ name: "AbortError" });
    expect(clock.expired).toBe(false);
    clock.dispose();
  });
});

function createVisibilityHarness() {
  let visible = true;
  let notify: () => void = () => {};
  return {
    visibility: {
      isVisible: () => visible,
      subscribe: (listener: () => void) => {
        notify = listener;
        return () => {};
      },
    },
    hide() {
      visible = false;
      notify();
    },
    show() {
      visible = true;
      notify();
    },
  };
}

describe("task navigation recovery clock delayed waits", () => {
  // @covers AC-PLATFORM-INTERACTIVE-READS-005.9
  it("keeps a later retry delay whole when the tab hides after earlier visible time", async () => {
    vi.useFakeTimers();
    const harness = createVisibilityHarness();
    const clock = new TaskNavigationRecoveryClock({
      onDeadline: vi.fn(),
      visibility: harness.visibility,
    });
    const controller = new AbortController();
    let resolved = false;
    clock.start();
    await vi.advanceTimersByTimeAsync(2_000);
    const wait = clock.wait(5_000, controller.signal).then(() => {
      resolved = true;
    });

    await vi.advanceTimersByTimeAsync(1_000);
    harness.hide();
    await vi.advanceTimersByTimeAsync(60_000);
    harness.show();
    await vi.advanceTimersByTimeAsync(3_999);
    await Promise.resolve();
    expect(resolved).toBe(false);

    await vi.advanceTimersByTimeAsync(1);
    await wait;
    expect(resolved).toBe(true);
    clock.dispose();
  });

  // @covers AC-PLATFORM-INTERACTIVE-READS-005.9
  it("starts a retry delay when the wait begins, not when the cycle began", async () => {
    vi.useFakeTimers();
    const harness = createVisibilityHarness();
    const clock = new TaskNavigationRecoveryClock({
      onDeadline: vi.fn(),
      visibility: harness.visibility,
    });
    const controller = new AbortController();
    let resolved = false;
    clock.start();
    await vi.advanceTimersByTimeAsync(6_000);
    const wait = clock.wait(2_000, controller.signal).then(() => {
      resolved = true;
    });

    await vi.advanceTimersByTimeAsync(1_000);
    harness.hide();
    await vi.advanceTimersByTimeAsync(60_000);
    harness.show();
    await vi.advanceTimersByTimeAsync(999);
    await Promise.resolve();
    expect(resolved).toBe(false);

    await vi.advanceTimersByTimeAsync(1);
    await wait;
    expect(resolved).toBe(true);
    clock.dispose();
  });

  // @covers AC-PLATFORM-INTERACTIVE-READS-005.10
  it("does not shorten a Retry-After wait that begins after earlier visible time", async () => {
    vi.useFakeTimers();
    const harness = createVisibilityHarness();
    const clock = new TaskNavigationRecoveryClock({
      onDeadline: vi.fn(),
      visibility: harness.visibility,
    });
    const controller = new AbortController();
    let resolved = false;
    clock.start();
    await vi.advanceTimersByTimeAsync(5_000);
    const wait = clock.wait(8_000, controller.signal).then(() => {
      resolved = true;
    });

    await vi.advanceTimersByTimeAsync(1_000);
    harness.hide();
    await vi.advanceTimersByTimeAsync(60_000);
    harness.show();
    await vi.advanceTimersByTimeAsync(6_999);
    await Promise.resolve();
    expect(resolved).toBe(false);

    await vi.advanceTimersByTimeAsync(1);
    await wait;
    expect(resolved).toBe(true);
    clock.dispose();
  });
});
