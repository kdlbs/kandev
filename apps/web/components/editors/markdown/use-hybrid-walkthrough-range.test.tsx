import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  EditorModel,
  LocalHistoryStrategy,
  OffsetRange,
  Rect2D,
  Selection,
  StringEdit,
  StringValue,
  type EditorView,
} from "@vscode/markdown-editor";
import {
  hybridWalkthroughOffsets,
  useHybridWalkthroughRange,
} from "./use-hybrid-walkthrough-range";
import type { MarkdownWalkthroughTarget } from "@/lib/markdown/walkthrough-range";
import {
  getWalkthroughEditorAnchor,
  setWalkthroughEditorAnchor,
} from "@/lib/walkthrough-editor-anchor";
const LF_SOURCE = "one\ntwo\nthree";
const target: MarkdownWalkthroughTarget = {
  key: "target",
  taskId: "task",
  path: "guide.md",
  file: "guide.md",
  stepIndex: 0,
  startLine: 2,
  endLine: 3,
};
beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
});
afterEach(() => {
  cleanup();
  document.body.replaceChildren();
  setWalkthroughEditorAnchor(null);
  vi.useRealTimers();
  vi.unstubAllGlobals();
});
function setup(content: string) {
  const model = new EditorModel();
  model.sourceText.set(new StringValue(content), undefined, undefined);
  const root = document.createElement("div");
  document.body.append(root);
  vi.spyOn(root, "getBoundingClientRect").mockReturnValue(new DOMRect(10, 20, 400, 300));
  Object.defineProperty(document, "elementFromPoint", { configurable: true, value: () => root });
  let rects = [
    { x: 20, y: 50, width: 100, height: 20 },
    { x: 20, y: 80, width: 130, height: 20 },
  ];
  const observers = new Set<{ endUpdate: () => void }>();
  const geometry = {
    get: () => rects,
    addObserver: (observer: { endUpdate: () => void }) => observers.add(observer),
    removeObserver: (observer: { endUpdate: () => void }) => observers.delete(observer),
  };
  const dispose = vi.fn();
  let measurements = [
    {
      absoluteStart: 0,
      block: { length: content.length, kind: "paragraph" },
      rect: Rect2D.fromPointSize(20, 35, 200, 120),
    },
  ];
  const view = {
    overlayContainer: root,
    rangeRects: vi.fn(() => geometry),
    measuredLayout: { measurements: { get: () => measurements } },
    coordinateSpace: {
      capture: () => ({
        toLocalPoint: (point: { x: number; y: number }) => ({ x: point.x - 10, y: point.y - 20 }),
        toClientRect: (rect: Rect2D) => rect.translate(10, 20),
      }),
    },
    revealRangeInCenterIfOutsideViewport: vi.fn(() => ({ dispose })),
  };
  const lifecycle = { active: true, model, view: view as unknown as EditorView };
  return {
    root,
    model,
    view,
    lifecycle,
    observers,
    dispose,
    setMeasurements: (next: typeof measurements) => {
      measurements = next;
    },
    setRects: (next: typeof rects) => {
      rects = next;
      for (const observer of observers) observer.endUpdate();
    },
  };
}
function frame() {
  act(() => vi.advanceTimersByTime(20));
}
// @covers AC-UI-MARKDOWN-FILE-EDITING-002.17
it("maps inclusive LF, CRLF, and BOM ranges without changing source offsets", () => {
  expect(hybridWalkthroughOffsets(LF_SOURCE, 2, 3)).toEqual(new OffsetRange(4, 13));
  expect(hybridWalkthroughOffsets("\ufeffone\r\ntwo\r\nthree", 2, 3)).toEqual(
    new OffsetRange(6, 16),
  );
  expect(hybridWalkthroughOffsets("one\n", 8, 20)).toEqual(new OffsetRange(4, 4));
});
it("uses public range geometry reactively and preserves real model selection/source/history", () => {
  const source = "\ufeffone\r\ntwo\r\nthree";
  const fixture = setup(source);
  const { model } = fixture;
  const selection = new Selection(9, 6);
  model.selection.set(selection, undefined, undefined);
  const history = new LocalHistoryStrategy(model);
  const edit = StringEdit.replace(new OffsetRange(source.length, source.length), "!");
  history.record(() => model.applyEdit(edit, selection), edit);
  const content = model.sourceText.get().value;
  const { unmount } = renderHook(() =>
    useHybridWalkthroughRange({
      lifecycle: fixture.lifecycle,
      rootRef: { current: fixture.root },
      content,
      target,
      isActive: true,
    }),
  );
  frame();
  expect(
    fixture.root.querySelector<HTMLElement>("[data-testid='walkthrough-editor-range']")?.style
      .height,
  ).toBe("50px");
  expect(fixture.view.rangeRects).toHaveBeenCalledWith(new OffsetRange(6, 17));
  expect(model.sourceText.get().value).toBe(`${source}!`);
  expect(model.selection.get()).toEqual(selection);
  act(() => fixture.setRects([{ x: 20, y: 65, width: 100, height: 80 }]));
  frame();
  expect(getWalkthroughEditorAnchor()?.rect.height).toBe(80);
  expect(fixture.view.revealRangeInCenterIfOutsideViewport).toHaveBeenCalledOnce();
  unmount();
  expect(fixture.observers.size).toBe(0);
  expect(fixture.dispose).toHaveBeenCalledOnce();
  history.undo();
  expect(model.sourceText.get().value).toBe(source);
});
it("uses nearest same-document measured block when source syntax has no selection rectangles", () => {
  const fixture = setup(LF_SOURCE);
  fixture.setRects([]);
  renderHook(() =>
    useHybridWalkthroughRange({
      lifecycle: fixture.lifecycle,
      rootRef: { current: fixture.root },
      content: LF_SOURCE,
      target,
      isActive: true,
    }),
  );
  frame();
  expect(getWalkthroughEditorAnchor()?.rect.height).toBe(120);
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.17
it("excludes preceding source blocks and selection gap geometry from a rendered section", () => {
  const content = LF_SOURCE;
  const fixture = setup(content);
  fixture.setMeasurements([
    {
      absoluteStart: 0,
      block: { length: 4, kind: "paragraph" },
      rect: Rect2D.fromPointSize(20, 20, 200, 25),
    },
    {
      absoluteStart: 4,
      block: { length: 9, kind: "paragraph" },
      rect: Rect2D.fromPointSize(20, 80, 200, 70),
    },
  ]);
  fixture.setRects([
    { x: 20, y: 20, width: 100, height: 25 },
    { x: 20, y: 45, width: 100, height: 35 },
    { x: 20, y: 80, width: 100, height: 20 },
    { x: 20, y: 130, width: 100, height: 20 },
    { x: 20, y: 150, width: 100, height: 35 },
  ]);
  renderHook(() =>
    useHybridWalkthroughRange({
      lifecycle: fixture.lifecycle,
      rootRef: { current: fixture.root },
      content,
      target,
      isActive: true,
    }),
  );
  frame();
  expect(getWalkthroughEditorAnchor()?.rect.top).toBe(100);
  expect(getWalkthroughEditorAnchor()?.rect.bottom).toBe(170);
});
