import { describe, it, expect, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { computeNextSelection, useMultiSelect } from "./use-multi-select";

function makeRef(value: string | null = null) {
  return { current: value };
}

describe("computeNextSelection — ctrl/cmd click", () => {
  it("selects a single item", () => {
    const ref = makeRef();
    const result = computeNextSelection({
      prev: new Set(),
      path: "a",
      items: ["a", "b"],
      isShift: false,
      isCtrlOrMeta: true,
      lastClickedRef: ref,
    });
    expect(result).toEqual(new Set(["a"]));
    expect(ref.current).toBe("a");
  });

  it("toggles item out of selection", () => {
    const ref = makeRef("a");
    const result = computeNextSelection({
      prev: new Set(["a"]),
      path: "a",
      items: ["a", "b"],
      isShift: false,
      isCtrlOrMeta: true,
      lastClickedRef: ref,
    });
    expect(result.size).toBe(0);
  });

  it("accumulates selections", () => {
    const ref = makeRef("a");
    const result = computeNextSelection({
      prev: new Set(["a"]),
      path: "c",
      items: ["a", "b", "c"],
      isShift: false,
      isCtrlOrMeta: true,
      lastClickedRef: ref,
    });
    expect(result).toEqual(new Set(["a", "c"]));
  });
});

describe("computeNextSelection — shift click", () => {
  it("selects range from anchor", () => {
    const ref = makeRef("b");
    const result = computeNextSelection({
      prev: new Set(),
      path: "d",
      items: ["a", "b", "c", "d", "e"],
      isShift: true,
      isCtrlOrMeta: false,
      lastClickedRef: ref,
    });
    expect(result).toEqual(new Set(["b", "c", "d"]));
  });

  it("selects single item when no anchor exists", () => {
    const ref = makeRef();
    const result = computeNextSelection({
      prev: new Set(),
      path: "b",
      items: ["a", "b", "c"],
      isShift: true,
      isCtrlOrMeta: false,
      lastClickedRef: ref,
    });
    expect(result).toEqual(new Set(["b"]));
    expect(ref.current).toBe("b");
  });

  it("resets to single item when anchor is stale", () => {
    const ref = makeRef("x"); // not in items
    const result = computeNextSelection({
      prev: new Set(),
      path: "c",
      items: ["a", "b", "c"],
      isShift: true,
      isCtrlOrMeta: false,
      lastClickedRef: ref,
    });
    expect(result).toEqual(new Set(["c"]));
    expect(ref.current).toBe("c");
  });

  it("extends existing selection with shift+ctrl", () => {
    const ref = makeRef("b");
    const result = computeNextSelection({
      prev: new Set(["x"]),
      path: "d",
      items: ["a", "b", "c", "d", "e"],
      isShift: true,
      isCtrlOrMeta: true,
      lastClickedRef: ref,
    });
    expect(result).toEqual(new Set(["x", "b", "c", "d"]));
  });

  it("selects reverse range", () => {
    const ref = makeRef("d");
    const result = computeNextSelection({
      prev: new Set(),
      path: "b",
      items: ["a", "b", "c", "d", "e"],
      isShift: true,
      isCtrlOrMeta: false,
      lastClickedRef: ref,
    });
    expect(result).toEqual(new Set(["b", "c", "d"]));
  });
});

// --- Hook-level tests (renderHook) ---

function mouseEvent(overrides: Partial<React.MouseEvent> = {}): React.MouseEvent {
  return {
    ctrlKey: false,
    metaKey: false,
    shiftKey: false,
    button: 0,
    ...overrides,
  } as React.MouseEvent;
}

describe("useMultiSelect — stale-path pruning", () => {
  it("prunes selected paths when items shrink", () => {
    const { result, rerender } = renderHook(({ items }) => useMultiSelect({ items }), {
      initialProps: { items: ["a", "b", "c"] },
    });
    act(() => result.current.selectAll());
    expect(result.current.selectedPaths).toEqual(new Set(["a", "b", "c"]));

    rerender({ items: ["a", "c"] });
    expect(result.current.selectedPaths).toEqual(new Set(["a", "c"]));
  });

  it("returns empty set when all items removed", () => {
    const { result, rerender } = renderHook(({ items }) => useMultiSelect({ items }), {
      initialProps: { items: ["a", "b"] },
    });
    act(() => result.current.selectAll());
    rerender({ items: [] });
    expect(result.current.selectedPaths.size).toBe(0);
  });
});

describe("useMultiSelect — plain click", () => {
  it("returns false and does not select", () => {
    const { result } = renderHook(() => useMultiSelect({ items: ["a", "b"] }));
    let consumed: boolean;
    act(() => {
      consumed = result.current.handleClick("a", mouseEvent());
    });
    expect(consumed!).toBe(false);
    expect(result.current.selectedPaths.size).toBe(0);
  });

  it("clears existing selection", () => {
    const { result } = renderHook(() => useMultiSelect({ items: ["a", "b"] }));
    act(() => result.current.handleClick("a", mouseEvent({ ctrlKey: true })));
    expect(result.current.selectedPaths.size).toBe(1);

    act(() => result.current.handleClick("b", mouseEvent()));
    expect(result.current.selectedPaths.size).toBe(0);
  });
});

describe("useMultiSelect — selectAll / clearSelection", () => {
  it("selectAll selects all items", () => {
    const { result } = renderHook(() => useMultiSelect({ items: ["a", "b", "c"] }));
    act(() => result.current.selectAll());
    expect(result.current.selectedPaths).toEqual(new Set(["a", "b", "c"]));
  });

  it("clearSelection empties selection", () => {
    const { result } = renderHook(() => useMultiSelect({ items: ["a", "b"] }));
    act(() => result.current.selectAll());
    act(() => result.current.clearSelection());
    expect(result.current.selectedPaths.size).toBe(0);
  });

  it("onSelectionChange fires on selectAll", () => {
    const onChange = vi.fn();
    const { result } = renderHook(() =>
      useMultiSelect({ items: ["a", "b"], onSelectionChange: onChange }),
    );
    act(() => result.current.selectAll());
    expect(onChange).toHaveBeenCalledWith(new Set(["a", "b"]));
  });
});
