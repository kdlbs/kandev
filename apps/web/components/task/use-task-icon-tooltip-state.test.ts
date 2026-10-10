import { act, fireEvent, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useTaskIconTooltipState } from "./use-task-icon-tooltip-state";

function useHoverableTooltipState(onOpen?: () => void, openDelayMs = 0) {
  return useTaskIconTooltipState(onOpen, { hoverable: true, openDelayMs });
}

function mouseEvent() {
  return { pointerType: "mouse" } as never;
}

afterEach(() => vi.useRealTimers());

describe("delayed useTaskIconTooltipState disclosure", () => {
  it("allows keyboard focus after Escape cancels a pending mouse opening", () => {
    vi.useFakeTimers();
    const onOpen = vi.fn();
    const { result } = renderHook(() => useHoverableTooltipState(onOpen, 500));
    const trigger = document.createElement("span");
    vi.spyOn(trigger, "matches").mockReturnValue(true);
    act(() => result.current.onPointerEnter(mouseEvent()));
    fireEvent.keyDown(document.body, { key: "Escape" });
    act(() => result.current.onFocus({ type: "focus", currentTarget: trigger } as never));
    expect(result.current.open).toBe(true);
    expect(onOpen).toHaveBeenCalledTimes(1);
  });

  it("leaves pending hover intact for non-Escape keys and editable targets", () => {
    vi.useFakeTimers();
    const onOpen = vi.fn();
    const { result } = renderHook(() => useHoverableTooltipState(onOpen, 500));
    const input = document.createElement("input");
    document.body.append(input);
    try {
      act(() => result.current.onPointerEnter(mouseEvent()));
      fireEvent.keyDown(document.body, { key: "Enter" });
      fireEvent.keyDown(input, { key: "Escape" });
      act(() => vi.advanceTimersByTime(500));
      expect(result.current.open).toBe(true);
      expect(onOpen).toHaveBeenCalledTimes(1);
    } finally {
      input.remove();
    }
  });

  it("removes pending Escape listeners on pointer exit and unmount", () => {
    vi.useFakeTimers();
    const add = vi.spyOn(document, "addEventListener");
    const remove = vi.spyOn(document, "removeEventListener");
    try {
      const { result, unmount } = renderHook(() => useHoverableTooltipState(undefined, 500));
      act(() => result.current.onPointerEnter(mouseEvent()));
      const listener = add.mock.calls.find(([type]) => type === "keydown")?.[1];
      expect(listener).toBeTypeOf("function");
      act(() => result.current.onPointerLeave(mouseEvent()));
      expect(remove).toHaveBeenCalledWith("keydown", listener, true);

      act(() => result.current.onPointerEnter(mouseEvent()));
      const latestListener = add.mock.calls.filter(([type]) => type === "keydown").at(-1)?.[1];
      unmount();
      expect(remove).toHaveBeenCalledWith("keydown", latestListener, true);
      expect(vi.getTimerCount()).toBe(0);
    } finally {
      add.mockRestore();
      remove.mockRestore();
    }
  });
});

describe("delayed useTaskIconTooltipState timing", () => {
  it("waits for the full configured delay before opening on pointer entry", () => {
    vi.useFakeTimers();
    const onOpen = vi.fn();
    const { result } = renderHook(() => useHoverableTooltipState(onOpen, 500));

    act(() => result.current.onPointerEnter(mouseEvent()));
    expect(result.current.open).toBe(false);
    expect(onOpen).not.toHaveBeenCalled();

    act(() => vi.advanceTimersByTime(499));
    expect(result.current.open).toBe(false);
    expect(onOpen).not.toHaveBeenCalled();

    act(() => vi.advanceTimersByTime(1));
    expect(result.current.open).toBe(true);
    expect(onOpen).toHaveBeenCalledTimes(1);
  });

  it("cancels early exits and requires a full delay after re-entry", () => {
    vi.useFakeTimers();
    const onOpen = vi.fn();
    const { result } = renderHook(() => useHoverableTooltipState(onOpen, 500));

    act(() => result.current.onPointerEnter(mouseEvent()));
    act(() => vi.advanceTimersByTime(250));
    act(() => result.current.onPointerLeave(mouseEvent()));
    act(() => vi.advanceTimersByTime(500));
    expect(result.current.open).toBe(false);
    expect(onOpen).not.toHaveBeenCalled();

    act(() => result.current.onPointerEnter(mouseEvent()));
    act(() => vi.advanceTimersByTime(499));
    expect(result.current.open).toBe(false);
    act(() => vi.advanceTimersByTime(1));
    expect(result.current.open).toBe(true);
    expect(onOpen).toHaveBeenCalledTimes(1);
  });

  it("opens immediately for visible keyboard focus during a pending hover", () => {
    vi.useFakeTimers();
    const onOpen = vi.fn();
    const { result } = renderHook(() => useHoverableTooltipState(onOpen, 500));
    const trigger = document.createElement("span");
    vi.spyOn(trigger, "matches").mockImplementation((selector) => selector === ":focus-visible");

    act(() => result.current.onPointerEnter(mouseEvent()));
    act(() => result.current.onFocus({ type: "focus", currentTarget: trigger } as never));

    expect(result.current.open).toBe(true);
    expect(onOpen).toHaveBeenCalledTimes(1);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("uses the latest hydration callback once per actual opening", () => {
    vi.useFakeTimers();
    const staleCallback = vi.fn();
    const latestCallback = vi.fn();
    const { result, rerender } = renderHook(
      ({ callback }: { callback: () => void }) => useHoverableTooltipState(callback, 500),
      { initialProps: { callback: staleCallback } },
    );

    act(() => result.current.onPointerEnter(mouseEvent()));
    rerender({ callback: latestCallback });
    act(() => vi.advanceTimersByTime(500));
    expect(result.current.open).toBe(true);
    expect(staleCallback).not.toHaveBeenCalled();
    expect(latestCallback).toHaveBeenCalledTimes(1);

    rerender({ callback: vi.fn() });
    expect(latestCallback).toHaveBeenCalledTimes(1);
  });

  it("keeps independent delays for separate indicators", () => {
    vi.useFakeTimers();
    const first = renderHook(() => useHoverableTooltipState(undefined, 500));
    const second = renderHook(() => useHoverableTooltipState(undefined, 500));

    act(() => {
      first.result.current.onPointerEnter(mouseEvent());
      second.result.current.onPointerEnter(mouseEvent());
    });
    act(() => vi.advanceTimersByTime(250));
    act(() => first.result.current.onPointerLeave(mouseEvent()));
    act(() => vi.advanceTimersByTime(250));

    expect(first.result.current.open).toBe(false);
    expect(second.result.current.open).toBe(true);
  });
});

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
    act(() => vi.advanceTimersByTime(0));
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
