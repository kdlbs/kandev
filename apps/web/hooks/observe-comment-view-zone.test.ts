import { afterEach, expect, it, vi } from "vitest";
import type { editor as monacoEditor } from "monaco-editor";
import { observeCommentViewZone } from "./observe-comment-view-zone";

afterEach(() => vi.unstubAllGlobals());

it("grows the zone for the open attachment list and restores the original height when it closes", () => {
  let report!: ResizeObserverCallback;
  const observe = vi.fn();
  const disconnect = vi.fn();
  const layoutZone = vi.fn();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: ResizeObserverCallback) {
        report = callback;
      }
      observe = observe;
      disconnect = disconnect;
    },
  );
  const editor = {
    changeViewZones: (update: (accessor: { layoutZone: typeof layoutZone }) => void) =>
      update({ layoutZone }),
  } as unknown as monacoEditor.ICodeEditor;
  const content = document.createElement("div");
  const zone = { afterLineNumber: 1, heightInPx: 120, domNode: document.createElement("div") };

  const stop = observeCommentViewZone(editor, zone, "comment-zone", content);
  const resize = (height: number) =>
    report([{ contentRect: { height } } as ResizeObserverEntry], {} as ResizeObserver);

  expect(observe).toHaveBeenCalledWith(content);
  resize(245.2);
  expect(zone.heightInPx).toBe(246);
  expect(layoutZone).toHaveBeenCalledWith("comment-zone");
  resize(100);
  expect(zone.heightInPx).toBe(120);
  resize(100);
  expect(layoutZone).toHaveBeenCalledTimes(2);
  stop();
  expect(disconnect).toHaveBeenCalledOnce();
});
