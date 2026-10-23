import { describe, expect, it } from "vitest";
import { markdownPreviewRangeElements } from "./use-markdown-preview-walkthrough-range";

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.16
// @covers AC-UI-MARKDOWN-FILE-EDITING-002.17
describe("Markdown Preview walkthrough mapping", () => {
  it("chooses the smallest nested source blocks and spans intervening sections", () => {
    const root = document.createElement("div");
    root.innerHTML = `<blockquote data-md-source-start="1" data-md-source-end="12">
      <p data-md-source-start="2" data-md-source-end="3">First</p>
      <ul><li data-md-source-start="5" data-md-source-end="9">
        <p data-md-source-start="5" data-md-source-end="6">Second</p>
        <p data-md-source-start="8" data-md-source-end="9">Third</p>
      </li></ul></blockquote>`;
    expect(
      markdownPreviewRangeElements(root, { startLine: 2, endLine: 9 }).map((el) =>
        el.textContent?.trim(),
      ),
    ).toEqual(["First", "Second", "Third"]);
  });
  it("keeps a list item's own first line when only its nested item has another mapping", () => {
    const root = document.createElement("div");
    root.innerHTML = `<ul><li data-md-source-start="2" data-md-source-end="10">Parent text<ul><li data-md-source-start="6" data-md-source-end="10">Nested text</li></ul></li></ul>`;
    expect(
      markdownPreviewRangeElements(root, { startLine: 2, endLine: 2 })[0]?.getAttribute(
        "data-md-source-start",
      ),
    ).toBe("2");
    expect(
      markdownPreviewRangeElements(root, { startLine: 6, endLine: 6 })[0]?.getAttribute(
        "data-md-source-start",
      ),
    ).toBe("6");
  });
  it("maps blank lines to the nearest mapped block without a distant ancestor", () => {
    const root = document.createElement("div");
    root.innerHTML = `<blockquote data-md-source-start="1" data-md-source-end="12"><p data-md-source-start="2" data-md-source-end="3">First</p><p data-md-source-start="8" data-md-source-end="9">Last</p></blockquote>`;
    expect(
      markdownPreviewRangeElements(root, { startLine: 6, endLine: 6 }).map((el) => el.textContent),
    ).toEqual(["Last"]);
    expect(
      markdownPreviewRangeElements(document.createElement("div"), { startLine: 1, endLine: 2 }),
    ).toEqual([]);
  });
});

import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, vi } from "vitest";
import { useMarkdownPreviewWalkthroughRange } from "./use-markdown-preview-walkthrough-range";
import {
  getWalkthroughEditorAnchor,
  setWalkthroughEditorAnchor,
} from "@/lib/walkthrough-editor-anchor";
import type { MarkdownWalkthroughTarget } from "@/lib/markdown/walkthrough-range";
let notifyLayout: () => void;
beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: () => void) {
        notifyLayout = callback;
      }
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
// @covers AC-UI-MARKDOWN-FILE-EDITING-002.16
it("reveals only the active target and follows source and asynchronous block reflow without recentering", () => {
  const scroll = document.createElement("div");
  const root = document.createElement("div");
  scroll.append(root);
  document.body.append(scroll);
  root.innerHTML = `<p data-md-source-start="1" data-md-source-end="2">First</p><table data-md-source-start="7" data-md-source-end="10"></table>`;
  const viewport = { left: 10, top: 50, width: 400, height: 200, right: 410, bottom: 250 };
  vi.spyOn(scroll, "getBoundingClientRect").mockReturnValue(viewport as DOMRect);
  Object.defineProperty(document, "elementFromPoint", { configurable: true, value: () => root });
  let tableHeight = 80;
  vi.spyOn(root.children[0], "getBoundingClientRect").mockImplementation(
    () =>
      ({
        ...viewport,
        top: 70 - scroll.scrollTop,
        bottom: 100 - scroll.scrollTop,
        height: 30,
      }) as DOMRect,
  );
  vi.spyOn(root.children[1], "getBoundingClientRect").mockImplementation(
    () =>
      ({
        ...viewport,
        top: 500 - scroll.scrollTop,
        bottom: 500 + tableHeight - scroll.scrollTop,
        height: tableHeight,
      }) as DOMRect,
  );
  const target: MarkdownWalkthroughTarget = {
    key: "one",
    path: "guide.md",
    file: "guide.md",
    taskId: "task",
    stepIndex: 0,
    startLine: 7,
    endLine: 10,
  };
  const refs = { rootRef: { current: root }, scrollRef: { current: scroll } };
  const { rerender } = renderHook(
    ({ content, isActive, currentTarget }) =>
      useMarkdownPreviewWalkthroughRange({ ...refs, content, isActive, target: currentTarget }),
    { initialProps: { content: "first", isActive: true, currentTarget: target } },
  );
  act(() => vi.advanceTimersByTime(20));
  expect(scroll.scrollTop).toBe(390);
  expect(
    scroll.querySelector<HTMLElement>("[data-testid='walkthrough-editor-range']")?.style.top,
  ).toBe("450px");
  expect(getWalkthroughEditorAnchor()?.rect.height).toBe(80);
  scroll.scrollTop = 25;
  tableHeight = 130;
  act(() => notifyLayout());
  act(() => vi.advanceTimersByTime(20));
  expect(getWalkthroughEditorAnchor()?.rect.height).toBe(130);
  expect(scroll.scrollTop).toBe(25);
  root.children[0].setAttribute("data-md-source-end", "10");
  rerender({
    content: "replaced",
    isActive: true,
    currentTarget: { ...target, key: "two", startLine: 1 },
  });
  act(() => vi.advanceTimersByTime(20));
  expect(getWalkthroughEditorAnchor()?.rect.height).toBe(560);
  rerender({ content: "replaced", isActive: false, currentTarget: target });
  act(() => vi.advanceTimersByTime(200));
  expect(scroll.querySelector("[data-testid='walkthrough-editor-range']")).toBeNull();
  expect(getWalkthroughEditorAnchor()).toBeNull();
});
