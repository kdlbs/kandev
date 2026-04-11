import { describe, it, expect } from "vitest";
import { computeNextSelection } from "./use-multi-select";

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
