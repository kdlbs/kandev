import { describe, it, expect } from "vitest";
import { multiSelectReducer, INITIAL_STATE } from "./use-task-multi-select";

const ordered = ["a", "b", "c", "d", "e"];

describe("multiSelectReducer repeated row occurrences", () => {
  it.each([
    { anchor: "b", anchorKey: "beta:b", target: "a", targetKey: "beta:a" },
    { anchor: "a", anchorKey: "beta:a", target: "b", targetKey: "beta:b" },
  ])(
    "ranges between the clicked occurrences from $anchorKey to $targetKey",
    ({ anchor, anchorKey, target, targetKey }) => {
      const state = multiSelectReducer(INITIAL_STATE, {
        type: "toggle_select",
        taskId: anchor,
        occurrenceId: anchorKey,
      });
      const next = multiSelectReducer(state, {
        type: "select_range",
        taskId: target,
        orderedIds: ["a", "x", "b", "a"],
        occurrence: { key: targetKey, orderedKeys: ["alpha:a", "alpha:x", "beta:b", "beta:a"] },
      });
      expect(next.selectedIds).toEqual(new Set(["a", "b"]));
      expect(next.anchorOccurrenceId).toBe(anchorKey);
    },
  );

  it("deduplicates canonical task IDs when a range spans repeated rows", () => {
    const state = multiSelectReducer(INITIAL_STATE, {
      type: "toggle_select",
      taskId: "a",
      occurrenceId: "alpha:a",
    });
    const next = multiSelectReducer(state, {
      type: "select_range",
      taskId: "a",
      orderedIds: ["a", "x", "b", "a"],
      occurrence: { key: "beta:a", orderedKeys: ["alpha:a", "alpha:x", "beta:b", "beta:a"] },
    });
    expect([...next.selectedIds]).toEqual(["a", "x", "b"]);
  });

  it("does not substitute another copy when the anchored occurrence disappears", () => {
    const state = multiSelectReducer(INITIAL_STATE, {
      type: "toggle_select",
      taskId: "a",
      occurrenceId: "beta:a",
    });
    const next = multiSelectReducer(state, {
      type: "select_range",
      taskId: "x",
      orderedIds: ["a", "middle", "x"],
      occurrence: { key: "alpha:x", orderedKeys: ["alpha:a", "alpha:middle", "alpha:x"] },
    });
    expect(next.selectedIds).toEqual(new Set(["a", "x"]));
    expect(next.anchorOccurrenceId).toBe("alpha:x");
  });
});

describe("multiSelectReducer anchor and range", () => {
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

  it("unions with the existing selection and re-anchors when the anchor is in another column", () => {
    const anchored = { ...INITIAL_STATE, selectedIds: new Set(["z"]), anchorId: "z" };
    const next = multiSelectReducer(anchored, {
      type: "select_range",
      taskId: "c",
      orderedIds: ordered,
    });
    expect(next.selectedIds).toEqual(new Set(["z", "c"]));
    expect(next.anchorId).toBe("c");
  });

  it("toggling off the only selected task clears the anchor", () => {
    const one = multiSelectReducer(INITIAL_STATE, { type: "toggle_select", taskId: "a" });
    expect(one.anchorId).toBe("a");
    const empty = multiSelectReducer(one, { type: "toggle_select", taskId: "a" });
    expect(empty.selectedIds.size).toBe(0);
    expect(empty.anchorId).toBeNull();
  });

  it("toggling off the anchor realigns it to a remaining selected task", () => {
    let s = multiSelectReducer(INITIAL_STATE, { type: "toggle_select", taskId: "a" });
    s = multiSelectReducer(s, { type: "toggle_select", taskId: "b" });
    // anchor is now 'b'; remove it and the anchor must move to the surviving 'a'.
    const next = multiSelectReducer(s, { type: "toggle_select", taskId: "b" });
    expect(next.selectedIds).toEqual(new Set(["a"]));
    expect(next.anchorId).toBe("a");
  });

  it("clearing the selection (set_selected empty) drops the anchor", () => {
    const dirty = { ...INITIAL_STATE, selectedIds: new Set(["a"]), anchorId: "a" };
    const next = multiSelectReducer(dirty, { type: "set_selected", ids: new Set<string>() });
    expect(next.anchorId).toBeNull();
  });

  it("set_selected keeps the anchor when it survives the replacement", () => {
    const state = { ...INITIAL_STATE, selectedIds: new Set(["a", "b", "c"]), anchorId: "b" };
    const next = multiSelectReducer(state, { type: "set_selected", ids: new Set(["b", "c"]) });
    expect(next.anchorId).toBe("b");
  });

  it("set_selected realigns the anchor when it is dropped (partial bulk failure)", () => {
    const state = { ...INITIAL_STATE, selectedIds: new Set(["a", "b", "c"]), anchorId: "a" };
    // 'a' archived away; only the failed ids remain — anchor must point at one.
    const next = multiSelectReducer(state, { type: "set_selected", ids: new Set(["b", "c"]) });
    expect(next.anchorId).not.toBeNull();
    expect(next.selectedIds.has(next.anchorId!)).toBe(true);
  });
});

describe("multiSelectReducer replacement occurrence anchors", () => {
  it.each(["deselect", "replace"])(
    "resolves the surviving task's occurrence after anchor %s",
    (mode) => {
      let state = multiSelectReducer(INITIAL_STATE, {
        type: "toggle_select",
        taskId: "a",
        occurrenceId: "alpha:a",
      });
      state = multiSelectReducer(state, {
        type: "toggle_select",
        taskId: "b",
        occurrenceId: "alpha:b",
      });
      state =
        mode === "deselect"
          ? multiSelectReducer(state, {
              type: "toggle_select",
              taskId: "b",
              occurrenceId: "alpha:b",
            })
          : multiSelectReducer(state, { type: "set_selected", ids: new Set(["a"]) });
      const next = multiSelectReducer(state, {
        type: "select_range",
        taskId: "d",
        orderedIds: ["a", "b", "c", "d"],
        occurrence: { key: "alpha:d", orderedKeys: ["alpha:a", "alpha:b", "alpha:c", "alpha:d"] },
      });
      expect(next.selectedIds).toEqual(new Set(["a", "b", "c", "d"]));
      expect(next.anchorOccurrenceId).toBe("alpha:a");
    },
  );
});
