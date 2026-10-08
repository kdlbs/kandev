import { describe, expect, it } from "vitest";
import {
  GROUP_OPTION_LABEL_KEYS,
  SORT_OPTION_LABEL_KEYS,
  TASKS_LIST_GROUP_OPTIONS,
  TASKS_LIST_SORT_OPTIONS,
  sortTasksByFacet,
  parseTasksListGroup,
  DEFAULT_TASKS_LIST_GROUP,
} from "./tasks-list-options";

// @covers AC-UI-LIST-STEP-GROUPING-001.1 and AC-UI-LIST-STEP-GROUPING-001.5
describe("workflow step grouping preferences", () => {
  it.each(["state", "workflow_step", "invalid", null, undefined, ""])(
    "resolves %s to workflow step grouping",
    (value) => expect(parseTasksListGroup(value)).toBe("workflow_step"),
  );
  it("offers workflow step as the default instead of runtime state", () => {
    expect(DEFAULT_TASKS_LIST_GROUP).toBe("workflow_step");
    expect(TASKS_LIST_GROUP_OPTIONS.map((option) => option.value)).toEqual([
      "workflow_step",
      "workflow",
      "repository",
      "none",
    ]);
  });
});

describe("SORT_OPTION_LABEL_KEYS", () => {
  it("maps every sort option to a tasks: translation key", () => {
    for (const option of TASKS_LIST_SORT_OPTIONS) {
      const key = SORT_OPTION_LABEL_KEYS[option.value];
      expect(key).toBeDefined();
      expect(key).toMatch(/^tasks:/);
    }
  });

  it("has no stray keys beyond the configured sort options", () => {
    const optionValues = new Set(TASKS_LIST_SORT_OPTIONS.map((option) => option.value));
    expect(Object.keys(SORT_OPTION_LABEL_KEYS).sort()).toEqual([...optionValues].sort());
  });
});

describe("sortTasksByFacet", () => {
  it("sorts by the first label, keeps ties stable, and leaves untagged tasks last", () => {
    const tasks = [
      { id: "one", title: "one" },
      { id: "two", title: "two" },
      { id: "three", title: "three" },
    ] as never[];
    expect(
      sortTasksByFacet(tasks, "facet:plugin:tags", {
        "facet:plugin:tags:one": [{ value: "z", label: "Zulu" }],
        "facet:plugin:tags:two": [
          { value: "a", label: "alpha" },
          { value: "z", label: "Zulu" },
        ],
      }).map((task) => task.id),
    ).toEqual(["two", "one", "three"]);
  });
});

describe("GROUP_OPTION_LABEL_KEYS", () => {
  it("maps every group option to a tasks: translation key", () => {
    for (const option of TASKS_LIST_GROUP_OPTIONS) {
      const key = GROUP_OPTION_LABEL_KEYS[option.value];
      expect(key).toBeDefined();
      expect(key).toMatch(/^tasks:/);
    }
  });

  it("has no stray keys beyond the configured group options", () => {
    const optionValues = new Set(TASKS_LIST_GROUP_OPTIONS.map((option) => option.value));
    expect(Object.keys(GROUP_OPTION_LABEL_KEYS).sort()).toEqual([...optionValues].sort());
  });
});
