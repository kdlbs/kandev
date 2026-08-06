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
  let measuredWidths = [...widths];
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
  widths.forEach((_, index) => {
    const cell = row.insertCell();
    Object.defineProperty(cell, "getBoundingClientRect", {
      value: () => {
        const left = 10 + measuredWidths.slice(0, index).reduce((sum, width) => sum + width, 0);
        return rect(left, 20, measuredWidths[index] ?? 0, 30);
      },
    });
  });
  return {
    row,
    setMeasuredWidths: (next: number[]) => {
      measuredWidths = next;
    },
    table,
    wrapper,
  };
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

  it("remeasures separator geometry when controlled widths change", () => {
    const { result, setMeasuredWidths } = renderResizeHook();
    expect(result.current.geometry?.boundaries[0]).toBe(120);

    setMeasuredWidths([140, 160]);
    act(() => result.current.resizeWithKeyboard(0, keyboardEvent("ArrowRight") as never));

    expect(result.current.geometry?.boundaries[0]).toBe(140);
  });

  it("clears custom widths when resizing becomes disabled", () => {
    const { result, rerender } = renderResizeHook();

    act(() => result.current.resizeWithKeyboard(0, keyboardEvent("ArrowRight") as never));
    expect(result.current.columnWidths).not.toBeNull();

    rerender({ enabled: false });
    expect(result.current.columnWidths).toBeNull();
    expect(result.current.fixedTableWidth).toBeNull();
  });

  it("clears an active drag and document resize state when resizing becomes disabled", () => {
    const { result, rerender } = renderResizeHook();
    const pointer = {
      button: 0,
      clientX: 130,
      currentTarget: { setPointerCapture: vi.fn() },
      pointerId: 7,
      preventDefault: vi.fn(),
    };

    act(() => result.current.startResize(0, pointer as never));
    expect(result.current.activeBoundary).toBe(0);
    expect(document.body.style.cursor).toBe("col-resize");
    expect(document.body.style.userSelect).toBe("none");

    rerender({ enabled: false });
    expect(result.current.activeBoundary).toBeNull();
    expect(document.body.style.cursor).toBe("");
    expect(document.body.style.userSelect).toBe("");
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
