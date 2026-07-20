import { act, renderHook } from "@testing-library/react";
import type { editor as monacoEditor } from "monaco-editor";
import { describe, expect, it, vi } from "vitest";

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

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof hookState) => unknown) => selector(hookState),
}));

vi.mock("@/lib/walkthrough-open-state", () => ({
  useIsWalkthroughOpenForTask: () => true,
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

    act(() => fake.switchModel(3));

    expect(fake.decoratedLines()).toEqual([2, 3]);
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
