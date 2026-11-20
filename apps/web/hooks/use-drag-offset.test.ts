import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { startMouseDrag, useDrag } from "./use-drag-offset";

const SHIELD = "[data-selection-comment-drag-shield]";

function mouse(type: string, x: number, y: number) {
  document.dispatchEvent(new MouseEvent(type, { clientX: x, clientY: y, bubbles: true }));
}

afterEach(() => {
  document.body.innerHTML = "";
});

describe("startMouseDrag", () => {
  it("reports movement over a shield and ends on mouseup", () => {
    const onMove = vi.fn();
    const onEnd = vi.fn();
    const preventDefault = vi.fn();
    startMouseDrag({ clientX: 10, clientY: 20, preventDefault }, { onMove, onEnd });

    expect(preventDefault).toHaveBeenCalled();
    expect(document.querySelector(SHIELD)).not.toBeNull();
    mouse("mousemove", 25, 50);
    expect(onMove).toHaveBeenLastCalledWith(15, 30);

    mouse("mouseup", 30, 60);
    expect(onEnd).toHaveBeenCalledWith(20, 40);
    expect(document.querySelector(SHIELD)).toBeNull();
    mouse("mousemove", 90, 90);
    expect(onMove).toHaveBeenCalledTimes(1);
  });
});

describe("useDrag", () => {
  it("accumulates the offset across drags", () => {
    const { result } = renderHook(() => useDrag());
    const start = (x: number, y: number) =>
      act(() =>
        result.current.onMouseDown({
          clientX: x,
          clientY: y,
          preventDefault: () => undefined,
        } as React.MouseEvent),
      );
    start(0, 0);
    act(() => mouse("mousemove", 10, 5));
    act(() => mouse("mouseup", 10, 5));
    start(100, 100);
    act(() => mouse("mousemove", 104, 103));
    act(() => mouse("mouseup", 104, 103));
    expect(result.current.offset).toEqual({ dx: 14, dy: 8 });
  });
});
