import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useRef } from "react";
import {
  useReviewSidebarResize,
  clampSidebarWidth,
  REVIEW_SIDEBAR_LIMITS,
} from "./use-review-sidebar-resize";

function mouseMove(clientX: number) {
  document.dispatchEvent(new MouseEvent("mousemove", { clientX, bubbles: true }));
}

function mouseUp() {
  document.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
}

function reactMouse(clientX: number) {
  return { clientX, preventDefault: () => {} } as unknown as React.MouseEvent;
}

describe("clampSidebarWidth", () => {
  it("returns the value when within [min, max]", () => {
    expect(clampSidebarWidth(250)).toBe(250);
  });

  it("clamps below min", () => {
    expect(clampSidebarWidth(50)).toBe(REVIEW_SIDEBAR_LIMITS.minWidth);
  });

  it("clamps above max", () => {
    expect(clampSidebarWidth(9999)).toBe(REVIEW_SIDEBAR_LIMITS.maxWidth);
  });

  it("clamps against (containerWidth - minDiffPaneWidth) when it bites first", () => {
    // container 700 - minDiffPane 320 → effective max 380
    expect(clampSidebarWidth(500, 700)).toBe(700 - REVIEW_SIDEBAR_LIMITS.minDiffPaneWidth);
  });

  it("does not let containerWidth force effective max below min", () => {
    // container 200 - minDiff 320 = -120 → floor at MIN_SIDEBAR_WIDTH
    expect(clampSidebarWidth(500, 200)).toBe(REVIEW_SIDEBAR_LIMITS.minWidth);
  });

  it("does not let containerWidth raise effective max above hard max", () => {
    // container 5000 - 320 = 4680 → capped by hard max
    expect(clampSidebarWidth(9999, 5000)).toBe(REVIEW_SIDEBAR_LIMITS.maxWidth);
  });
});

describe("useReviewSidebarResize", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
  });

  afterEach(() => {
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
  });

  it("defaults to DEFAULT_SIDEBAR_WIDTH when sessionStorage is empty", () => {
    const { result } = renderHook(() => useReviewSidebarResize());
    expect(result.current.width).toBe(REVIEW_SIDEBAR_LIMITS.defaultWidth);
    expect(typeof result.current.resizeHandleProps.onMouseDown).toBe("function");
  });

  it("reads a previously persisted width from sessionStorage on mount", () => {
    window.sessionStorage.setItem(REVIEW_SIDEBAR_LIMITS.storageKey, "350");
    const { result } = renderHook(() => useReviewSidebarResize());
    expect(result.current.width).toBe(350);
  });

  it("clamps a persisted width that's outside [min, max] on mount", () => {
    window.sessionStorage.setItem(REVIEW_SIDEBAR_LIMITS.storageKey, "9999");
    const { result } = renderHook(() => useReviewSidebarResize());
    expect(result.current.width).toBe(REVIEW_SIDEBAR_LIMITS.maxWidth);
  });

  it("falls back to default when sessionStorage contains a non-numeric value", () => {
    window.sessionStorage.setItem(REVIEW_SIDEBAR_LIMITS.storageKey, "not-a-number");
    const { result } = renderHook(() => useReviewSidebarResize());
    expect(result.current.width).toBe(REVIEW_SIDEBAR_LIMITS.defaultWidth);
  });

  it("updates width live during mousemove and clamps the result", () => {
    const { result } = renderHook(() => useReviewSidebarResize());
    act(() => result.current.resizeHandleProps.onMouseDown(reactMouse(300)));
    // move right by 80px → 220 + 80 = 300
    act(() => mouseMove(380));
    expect(result.current.width).toBe(300);

    // overshoot past max → clamps to maxWidth
    act(() => mouseMove(2000));
    expect(result.current.width).toBe(REVIEW_SIDEBAR_LIMITS.maxWidth);
  });

  it("persists final width to sessionStorage on mouseup", () => {
    const { result } = renderHook(() => useReviewSidebarResize());
    act(() => result.current.resizeHandleProps.onMouseDown(reactMouse(300)));
    act(() => mouseMove(380)); // → 300
    act(() => mouseUp());

    expect(window.sessionStorage.getItem(REVIEW_SIDEBAR_LIMITS.storageKey)).toBe("300");
    expect(result.current.width).toBe(300);
  });

  it("stops responding to mousemove after mouseup", () => {
    const { result } = renderHook(() => useReviewSidebarResize());
    act(() => result.current.resizeHandleProps.onMouseDown(reactMouse(300)));
    act(() => mouseMove(380));
    expect(result.current.width).toBe(300);

    act(() => mouseUp());
    act(() => mouseMove(500)); // would push to 420 if still listening

    expect(result.current.width).toBe(300);
  });

  it("sets body cursor + userSelect during drag and restores them on mouseup", () => {
    const { result } = renderHook(() => useReviewSidebarResize());
    act(() => result.current.resizeHandleProps.onMouseDown(reactMouse(300)));
    expect(document.body.style.cursor).toBe("col-resize");
    expect(document.body.style.userSelect).toBe("none");

    act(() => mouseUp());
    expect(document.body.style.cursor).toBe("");
    expect(document.body.style.userSelect).toBe("");
  });

  it("restores body styles when the component unmounts mid-drag", () => {
    const { result, unmount } = renderHook(() => useReviewSidebarResize());
    act(() => result.current.resizeHandleProps.onMouseDown(reactMouse(300)));
    expect(document.body.style.cursor).toBe("col-resize");

    act(() => unmount());

    expect(document.body.style.cursor).toBe("");
    expect(document.body.style.userSelect).toBe("");
    // After unmount, the now-detached window listeners must not be able to
    // mutate any state — a stray mousemove should be a no-op.
    act(() => mouseMove(800));
    // Nothing to assert on result.current after unmount, but the absence of
    // a thrown error from the listener confirms cleanup ran.
  });

  it("clamps width against the container width when one is provided", () => {
    // jsdom doesn't ship ResizeObserver; the initial reclamp still runs.
    window.sessionStorage.setItem(REVIEW_SIDEBAR_LIMITS.storageKey, "500");
    const { result } = renderHook(() => {
      const ref = useRef<HTMLDivElement>(null);
      // Simulate a container element that reports a 600px width.
      const fakeEl = { getBoundingClientRect: () => ({ width: 600 }) as DOMRect };
      (ref as React.MutableRefObject<HTMLDivElement | null>).current =
        fakeEl as unknown as HTMLDivElement;
      return useReviewSidebarResize(ref);
    });
    // 600 - 320 = 280 → stored 500 should be clamped down to 280
    expect(result.current.width).toBe(600 - REVIEW_SIDEBAR_LIMITS.minDiffPaneWidth);
  });

  it("reclamps when the dialog transitions from closed to open", () => {
    // Mirrors the real lifecycle: Radix doesn't mount the DialogContent
    // portal while open=false, so containerRef.current is null on first
    // effect run. When open flips to true, the effect must re-fire and
    // pick up the now-populated element.
    window.sessionStorage.setItem(REVIEW_SIDEBAR_LIMITS.storageKey, "500");
    const ref = { current: null } as React.MutableRefObject<HTMLDivElement | null>;
    const { result, rerender } = renderHook(({ open }) => useReviewSidebarResize(ref, open), {
      initialProps: { open: false },
    });
    // Closed → stored width returned unclamped by container (since no el).
    expect(result.current.width).toBe(500);

    // Dialog opens: portal mounts, ref populates with a 600px container.
    const fakeEl = { getBoundingClientRect: () => ({ width: 600 }) as DOMRect };
    ref.current = fakeEl as unknown as HTMLDivElement;
    rerender({ open: true });

    // Effect must re-run and reclamp to 600 - 320 = 280.
    expect(result.current.width).toBe(600 - REVIEW_SIDEBAR_LIMITS.minDiffPaneWidth);
  });
});
