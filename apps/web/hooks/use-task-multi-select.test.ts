import { describe, it, expect } from "vitest";
import {
  multiSelectReducer,
  INITIAL_STATE,
  buildPipelineStepIndexOf,
} from "./use-task-multi-select";

describe("multiSelectReducer", () => {
  it("reset returns initial state", () => {
    const dirty = {
      selectedIds: new Set(["a", "b"]),
      isMultiSelectEnabled: true,
      isDeleting: true,
      isArchiving: false,
      anchorId: "a",
    };
    expect(multiSelectReducer(dirty, { type: "reset" })).toBe(INITIAL_STATE);
  });

  it("toggle_select adds a task", () => {
    const next = multiSelectReducer(INITIAL_STATE, { type: "toggle_select", taskId: "t1" });
    expect(next.selectedIds).toEqual(new Set(["t1"]));
  });

  it("toggle_select removes an already-selected task", () => {
    const state = { ...INITIAL_STATE, selectedIds: new Set(["t1", "t2"]) };
    const next = multiSelectReducer(state, { type: "toggle_select", taskId: "t1" });
    expect(next.selectedIds).toEqual(new Set(["t2"]));
  });

  it("set_selected replaces the selection set", () => {
    const state = { ...INITIAL_STATE, selectedIds: new Set(["old"]) };
    const next = multiSelectReducer(state, { type: "set_selected", ids: new Set(["a", "b"]) });
    expect(next.selectedIds).toEqual(new Set(["a", "b"]));
  });

  it("set_enabled controls isMultiSelectEnabled", () => {
    const on = multiSelectReducer(INITIAL_STATE, { type: "set_enabled", value: true });
    expect(on.isMultiSelectEnabled).toBe(true);
    const off = multiSelectReducer(on, { type: "set_enabled", value: false });
    expect(off.isMultiSelectEnabled).toBe(false);
  });

  describe("bulk operation state flags", () => {
    it("set_deleting toggles isDeleting", () => {
      const on = multiSelectReducer(INITIAL_STATE, { type: "set_deleting", value: true });
      expect(on.isDeleting).toBe(true);
      const off = multiSelectReducer(on, { type: "set_deleting", value: false });
      expect(off.isDeleting).toBe(false);
    });

    it("set_archiving toggles isArchiving", () => {
      const on = multiSelectReducer(INITIAL_STATE, { type: "set_archiving", value: true });
      expect(on.isArchiving).toBe(true);
      const off = multiSelectReducer(on, { type: "set_archiving", value: false });
      expect(off.isArchiving).toBe(false);
    });
  });

  describe("bulk action scenarios (reducer-level)", () => {
    it("all succeed: selectedIds empty + enabled false", () => {
      const state = {
        ...INITIAL_STATE,
        selectedIds: new Set(["t1", "t2"]),
        isMultiSelectEnabled: true,
      };
      // Simulate: set_selected with empty failed set, then set_enabled false
      const afterSelect = multiSelectReducer(state, {
        type: "set_selected",
        ids: new Set<string>(),
      });
      const afterDisable = multiSelectReducer(afterSelect, { type: "set_enabled", value: false });
      expect(afterDisable.selectedIds.size).toBe(0);
      expect(afterDisable.isMultiSelectEnabled).toBe(false);
    });

    it("some fail: selectedIds contains failed IDs, enabled stays true", () => {
      const state = {
        ...INITIAL_STATE,
        selectedIds: new Set(["t1", "t2", "t3"]),
        isMultiSelectEnabled: true,
      };
      // Simulate: set_selected with failed IDs only, no set_enabled call
      const afterSelect = multiSelectReducer(state, {
        type: "set_selected",
        ids: new Set(["t2"]),
      });
      expect(afterSelect.selectedIds).toEqual(new Set(["t2"]));
      expect(afterSelect.isMultiSelectEnabled).toBe(true);
    });

    it("all fail: selectedIds unchanged, enabled stays true", () => {
      const state = {
        ...INITIAL_STATE,
        selectedIds: new Set(["t1", "t2"]),
        isMultiSelectEnabled: true,
      };
      const afterSelect = multiSelectReducer(state, {
        type: "set_selected",
        ids: new Set(["t1", "t2"]),
      });
      expect(afterSelect.selectedIds).toEqual(new Set(["t1", "t2"]));
      expect(afterSelect.isMultiSelectEnabled).toBe(true);
    });
  });
});

describe("buildPipelineStepIndexOf", () => {
  const snapshots = {
    "wf-1": {
      steps: [
        { id: "todo", position: 0 },
        { id: "review", position: 1 },
        { id: "done", position: 2 },
      ],
    },
  };

  it("indexes steps by their displayed (position) order", () => {
    const indexOf = buildPipelineStepIndexOf(snapshots, {});
    expect(indexOf("todo")).toBe(0);
    expect(indexOf("review")).toBe(1);
    expect(indexOf("done")).toBe(2);
  });

  it("skips a hidden step when indexing the displayed order", () => {
    const indexOf = buildPipelineStepIndexOf(snapshots, { "wf-1": ["review"] });
    expect(indexOf("todo")).toBe(0);
    expect(indexOf("done")).toBe(1);
    expect(indexOf("review")).toBe(Infinity);
  });

  it("sorts an unknown or undefined step id last", () => {
    const indexOf = buildPipelineStepIndexOf(snapshots, {});
    expect(indexOf("unknown-step")).toBe(Infinity);
    expect(indexOf(undefined)).toBe(Infinity);
  });
});
