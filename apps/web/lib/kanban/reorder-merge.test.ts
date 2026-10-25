import { describe, expect, it } from "vitest";
import { arraysEqual, mergeVisibleReorderIntoBand, moveWithinOrder } from "./reorder-merge";

describe("moveWithinOrder", () => {
  it("moves an item forward", () => {
    expect(moveWithinOrder(["a", "b", "c", "d"], 0, 2)).toEqual(["b", "c", "a", "d"]);
  });

  it("moves an item backward", () => {
    expect(moveWithinOrder(["a", "b", "c", "d"], 3, 1)).toEqual(["a", "d", "b", "c"]);
  });

  it("returns the same order when indexes are equal", () => {
    const order = ["a", "b", "c"];
    expect(moveWithinOrder(order, 1, 1)).toEqual(order);
  });

  it("returns the original array unchanged for an out-of-range index", () => {
    const order = ["a", "b", "c"];
    expect(moveWithinOrder(order, -1, 1)).toBe(order);
    expect(moveWithinOrder(order, 1, 3)).toBe(order);
  });
});

describe("mergeVisibleReorderIntoBand", () => {
  it("inserts the dragged task before its new visible neighbor, preserving hidden tasks", () => {
    // Full band: A, H1(hidden), B, C, H2(hidden), D. Visible: A, B, C, D.
    // Dragging C to the front of the visible order.
    const fullBandOrder = ["A", "H1", "B", "C", "H2", "D"];
    const visibleOrderAfterMove = ["C", "A", "B", "D"];
    expect(mergeVisibleReorderIntoBand(fullBandOrder, visibleOrderAfterMove, "C")).toEqual([
      "C",
      "A",
      "H1",
      "B",
      "H2",
      "D",
    ]);
  });

  it("inserts the dragged task after its new visible neighbor when moved to the visible end", () => {
    const fullBandOrder = ["A", "H1", "B", "C", "H2", "D"];
    const visibleOrderAfterMove = ["B", "C", "D", "A"];
    expect(mergeVisibleReorderIntoBand(fullBandOrder, visibleOrderAfterMove, "A")).toEqual([
      "H1",
      "B",
      "C",
      "H2",
      "D",
      "A",
    ]);
  });

  it("leaves every non-dragged task's relative order unchanged", () => {
    const fullBandOrder = ["A", "B", "C", "D", "E"];
    const visibleOrderAfterMove = ["A", "C", "B", "D", "E"];
    const result = mergeVisibleReorderIntoBand(fullBandOrder, visibleOrderAfterMove, "C");
    const others = result.filter((id) => id !== "C");
    expect(others).toEqual(["A", "B", "D", "E"]);
  });

  it("is a no-op when there is no hidden task and the visible order already matches", () => {
    const fullBandOrder = ["A", "B", "C"];
    const visibleOrderAfterMove = ["A", "B", "C"];
    expect(mergeVisibleReorderIntoBand(fullBandOrder, visibleOrderAfterMove, "B")).toEqual([
      "A",
      "B",
      "C",
    ]);
  });

  it("returns the original order when the dragged id is not in the visible order", () => {
    const fullBandOrder = ["A", "B", "C"];
    expect(mergeVisibleReorderIntoBand(fullBandOrder, ["A", "B"], "missing")).toEqual(
      fullBandOrder,
    );
  });

  it("moves the dragged task to the true end of the band when a hidden task trails the last visible member (AC.34)", () => {
    // Full band: v1, v2, h(hidden). Visible: v1, v2. Dragging v1 below v2 —
    // the last visible member — must land last in the WHOLE band, after h,
    // not merely after v2.
    const fullBandOrder = ["v1", "v2", "h"];
    const visibleOrderAfterMove = ["v2", "v1"];
    expect(mergeVisibleReorderIntoBand(fullBandOrder, visibleOrderAfterMove, "v1")).toEqual([
      "v2",
      "h",
      "v1",
    ]);
  });
});

describe("arraysEqual", () => {
  it("is true for identical sequences", () => {
    expect(arraysEqual(["a", "b"], ["a", "b"])).toBe(true);
  });

  it("is false for a different order", () => {
    expect(arraysEqual(["a", "b"], ["b", "a"])).toBe(false);
  });

  it("is false for a different length", () => {
    expect(arraysEqual(["a"], ["a", "b"])).toBe(false);
  });
});
