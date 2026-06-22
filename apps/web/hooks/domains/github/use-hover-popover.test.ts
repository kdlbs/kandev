import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";

import { useHoverPopover } from "./use-hover-popover";

const OPEN = 150;
const CLOSE = 150;

function setup(disabled = false) {
  return renderHook(() => useHoverPopover({ openDelayMs: OPEN, closeDelayMs: CLOSE, disabled }));
}

describe("useHoverPopover", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("opens after the open delay on trigger enter", () => {
    const { result } = setup();
    act(() => result.current.onTriggerEnter());
    expect(result.current.open).toBe(false);
    act(() => vi.advanceTimersByTime(OPEN));
    expect(result.current.open).toBe(true);
  });

  it("closes after the close delay once the pointer leaves both regions", () => {
    const { result } = setup();
    act(() => result.current.onTriggerEnter());
    act(() => vi.advanceTimersByTime(OPEN));
    act(() => result.current.onTriggerLeave());
    expect(result.current.open).toBe(true);
    act(() => vi.advanceTimersByTime(CLOSE));
    expect(result.current.open).toBe(false);
  });

  it("stays open when the cursor bridges trigger -> content (leave then enter)", () => {
    const { result } = setup();
    act(() => result.current.onTriggerEnter());
    act(() => vi.advanceTimersByTime(OPEN));
    // Normal ordering: leave the trigger, then enter the content.
    act(() => result.current.onTriggerLeave());
    act(() => result.current.onContentEnter());
    act(() => vi.advanceTimersByTime(CLOSE * 2));
    expect(result.current.open).toBe(true);
  });

  it("stays open when content-enter fires BEFORE trigger-leave (portal ordering)", () => {
    const { result } = setup();
    act(() => result.current.onTriggerEnter());
    act(() => vi.advanceTimersByTime(OPEN));
    // Portal can dispatch the content's mouseenter before the trigger's
    // mouseleave — the regression this hook exists to prevent.
    act(() => result.current.onContentEnter());
    act(() => result.current.onTriggerLeave());
    act(() => vi.advanceTimersByTime(CLOSE * 2));
    expect(result.current.open).toBe(true);
  });

  it("closes after leaving the content too", () => {
    const { result } = setup();
    act(() => result.current.onTriggerEnter());
    act(() => vi.advanceTimersByTime(OPEN));
    act(() => result.current.onTriggerLeave());
    act(() => result.current.onContentEnter());
    act(() => result.current.onContentLeave());
    act(() => vi.advanceTimersByTime(CLOSE));
    expect(result.current.open).toBe(false);
  });

  it("re-entering the content cancels a pending close", () => {
    const { result } = setup();
    act(() => result.current.onTriggerEnter());
    act(() => vi.advanceTimersByTime(OPEN));
    act(() => result.current.onContentEnter());
    act(() => result.current.onContentLeave());
    // Back onto the content before the close fires.
    act(() => vi.advanceTimersByTime(CLOSE / 2));
    act(() => result.current.onContentEnter());
    act(() => vi.advanceTimersByTime(CLOSE * 2));
    expect(result.current.open).toBe(true);
  });

  it("never opens while disabled (mobile/touch)", () => {
    const { result } = setup(true);
    act(() => result.current.onTriggerEnter());
    act(() => vi.advanceTimersByTime(OPEN * 2));
    expect(result.current.open).toBe(false);
  });

  it("onOpenChange(false) force-closes and clears hover regions", () => {
    const { result } = setup();
    act(() => result.current.onOpenChange(true));
    expect(result.current.open).toBe(true);
    act(() => result.current.onOpenChange(false));
    expect(result.current.open).toBe(false);
  });
});
