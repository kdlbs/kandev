import { describe, expect, it } from "vitest";
import {
  parseThreadMaxColumns,
  threadTaskScopeModeUpdate,
  updateThreadTaskSelection,
  updateVisibleThreadTaskSelection,
} from "./threads-view-editor-utils";

describe("parseThreadMaxColumns", () => {
  it.each([
    ["", null],
    ["1", 1],
    ["30", 30],
  ])("parses %j as %j", (input, expected) => {
    expect(parseThreadMaxColumns(input)).toBe(expected);
  });

  it.each(["-", "abc", "2.5", "0", "31", "Infinity", "1e2"])(
    "rejects invalid value %j",
    (input) => {
      expect(parseThreadMaxColumns(input)).toBeUndefined();
    },
  );

  it("rejects the empty DOM value produced by a transient number-input edit", () => {
    expect(parseThreadMaxColumns("", true)).toBeUndefined();
  });
});

describe("thread task selection limits", () => {
  it("does not add a task after the selection reaches the cap", () => {
    const selected = Array.from({ length: 200 }, (_, index) => `task-${index}`);
    expect(updateThreadTaskSelection(selected, "task-200", true)).toEqual(selected);
  });

  it("caps select-all and still removes every visible task", () => {
    const visible = Array.from({ length: 201 }, (_, index) => `task-${index}`);
    const selected = updateVisibleThreadTaskSelection([], visible, true);
    expect(selected).toHaveLength(200);
    expect(updateVisibleThreadTaskSelection(selected, visible, false)).toEqual([]);
  });
});

describe("threadTaskScopeModeUpdate", () => {
  // @covers AC-UI-THREADS-SAVED-VIEWS-003.17
  it("replaces a selection sort with ascending attention when the scope becomes all", () => {
    expect(
      threadTaskScopeModeUpdate(
        {
          taskScope: { mode: "selected", taskIds: ["task-b", "task-a"] },
          sort: { key: "selection", direction: "desc" },
        },
        "all",
      ),
    ).toEqual({
      taskScope: { mode: "all", taskIds: [] },
      sort: { key: "attention", direction: "asc" },
    });
  });

  it("keeps any other sort when the scope becomes all", () => {
    expect(
      threadTaskScopeModeUpdate(
        {
          taskScope: { mode: "selected", taskIds: ["task-a"] },
          sort: { key: "title", direction: "desc" },
        },
        "all",
      ),
    ).toEqual({ taskScope: { mode: "all", taskIds: [] } });
  });

  it("keeps the stored selection order when the scope stays selected", () => {
    expect(
      threadTaskScopeModeUpdate(
        {
          taskScope: { mode: "selected", taskIds: ["task-b", "task-a"] },
          sort: { key: "selection", direction: "asc" },
        },
        "selected",
      ),
    ).toEqual({ taskScope: { mode: "selected", taskIds: ["task-b", "task-a"] } });
  });
});
