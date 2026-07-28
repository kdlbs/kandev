import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import type { EditorView } from "@codemirror/view";

const hookState = vi.hoisted(() => ({
  tasks: { activeTaskId: "task-1" },
  walkthroughs: {
    activeStepByTaskId: { "task-1": 0 },
    byTaskId: {
      "task-1": {
        steps: [
          {
            file: "walkthrough_a.txt",
            line: 2,
            line_end: 3,
            text: "Explain this range",
          },
        ],
      },
    },
  },
}));

const anchorMocks = vi.hoisted(() => ({
  clear: vi.fn(),
  isVisible: vi.fn(() => true),
  set: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof hookState) => unknown) => selector(hookState),
}));

vi.mock("@/lib/walkthrough-open-state", () => ({
  useIsWalkthroughOpenForTask: () => true,
}));

vi.mock("@/lib/walkthrough-editor-anchor", () => ({
  clearWalkthroughEditorAnchor: anchorMocks.clear,
  isWalkthroughAnchorTargetVisible: anchorMocks.isVisible,
  setWalkthroughEditorAnchor: anchorMocks.set,
}));

vi.mock("@codemirror/view", () => ({
  EditorView: { scrollIntoView: vi.fn() },
}));

import { useCodeMirrorWalkthroughRange } from "./use-codemirror-walkthrough-range";

function rect(left: number, top: number, width: number, height: number): DOMRect {
  return { left, top, width, height, right: left + width, bottom: top + height } as DOMRect;
}

function createView(): EditorView {
  const dom = document.createElement("div");
  document.body.append(dom);
  vi.spyOn(dom, "getBoundingClientRect").mockReturnValue(rect(20, 30, 500, 300));
  return {
    coordsAtPos: (position: number) =>
      position < 30 ? { left: 24, top: 70, bottom: 90 } : { left: 24, top: 90, bottom: 110 },
    dispatch: vi.fn(),
    dom,
    scrollDOM: dom,
    state: {
      doc: {
        line: (line: number) => ({ from: line * 10, to: line * 10 + 5 }),
        lines: 3,
      },
    },
  } as unknown as EditorView;
}

beforeEach(() => {
  anchorMocks.clear.mockClear();
  anchorMocks.isVisible.mockClear();
  anchorMocks.isVisible.mockReturnValue(true);
  anchorMocks.set.mockClear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  document.body.replaceChildren();
});

describe("useCodeMirrorWalkthroughRange", () => {
  it("preserves the range box but clears the anchor when its target is occluded", () => {
    const view = createView();
    const area = document.createElement("div");
    document.body.append(area);
    const editorAreaRef = { current: area };
    vi.spyOn(area, "getBoundingClientRect").mockReturnValue(rect(0, 0, 600, 400));
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(0);
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    vi.stubGlobal(
      "ResizeObserver",
      class {
        disconnect() {}
        observe() {}
      },
    );
    anchorMocks.isVisible.mockReturnValue(false);

    const { result } = renderHook(() =>
      useCodeMirrorWalkthroughRange({
        view,
        editorAreaRef,
        path: "walkthrough_a.txt",
      }),
    );

    expect(result.current).toMatchObject({ startLine: 2, endLine: 3 });
    expect(anchorMocks.clear).toHaveBeenCalledWith("task-1:0::walkthrough_a.txt:cm");
    expect(anchorMocks.set).not.toHaveBeenCalled();
  });
});
