import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useResizableClarificationOverlay } from "./use-resizable-clarification-overlay";

const MIN_HEIGHT = 120;

function setInnerHeight(h: number) {
  Object.defineProperty(window, "innerHeight", { configurable: true, value: h, writable: true });
}

// Stub getBoundingClientRect on the (detached) container ref so the hook's
// "measure current rendered height on drag start" branch has a value to read.
function attachContainerWithHeight(ref: React.RefObject<HTMLDivElement | null>, height: number) {
  const el = document.createElement("div");
  el.getBoundingClientRect = () => ({
    height,
    width: 800,
    x: 0,
    y: 0,
    top: 0,
    left: 0,
    right: 800,
    bottom: height,
    toJSON: () => ({}),
  });
  (ref as { current: HTMLDivElement }).current = el;
}

function dispatchMouseMove(clientY: number) {
  document.dispatchEvent(new MouseEvent("mousemove", { clientY, bubbles: true }));
}

function dispatchMouseUp() {
  document.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
}

// Fake MouseEvent for handleMouseDown — only clientY and preventDefault are read.
function fakeReactMouseEvent(clientY: number) {
  return { clientY, preventDefault: () => {} } as unknown as React.MouseEvent;
}

describe("useResizableClarificationOverlay", () => {
  beforeEach(() => {
    setInnerHeight(1000);
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
  });

  afterEach(() => {
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
  });

  it("starts with height=null (auto-sized) and exposes a containerRef + handle props", () => {
    const { result } = renderHook(() => useResizableClarificationOverlay());
    expect(result.current.height).toBeNull();
    expect(result.current.containerRef.current).toBeNull();
    expect(typeof result.current.resizeHandleProps.onMouseDown).toBe("function");
    expect(typeof result.current.resizeHandleProps.onDoubleClick).toBe("function");
  });

  it("drag UP from a measured 300px grows height proportionally", () => {
    const { result } = renderHook(() => useResizableClarificationOverlay());
    attachContainerWithHeight(result.current.containerRef, 300);

    // Mouse down at y=500, drag up to y=400 (delta=100), expect height ≈ 400.
    act(() => result.current.resizeHandleProps.onMouseDown(fakeReactMouseEvent(500)));
    act(() => dispatchMouseMove(400));

    expect(result.current.height).toBe(400);
  });

  it("clamps below MIN_HEIGHT when dragging down past it", () => {
    const { result } = renderHook(() => useResizableClarificationOverlay());
    attachContainerWithHeight(result.current.containerRef, 200);

    // Drag down (clientY grows) 500px → would target -300, must clamp to MIN_HEIGHT.
    act(() => result.current.resizeHandleProps.onMouseDown(fakeReactMouseEvent(200)));
    act(() => dispatchMouseMove(700));

    expect(result.current.height).toBe(MIN_HEIGHT);
  });

  it("clamps above 50% of viewport when dragging up past it", () => {
    setInnerHeight(800); // 50% = 400 cap
    const { result } = renderHook(() => useResizableClarificationOverlay());
    attachContainerWithHeight(result.current.containerRef, 300);

    act(() => result.current.resizeHandleProps.onMouseDown(fakeReactMouseEvent(500)));
    act(() => dispatchMouseMove(0)); // delta=500 → 800 requested, must clamp to 400

    expect(result.current.height).toBe(400);
  });

  it("does not update height after mouseup", () => {
    const { result } = renderHook(() => useResizableClarificationOverlay());
    attachContainerWithHeight(result.current.containerRef, 250);

    act(() => result.current.resizeHandleProps.onMouseDown(fakeReactMouseEvent(500)));
    act(() => dispatchMouseMove(450)); // delta=50 → 300
    expect(result.current.height).toBe(300);

    act(() => dispatchMouseUp());
    act(() => dispatchMouseMove(200)); // would target 550 if still dragging

    expect(result.current.height).toBe(300);
  });

  it("sets document.body cursor/userSelect during drag and restores on mouseup", () => {
    const { result } = renderHook(() => useResizableClarificationOverlay());
    attachContainerWithHeight(result.current.containerRef, 200);

    act(() => result.current.resizeHandleProps.onMouseDown(fakeReactMouseEvent(400)));
    expect(document.body.style.cursor).toBe("ns-resize");
    expect(document.body.style.userSelect).toBe("none");

    act(() => dispatchMouseUp());
    expect(document.body.style.cursor).toBe("");
    expect(document.body.style.userSelect).toBe("");
  });

  it("falls back to MIN_HEIGHT as the drag origin when the container ref is empty", () => {
    const { result } = renderHook(() => useResizableClarificationOverlay());
    // No containerRef attached → fallback path.

    act(() => result.current.resizeHandleProps.onMouseDown(fakeReactMouseEvent(500)));
    act(() => dispatchMouseMove(400)); // delta=100 → MIN_HEIGHT + 100 = 220

    expect(result.current.height).toBe(MIN_HEIGHT + 100);
  });

  it("resetHeight and the handle's onDoubleClick both revert to null (auto)", () => {
    const { result } = renderHook(() => useResizableClarificationOverlay());
    attachContainerWithHeight(result.current.containerRef, 250);

    act(() => result.current.resizeHandleProps.onMouseDown(fakeReactMouseEvent(500)));
    act(() => dispatchMouseMove(400));
    act(() => dispatchMouseUp());
    // startHeight=250 + delta=100 (500→400) = 350
    expect(result.current.height).toBe(350);

    act(() => result.current.resetHeight());
    expect(result.current.height).toBeNull();

    // Drag again, then double-click resets back to auto-sized.
    act(() => result.current.resizeHandleProps.onMouseDown(fakeReactMouseEvent(500)));
    act(() => dispatchMouseMove(380));
    act(() => dispatchMouseUp());
    expect(result.current.height).not.toBeNull();

    act(() => result.current.resizeHandleProps.onDoubleClick());
    expect(result.current.height).toBeNull();
  });

  it("cleans up body cursor and stops listening on unmount mid-drag", () => {
    const { result, unmount } = renderHook(() => useResizableClarificationOverlay());
    attachContainerWithHeight(result.current.containerRef, 250);

    act(() => result.current.resizeHandleProps.onMouseDown(fakeReactMouseEvent(500)));
    expect(document.body.style.cursor).toBe("ns-resize");

    unmount();

    // Unmount mid-drag must restore body styles.
    expect(document.body.style.cursor).toBe("");
    expect(document.body.style.userSelect).toBe("");

    // Listeners removed: a synthetic mousemove must not throw / update anything.
    expect(() => dispatchMouseMove(200)).not.toThrow();
    expect(result.current.height).toBeNull();
  });
});
