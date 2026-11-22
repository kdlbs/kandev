import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { HOLD_DELAY_MS, useHoldToCompare } from "./use-hold-to-compare";

function pointer(type: string, x = 0, y = 0, pointerId = 1) {
  return {
    type,
    clientX: x,
    clientY: y,
    pointerId,
    isPrimary: pointerId === 1,
  } as unknown as React.PointerEvent<HTMLElement>;
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("useHoldToCompare", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.12
  it("shows A while a still press is held and releases on pointer up", () => {
    const { result } = renderHook(() => useHoldToCompare(true));
    act(() => result.current.pointer.onPointerDown(pointer("pointerdown")));
    expect(result.current.holding).toBe(false);
    act(() => vi.advanceTimersByTime(HOLD_DELAY_MS));
    expect(result.current.holding).toBe(true);
    act(() => result.current.pointer.onPointerUp(pointer("pointerup")));
    expect(result.current.holding).toBe(false);
  });

  it("does not hold when the press moves, as for panning", () => {
    const { result } = renderHook(() => useHoldToCompare(true));
    act(() => result.current.pointer.onPointerDown(pointer("pointerdown")));
    act(() => result.current.pointer.onPointerMove(pointer("pointermove", 20, 0)));
    act(() => vi.advanceTimersByTime(HOLD_DELAY_MS));
    expect(result.current.holding).toBe(false);
  });

  it("holds with Space and ignores key repeat", () => {
    const { result } = renderHook(() => useHoldToCompare(true));
    act(() => {
      expect(result.current.keyDown({ key: " ", repeat: false })).toBe(true);
    });
    expect(result.current.holding).toBe(true);
    act(() => {
      expect(result.current.keyDown({ key: " ", repeat: true })).toBe(true);
    });
    act(() => {
      expect(result.current.keyUp({ key: " " })).toBe(true);
    });
    expect(result.current.holding).toBe(false);
  });

  it("does nothing when disabled", () => {
    const { result } = renderHook(() => useHoldToCompare(false));
    act(() => result.current.pointer.onPointerDown(pointer("pointerdown")));
    act(() => vi.advanceTimersByTime(HOLD_DELAY_MS));
    expect(result.current.holding).toBe(false);
    expect(result.current.keyDown({ key: " ", repeat: false })).toBe(false);
  });

  it("releases when Space is let go elsewhere or the window loses focus", () => {
    const { result } = renderHook(() => useHoldToCompare(true));
    act(() => {
      result.current.keyDown({ key: " ", repeat: false });
    });
    act(() => {
      window.dispatchEvent(new KeyboardEvent("keyup", { key: " " }));
    });
    expect(result.current.holding).toBe(false);

    act(() => {
      result.current.keyDown({ key: " ", repeat: false });
    });
    act(() => {
      window.dispatchEvent(new Event("blur"));
    });
    expect(result.current.holding).toBe(false);
  });
});
