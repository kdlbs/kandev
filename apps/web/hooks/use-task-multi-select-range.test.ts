import { describe, it, expect } from "vitest";
import { multiSelectReducer, INITIAL_STATE } from "./use-task-multi-select";

describe("multiSelectReducer — anchor + range", () => {
  const ordered = ["a", "b", "c", "d", "e"];

  it("toggle_select sets the range anchor to the toggled task", () => {
    const next = multiSelectReducer(INITIAL_STATE, { type: "toggle_select", taskId: "t1" });
    expect(next.anchorId).toBe("t1");
  });

  it("select_range with no anchor selects only the clicked task and anchors it", () => {
    const next = multiSelectReducer(INITIAL_STATE, {
      type: "select_range",
      taskId: "c",
      orderedIds: ordered,
    });
    expect(next.selectedIds).toEqual(new Set(["c"]));
    expect(next.anchorId).toBe("c");
  });

  it("selects the inclusive forward range from the anchor", () => {
    const anchored = multiSelectReducer(INITIAL_STATE, { type: "toggle_select", taskId: "b" });
    const next = multiSelectReducer(anchored, {
      type: "select_range",
      taskId: "d",
      orderedIds: ordered,
    });
    expect(next.selectedIds).toEqual(new Set(["b", "c", "d"]));
    expect(next.anchorId).toBe("b");
  });

  it("selects the inclusive backward range from the anchor", () => {
    const anchored = multiSelectReducer(INITIAL_STATE, { type: "toggle_select", taskId: "d" });
    const next = multiSelectReducer(anchored, {
      type: "select_range",
      taskId: "b",
      orderedIds: ordered,
    });
    expect(next.selectedIds).toEqual(new Set(["b", "c", "d"]));
  });

  it("unions the range with the existing selection", () => {
    const withPrev = { ...INITIAL_STATE, selectedIds: new Set(["x"]), anchorId: "a" };
    const next = multiSelectReducer(withPrev, {
      type: "select_range",
      taskId: "c",
      orderedIds: ordered,
    });
    expect(next.selectedIds).toEqual(new Set(["x", "a", "b", "c"]));
  });

  it("falls back to single select when the anchor is in another column", () => {
    const anchored = { ...INITIAL_STATE, selectedIds: new Set(["z"]), anchorId: "z" };
    const next = multiSelectReducer(anchored, {
      type: "select_range",
      taskId: "c",
      orderedIds: ordered,
    });
    expect(next.selectedIds).toEqual(new Set(["z", "c"]));
    expect(next.anchorId).toBe("c");
  });

  it("clearing the selection (set_selected empty) drops the anchor", () => {
    const dirty = { ...INITIAL_STATE, selectedIds: new Set(["a"]), anchorId: "a" };
    const next = multiSelectReducer(dirty, { type: "set_selected", ids: new Set<string>() });
    expect(next.anchorId).toBeNull();
  });
});
