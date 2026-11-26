import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { usePulseOnSplitMove, useWipeNamesReveal } from "./use-wipe-names-reveal";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("useWipeNamesReveal", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.5
  it("shows the names while any source holds them and briefly after the last lets go", () => {
    const { result } = renderHook(() => useWipeNamesReveal());
    expect(result.current.shown).toBe(false);
    act(() => result.current.hold("handle", true));
    act(() => result.current.hold("slider", true));
    act(() => result.current.hold("handle", false));
    act(() => vi.advanceTimersByTime(5000));
    expect(result.current.shown).toBe(true);
    act(() => result.current.hold("slider", false));
    act(() => vi.advanceTimersByTime(1000));
    expect(result.current.shown).toBe(true);
    act(() => vi.advanceTimersByTime(300));
    expect(result.current.shown).toBe(false);
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.5
  it("shows the names briefly on a pulse, restarting the delay on each one", () => {
    const { result } = renderHook(() => useWipeNamesReveal());
    act(() => result.current.pulse());
    act(() => vi.advanceTimersByTime(1000));
    act(() => result.current.pulse());
    act(() => vi.advanceTimersByTime(1000));
    expect(result.current.shown).toBe(true);
    act(() => vi.advanceTimersByTime(300));
    expect(result.current.shown).toBe(false);
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.5
  it("clears every hold and the linger at once", () => {
    const { result } = renderHook(() => useWipeNamesReveal());
    act(() => result.current.hold("slider", true));
    act(() => result.current.pulse());
    act(() => result.current.clear());
    expect(result.current.shown).toBe(false);
  });

  it("does not pulse when releasing an unheld source", () => {
    const { result } = renderHook(() => useWipeNamesReveal());
    act(() => result.current.hold("handle", false));
    expect(result.current.shown).toBe(false);
    act(() => vi.advanceTimersByTime(2000));
    expect(result.current.shown).toBe(false);
  });
});

describe("usePulseOnSplitMove", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.5
  it("pulses when the split moves in Wipe, but not on entering Wipe", () => {
    const pulse = vi.fn();
    const reveal = { shown: false, hold: vi.fn(), pulse, clear: vi.fn() };
    const { rerender } = renderHook(
      (props: { active: boolean; split: string }) =>
        usePulseOnSplitMove(reveal, props.active, props.split),
      { initialProps: { active: false, split: "0.5" } },
    );
    rerender({ active: false, split: "0.6" });
    rerender({ active: true, split: "0.6" });
    expect(pulse).not.toHaveBeenCalled();
    rerender({ active: true, split: "0.7" });
    expect(pulse).toHaveBeenCalledTimes(1);
    rerender({ active: true, split: "0.7" });
    expect(pulse).toHaveBeenCalledTimes(1);
    rerender({ active: false, split: "0.7" });
    expect(reveal.clear).toHaveBeenCalled();
  });
});
