import { act, renderHook } from "@testing-library/react";
import type { editor as monacoEditor } from "monaco-editor";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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

const WALKTHROUGH_FILE = "walkthrough_a.txt";

import {
  buildWalkthroughRangeDecorations,
  clampWalkthroughRangeToLineCount,
  getWalkthroughEditorRange,
  useMonacoWalkthroughRange,
} from "./use-monaco-walkthrough-range";

function createModelSwitchingEditor(initialLineCount: number) {
  let lineCount = initialLineCount;
  let modelRevision = 0;
  let model = { id: "model-0", getLineCount: () => lineCount };
  const contentListeners = new Set<() => void>();
  const modelListeners = new Set<() => void>();
  const setDecorations = vi.fn();
  const revealLinesInCenter = vi.fn();
  const editor = {
    createDecorationsCollection: () => ({ set: setDecorations }),
    getModel: () => model,
    onDidChangeModel: (listener: () => void) => {
      modelListeners.add(listener);
      return {
        dispose: () => {
          modelListeners.delete(listener);
        },
      };
    },
    onDidChangeModelContent: (listener: () => void) => {
      contentListeners.add(listener);
      return {
        dispose: () => {
          contentListeners.delete(listener);
        },
      };
    },
    revealLinesInCenter,
  } as unknown as monacoEditor.IStandaloneCodeEditor;

  return {
    editor,
    revealLinesInCenter,
    setDecorations,
    decoratedLines() {
      const decorations = setDecorations.mock.lastCall?.[0] ?? [];
      return decorations.map(
        (decoration: monacoEditor.IModelDeltaDecoration) => decoration.range.startLineNumber,
      );
    },
    listenerCounts() {
      return { content: contentListeners.size, model: modelListeners.size };
    },
    changeLineCount(nextLineCount: number) {
      lineCount = nextLineCount;
      for (const listener of contentListeners) listener();
    },
    switchModel(nextLineCount: number) {
      lineCount = nextLineCount;
      modelRevision += 1;
      model = { id: `model-${modelRevision}`, getLineCount: () => lineCount };
      for (const listener of modelListeners) listener();
    },
  };
}

function rect(left: number, top: number, width: number, height: number): DOMRect {
  return { left, top, width, height, right: left + width, bottom: top + height } as DOMRect;
}

function createGeometryEditor() {
  const dom = document.createElement("div");
  document.body.append(dom);
  vi.spyOn(dom, "getBoundingClientRect").mockReturnValue(rect(20, 30, 500, 300));
  const noopSubscription = { dispose: vi.fn() };
  const editor = {
    createDecorationsCollection: () => ({ set: vi.fn() }),
    getDomNode: () => dom,
    getModel: () => ({ id: "model-0", getLineCount: () => 3 }),
    getScrolledVisiblePosition: ({ lineNumber }: { lineNumber: number }) => ({
      top: lineNumber * 20,
      left: 24,
      height: 20,
    }),
    onDidChangeModel: () => noopSubscription,
    onDidChangeModelContent: () => noopSubscription,
    onDidLayoutChange: () => noopSubscription,
    onDidScrollChange: () => noopSubscription,
    revealLinesInCenter: vi.fn(),
  } as unknown as monacoEditor.IStandaloneCodeEditor;
  return { dom, editor };
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

describe("getWalkthroughEditorRange", () => {
  it("returns the active walkthrough range for a matching editor file", () => {
    expect(
      getWalkthroughEditorRange(
        { path: "/tmp/worktree/src/app.ts" },
        { file: "src/app.ts", line: 8, line_end: 10, text: "Explain this" },
      ),
    ).toEqual({ startLine: 8, endLine: 10 });
  });

  it("returns null for a different file", () => {
    expect(
      getWalkthroughEditorRange(
        { path: "src/other.ts" },
        { file: "src/app.ts", line: 8, text: "Explain this" },
      ),
    ).toBeNull();
  });
});

describe("buildWalkthroughRangeDecorations", () => {
  it("builds a decoration for every line in the walkthrough range", () => {
    const decorations = buildWalkthroughRangeDecorations({ startLine: 2, endLine: 3 });

    expect(decorations).toHaveLength(2);
    expect(decorations[0].range).toMatchObject({ startLineNumber: 2, endLineNumber: 2 });
    expect(decorations[1].range).toMatchObject({ startLineNumber: 3, endLineNumber: 3 });
    expect(decorations[0].options.className).toBe("monaco-walkthrough-line");
  });
});

describe("clampWalkthroughRangeToLineCount", () => {
  it("clamps stale walkthrough ranges to the current Monaco model line count", () => {
    expect(clampWalkthroughRangeToLineCount({ startLine: 20, endLine: 24 }, 12)).toEqual({
      startLine: 12,
      endLine: 12,
    });
  });
});

describe("useMonacoWalkthroughRange", () => {
  it("preserves the range box but clears the anchor when its target is occluded", () => {
    const { dom, editor } = createGeometryEditor();
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
      useMonacoWalkthroughRange({
        editor,
        editorAreaRef,
        path: WALKTHROUGH_FILE,
      }),
    );

    expect(result.current).toMatchObject({ startLine: 2, endLine: 3 });
    expect(anchorMocks.clear).toHaveBeenCalledWith("task-1:0::walkthrough_a.txt");
    expect(anchorMocks.set).not.toHaveBeenCalled();
    dom.remove();
  });
});

describe("useMonacoWalkthroughRange model changes", () => {
  it("reclamps the active range after Monaco switches models", () => {
    const fake = createModelSwitchingEditor(2);
    renderHook(() =>
      useMonacoWalkthroughRange({
        editor: fake.editor,
        editorAreaRef: { current: null },
        path: WALKTHROUGH_FILE,
      }),
    );
    expect(fake.decoratedLines()).toEqual([2]);

    act(() => fake.switchModel(3));

    expect(fake.decoratedLines()).toEqual([2, 3]);
    expect(fake.revealLinesInCenter).toHaveBeenLastCalledWith(2, 3);
  });

  it("reclamps when the current Monaco model line count changes", () => {
    const fake = createModelSwitchingEditor(3);
    renderHook(() =>
      useMonacoWalkthroughRange({
        editor: fake.editor,
        editorAreaRef: { current: null },
        path: WALKTHROUGH_FILE,
      }),
    );

    act(() => fake.changeLineCount(2));

    expect(fake.decoratedLines()).toEqual([2]);
    expect(fake.revealLinesInCenter).toHaveBeenLastCalledWith(2, 2);
  });

  it("does not recenter for same-model line count changes outside the active range", () => {
    const fake = createModelSwitchingEditor(3);
    renderHook(() =>
      useMonacoWalkthroughRange({
        editor: fake.editor,
        editorAreaRef: { current: null },
        path: WALKTHROUGH_FILE,
      }),
    );
    fake.revealLinesInCenter.mockClear();

    act(() => fake.changeLineCount(4));

    expect(fake.revealLinesInCenter).not.toHaveBeenCalled();
  });

  it("reapplies the range after switching to a model with the same line count", () => {
    const fake = createModelSwitchingEditor(3);
    renderHook(() =>
      useMonacoWalkthroughRange({
        editor: fake.editor,
        editorAreaRef: { current: null },
        path: WALKTHROUGH_FILE,
      }),
    );
    fake.setDecorations.mockClear();
    fake.revealLinesInCenter.mockClear();

    act(() => fake.switchModel(3));

    expect(fake.decoratedLines()).toEqual([2, 3]);
    expect(fake.revealLinesInCenter).toHaveBeenLastCalledWith(2, 3);
  });

  it("unsubscribes from Monaco model events on unmount", () => {
    const fake = createModelSwitchingEditor(3);
    const { unmount } = renderHook(() =>
      useMonacoWalkthroughRange({
        editor: fake.editor,
        editorAreaRef: { current: null },
        path: WALKTHROUGH_FILE,
      }),
    );
    expect(fake.listenerCounts()).toEqual({ content: 1, model: 1 });

    unmount();

    expect(fake.listenerCounts()).toEqual({ content: 0, model: 0 });
  });
});
