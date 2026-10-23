import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  getWalkthroughEditorAnchor,
  setWalkthroughEditorAnchor,
} from "@/lib/walkthrough-editor-anchor";
import { consumePendingCursorPosition, setPendingCursorPosition } from "@/hooks/file-editor-cursor";
import {
  useRenderedWalkthroughRange,
  type RenderedWalkthroughAdapter,
} from "./use-rendered-walkthrough-range";
import type { MarkdownWalkthroughTarget } from "@/lib/markdown/walkthrough-range";
const target: MarkdownWalkthroughTarget = {
  key: "task:0:3-5",
  path: "guide.md",
  taskId: "task",
  sessionId: "session",
  stepIndex: 0,
  file: "guide.md",
  startLine: 3,
  endLine: 5,
};
const rect = { left: 10, top: 40, right: 410, bottom: 100, width: 400, height: 60 };
let container: HTMLDivElement;
let adapter: RenderedWalkthroughAdapter;
let covered = false;
let resize: () => void;
beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: () => void) {
        resize = callback;
      }
      observe() {}
      disconnect() {}
    },
  );
  container = document.createElement("div");
  document.body.append(container);
  vi.spyOn(container, "getBoundingClientRect").mockReturnValue({
    ...rect,
    top: 0,
    height: 300,
    bottom: 300,
  } as DOMRect);
  Object.defineProperty(document, "elementFromPoint", {
    configurable: true,
    value: () => (covered ? document.body : container),
  });
  adapter = {
    container,
    overlay: container,
    measure: () => ({ rect, box: rect }),
    reveal: vi.fn(() => ({ dispose: vi.fn() })),
  };
  covered = false;
});
afterEach(() => {
  cleanup();
  document.body.replaceChildren();
  setWalkthroughEditorAnchor(null);
  consumePendingCursorPosition(target.path, undefined, target.sessionId);
  vi.useRealTimers();
  vi.unstubAllGlobals();
});
function frame() {
  act(() => vi.advanceTimersByTime(20));
}

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.18
// @covers AC-UI-MARKDOWN-FILE-EDITING-002.19
it("reveals once, updates after layout, preserves an occluded outline and cancels on hide", () => {
  setPendingCursorPosition(target.path, 3, 1, undefined, { walkthroughOwner: Symbol() });
  const { rerender } = renderHook(
    ({ active, content }) => useRenderedWalkthroughRange({ target, active, content, adapter }),
    { initialProps: { active: true, content: "first" } },
  );
  frame();
  expect(container.querySelector("[data-line-range='3-5']")).not.toBeNull();
  expect(getWalkthroughEditorAnchor()?.rect).toEqual(rect);
  expect(consumePendingCursorPosition(target.path)).toBeUndefined();
  expect(adapter.reveal).toHaveBeenCalledOnce();
  covered = true;
  act(() => resize());
  frame();
  expect(getWalkthroughEditorAnchor()).toBeNull();
  expect((container.firstChild as HTMLElement).hidden).toBe(false);
  rerender({ active: true, content: "replaced" });
  frame();
  expect(adapter.reveal).toHaveBeenCalledOnce();
  rerender({ active: false, content: "replaced" });
  frame();
  expect(container.childElementCount).toBe(0);
  expect(getWalkthroughEditorAnchor()).toBeNull();
  covered = false;
  rerender({ active: true, content: "replaced" });
  frame();
  expect(adapter.reveal).toHaveBeenCalledTimes(2);
});
it("old instance cleanup cannot clear its replacement and newer navigation survives", () => {
  const first = renderHook(() =>
    useRenderedWalkthroughRange({ target, active: true, content: "", adapter }),
  );
  frame();
  setPendingCursorPosition(target.path, 12, 2, undefined, { flashLine: true });
  const second = renderHook(() =>
    useRenderedWalkthroughRange({ target, active: true, content: "", adapter }),
  );
  frame();
  const replacement = getWalkthroughEditorAnchor();
  first.unmount();
  frame();
  expect(getWalkthroughEditorAnchor()).toBe(replacement);
  expect(consumePendingCursorPosition(target.path)).toEqual({
    line: 12,
    column: 2,
    flashLine: true,
  });
  second.unmount();
  expect(getWalkthroughEditorAnchor()).toBeNull();
});
it("empty geometry has no indicator or anchor, and a stale scheduled callback is inert", () => {
  adapter.measure = () => null;
  const { unmount } = renderHook(() =>
    useRenderedWalkthroughRange({ target, active: true, content: "", adapter }),
  );
  frame();
  expect((container.firstChild as HTMLElement).hidden).toBe(true);
  expect(getWalkthroughEditorAnchor()).toBeNull();
  act(() => resize());
  unmount();
  frame();
  expect(container.childElementCount).toBe(0);
  expect(getWalkthroughEditorAnchor()).toBeNull();
});
