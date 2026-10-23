import { act, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { EditorModel, StringValue, type EditorView } from "@vscode/markdown-editor";
import type { SourcePositionHandle } from "../source-position";
import { useHybridSourcePosition } from "./use-hybrid-source-position";

afterEach(() => vi.unstubAllGlobals());
it("preserves the requested reading position when modes change before the deferred reveal", () => {
  const frames = new Map<number, FrameRequestCallback>();
  let frameId = 0;
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    frames.set(++frameId, callback);
    return frameId;
  });
  vi.stubGlobal("cancelAnimationFrame", (id: number) => frames.delete(id));
  const content = "first\nsecond\nthird\nfourth";
  const model = new EditorModel();
  model.sourceText.set(new StringValue(content), undefined, undefined);
  let visibleOffset = 0;
  const view = {
    focus: vi.fn(),
    measuredLayout: {
      visualLineMap: { get: () => ({ isEmpty: false, offsetAtPoint: () => visibleOffset }) },
    },
    coordinateSpace: { capture: () => ({ toLocalPoint: (point: unknown) => point }) },
    revealRangeInCenterIfOutsideViewport: (range: { start: number }) => {
      const id = requestAnimationFrame(() => {
        visibleOffset = range.start;
      });
      return { dispose: () => cancelAnimationFrame(id) };
    },
  } as unknown as EditorView;
  const rootRef = { current: document.createElement("div") };
  let handle: SourcePositionHandle | null = null;
  const { unmount } = renderHook(() =>
    useHybridSourcePosition({
      lifecycle: { active: true, model, view },
      rootRef,
      content,
      onSourcePositionHandle: (next) => {
        handle = next;
      },
      onError: vi.fn(),
    }),
  );
  act(() => handle!.restore({ selection: { anchor: 0, active: 0 }, nearestLine: 3 }));
  expect(handle!.capture()).toEqual({ selection: { anchor: 0, active: 0 }, nearestLine: 3 });
  act(() => {
    for (const callback of [...frames.values()]) callback(0);
    frames.clear();
  });
  visibleOffset = content.indexOf("fourth");
  expect(handle!.capture()?.nearestLine).toBe(4);
  act(() => handle!.restore({ selection: { anchor: 0, active: 0 }, nearestLine: 3 }));
  expect(handle!.capture()?.nearestLine).toBe(3);
  visibleOffset = content.indexOf("second");
  rootRef.current.dispatchEvent(new Event("scroll"));
  expect(handle!.capture()?.nearestLine).toBe(2);
  expect(frames.size).toBe(0);
  act(() => handle!.restore({ selection: { anchor: 0, active: 0 }, nearestLine: 3 }));
  expect(frames.size).toBe(2);
  unmount();
  expect(frames.size).toBe(0);
});
