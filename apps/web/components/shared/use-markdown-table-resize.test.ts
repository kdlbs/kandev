import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useMarkdownTableResize } from "./use-markdown-table-resize";

let notifyMutation: MutationCallback;

class ResizeObserverStub {
  disconnect = vi.fn();
  observe = vi.fn();
}

class MutationObserverStub {
  disconnect = vi.fn();
  observe = vi.fn();

  constructor(callback: MutationCallback) {
    notifyMutation = callback;
  }
}

function rect(left: number, top: number, width: number, height: number): DOMRect {
  return {
    bottom: top + height,
    height,
    left,
    right: left + width,
    top,
    width,
    x: left,
    y: top,
    toJSON: () => ({}),
  };
}

function createTableGeometry(widths: number[]) {
  const wrapper = document.createElement("div");
  const table = document.createElement("table");
  const row = table.insertRow();
  wrapper.append(table);
  document.body.append(wrapper);

  Object.defineProperty(wrapper, "getBoundingClientRect", {
    value: () => rect(10, 20, 300, 100),
  });
  Object.defineProperty(table, "getBoundingClientRect", {
    value: () => rect(10, 20, 300, 100),
  });
  let left = 10;
  for (const width of widths) {
    const cell = row.insertCell();
    const cellLeft = left;
    Object.defineProperty(cell, "getBoundingClientRect", {
      value: () => rect(cellLeft, 20, width, 30),
    });
    left += width;
  }
  return { row, table, wrapper };
}

function renderResizeHook() {
  const rendered = renderHook(({ enabled }) => useMarkdownTableResize(enabled), {
    initialProps: { enabled: false },
  });
  const elements = createTableGeometry([120, 180]);
  act(() => {
    rendered.result.current.tableRef.current = elements.table;
    rendered.result.current.wrapperRef.current = elements.wrapper;
  });
  rendered.rerender({ enabled: true });
  return { ...rendered, ...elements };
}

function keyboardEvent(key: string) {
  return { key, preventDefault: vi.fn() };
}

beforeEach(() => {
  vi.stubGlobal("ResizeObserver", ResizeObserverStub);
  vi.stubGlobal("MutationObserver", MutationObserverStub);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("useMarkdownTableResize", () => {
  it("resizes with arrow keys and resets with Enter", () => {
    const { result } = renderResizeHook();
    const right = keyboardEvent("ArrowRight");
    const left = keyboardEvent("ArrowLeft");
    const enter = keyboardEvent("Enter");

    act(() => result.current.resizeWithKeyboard(0, right as never));
    expect(right.preventDefault).toHaveBeenCalledOnce();
    expect(result.current.columnWidths).toEqual([128, 172]);
    expect(result.current.fixedTableWidth).toBe(300);

    act(() => result.current.resizeWithKeyboard(0, left as never));
    expect(left.preventDefault).toHaveBeenCalledOnce();
    expect(result.current.columnWidths).toEqual([120, 180]);

    act(() => result.current.resizeWithKeyboard(0, enter as never));
    expect(enter.preventDefault).toHaveBeenCalledOnce();
    expect(result.current.columnWidths).toBeNull();
    expect(result.current.fixedTableWidth).toBeNull();
  });

  it("clears custom widths when resizing becomes disabled", () => {
    const { result, rerender } = renderResizeHook();

    act(() => result.current.resizeWithKeyboard(0, keyboardEvent("ArrowRight") as never));
    expect(result.current.columnWidths).not.toBeNull();

    rerender({ enabled: false });
    expect(result.current.columnWidths).toBeNull();
    expect(result.current.fixedTableWidth).toBeNull();
  });

  it("clears custom widths when the table column count changes", () => {
    const { result, row } = renderResizeHook();
    act(() => result.current.resizeWithKeyboard(0, keyboardEvent("ArrowRight") as never));

    row.insertCell();
    act(() => notifyMutation([], {} as MutationObserver));

    expect(result.current.columnWidths).toBeNull();
    expect(result.current.fixedTableWidth).toBeNull();
  });
});
