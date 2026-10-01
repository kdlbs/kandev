import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useTaskIconTooltipState } from "./use-task-icon-tooltip-state";

function useHoverableTooltipState(onOpen?: () => void) {
  return useTaskIconTooltipState(onOpen, { hoverable: true });
}

function mouseEvent() {
  return { pointerType: "mouse" } as never;
}

afterEach(() => vi.useRealTimers());

describe("useTaskIconTooltipState", () => {
  it("leaves Escape available to an editable target", () => {
    const { result } = renderHook(() => useTaskIconTooltipState());
    act(() => result.current.onPointerEnter({ pointerType: "mouse" } as never));
    expect(result.current.open).toBe(true);

    const input = document.createElement("input");
    act(() => result.current.onEscapeKeyDown({ target: input } as unknown as Event));

    expect(result.current.open).toBe(true);
  });

  it("dismisses the tooltip for Escape from a non-editable target", () => {
    const { result } = renderHook(() => useTaskIconTooltipState());
    act(() => result.current.onPointerEnter({ pointerType: "mouse" } as never));

    const span = document.createElement("span");
    act(() => result.current.onEscapeKeyDown({ target: span } as unknown as Event));

    expect(result.current.open).toBe(false);
  });

  it("keeps the opt-in disclosure open as the pointer moves into content", () => {
    vi.useFakeTimers();
    const onOpen = vi.fn();
    const { result } = renderHook(() => useHoverableTooltipState(onOpen));

    act(() => result.current.onPointerEnter(mouseEvent()));
    expect(result.current.open).toBe(true);
    expect(onOpen).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.onPointerLeave(mouseEvent());
      result.current.onContentPointerEnter(mouseEvent());
      vi.advanceTimersByTime(200);
    });
    expect(result.current.open).toBe(true);
    expect(onOpen).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.onContentPointerLeave(mouseEvent());
      vi.advanceTimersByTime(200);
    });
    expect(result.current.open).toBe(false);
  });

  it("keeps keyboard focus in the disclosure content and Escape dismisses it", () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useHoverableTooltipState());
    const trigger = document.createElement("span");
    vi.spyOn(trigger, "matches").mockImplementation((selector) => selector === ":focus-visible");

    act(() => result.current.onFocus({ type: "focus", currentTarget: trigger } as never));
    expect(result.current.open).toBe(true);
    const content = document.createElement("div");
    vi.spyOn(content, "matches").mockReturnValue(true);

    act(() => {
      result.current.onBlur({ type: "blur" } as never);
      result.current.onContentFocus({ type: "focus", target: content } as never);
      vi.advanceTimersByTime(200);
    });
    expect(result.current.open).toBe(true);

    let dismissed = false;
    act(() => {
      dismissed = result.current.onEscapeKeyDown({
        target: document.createElement("span"),
      } as unknown as Event);
      result.current.onContentBlur({ type: "blur" } as never);
      result.current.onFocus({ type: "focus", currentTarget: trigger } as never);
    });
    expect(dismissed).toBe(true);
    expect(result.current.open).toBe(false);

    act(() => vi.advanceTimersByTime(200));
    expect(result.current.open).toBe(false);

    act(() => result.current.onBlur({ type: "blur" } as never));
    act(() => result.current.onFocus({ type: "focus", currentTarget: trigger } as never));
    expect(result.current.open).toBe(true);
  });

  it("cleans up the opt-in close timer when the disclosure unmounts", () => {
    vi.useFakeTimers();
    const { result, unmount } = renderHook(() => useHoverableTooltipState());

    act(() => result.current.onPointerEnter(mouseEvent()));
    act(() => result.current.onPointerLeave(mouseEvent()));
    expect(vi.getTimerCount()).toBeGreaterThan(0);

    unmount();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("keeps the default disclosure's immediate pointer-leave behavior", () => {
    const { result } = renderHook(() => useTaskIconTooltipState());

    act(() => result.current.onPointerEnter(mouseEvent()));
    expect(result.current.open).toBe(true);
    act(() => result.current.onPointerLeave(mouseEvent()));

    expect(result.current.open).toBe(false);
  });
});

describe("Escape focus restoration", () => {
  it("reopens on the first deliberate refocus after Escape on the trigger", () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useHoverableTooltipState());
    const trigger = document.createElement("span");
    vi.spyOn(trigger, "matches").mockImplementation((selector) => selector === ":focus-visible");

    act(() => result.current.onFocus({ type: "focus", currentTarget: trigger } as never));
    expect(result.current.open).toBe(true);

    let dismissed = false;
    act(() => {
      dismissed = result.current.onEscapeKeyDown({ target: trigger } as unknown as Event);
    });
    expect(dismissed).toBe(true);
    expect(result.current.open).toBe(false);

    act(() => result.current.onBlur({ type: "blur" } as never));
    act(() => result.current.onFocus({ type: "focus", currentTarget: trigger } as never));

    expect(result.current.open).toBe(true);
  });
});
