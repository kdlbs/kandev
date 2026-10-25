import { act, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useExecutionClock } from "./use-execution-clock";

afterEach(() => vi.useRealTimers());

it("shares one expiry clock and releases it when the last visible consumer leaves", () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-24T00:00:00Z"));
  const before = vi.getTimerCount();
  const start = Date.now();
  const first = renderHook(() => useExecutionClock(start + 750));
  const second = renderHook(() => useExecutionClock(start + 1500));
  expect(first.result.current).toBe(Date.now());
  expect(second.result.current).toBe(first.result.current);
  expect(vi.getTimerCount()).toBe(before + 1);
  act(() => vi.advanceTimersByTime(750));
  expect(first.result.current).toBe(Date.now());
  expect(vi.getTimerCount()).toBe(before + 1);
  first.unmount();
  act(() => vi.advanceTimersByTime(750));
  expect(second.result.current).toBe(Date.now());
  expect(vi.getTimerCount()).toBe(before);
  second.unmount();
  expect(vi.getTimerCount()).toBe(before);
  act(() => vi.advanceTimersByTime(60_000));
  const resumed = renderHook(() => useExecutionClock(Date.now() + 1000));
  expect(resumed.result.current).toBe(Date.now());
  resumed.unmount();
  expect(vi.getTimerCount()).toBe(before);
});

it("does not run a timer for inactive surfaces", () => {
  vi.useFakeTimers();
  const before = vi.getTimerCount();
  const view = renderHook(({ deadline }) => useExecutionClock(deadline), {
    initialProps: { deadline: null as number | null },
  });
  expect(vi.getTimerCount()).toBe(before);
  view.rerender({ deadline: Date.now() + 1000 });
  expect(view.result.current).toBe(Date.now());
  expect(vi.getTimerCount()).toBe(before + 1);
  view.rerender({ deadline: null });
  expect(vi.getTimerCount()).toBe(before);
  view.unmount();
});
