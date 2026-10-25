import { describe, expect, it } from "vitest";
import {
  DEFAULT_TASKS_LIST_GROUP,
  DEFAULT_TASKS_LIST_SORT,
  GROUP_OPTION_LABEL_KEYS,
  SORT_OPTION_LABEL_KEYS,
  TASKS_LIST_GROUP_OPTIONS,
  TASKS_LIST_SORT_OPTIONS,
  encodeTaskListFacetOption,
  firstFacetLabel,
  parseTaskListFacetOption,
  parseTasksListGroup,
  parseTasksListSort,
  resolveTasksListApiSort,
  sortTasksByFacet,
} from "./tasks-list-options";

const FACET_OPTION = "facet:plugin:tags";

describe("facet preference shape", () => {
  // AC-PLUGINS-TASKLIST-FACETS-003.4: the client must keep the trimmed token.
  it("round-trips facet and built-in preferences, trimming before matching", () => {
    for (const value of [
      FACET_OPTION,
      "facet:com.example.tags:tag-name",
      "facet:my_plugin:tags",
      ` ${FACET_OPTION} `,
    ]) {
      expect(parseTasksListSort(value)).toBe(value.trim());
      expect(parseTasksListGroup(value)).toBe(value.trim());
    }
    expect(parseTasksListSort(" title_asc ")).toBe("title_asc");
    expect(parseTasksListGroup(" workflow ")).toBe("workflow");
    for (const bad of [
      "facet:",
      "facet:Plugin:tags",
      "facet:plugin:tag id",
      "facet:plugin:",
      "facet:plugin:UPPER",
      "facet:plugin:tags:extra",
      "unknown",
    ]) {
      expect(parseTasksListSort(bad)).toBe(DEFAULT_TASKS_LIST_SORT);
      expect(parseTasksListGroup(bad)).toBe(DEFAULT_TASKS_LIST_GROUP);
    }
  });

  it("encodes a facet and resolves only built-in values for task-list requests", () => {
    expect(encodeTaskListFacetOption({ pluginId: "my_plugin", id: "tag-name" })).toBe(
      "facet:my_plugin:tag-name",
    );
    expect(parseTaskListFacetOption(" facet:my_plugin:tag-name ")).toEqual({
      pluginId: "my_plugin",
      id: "tag-name",
    });
    expect(parseTaskListFacetOption("facet:Plugin:tags")).toBeNull();
    expect(resolveTasksListApiSort(` ${FACET_OPTION} `)).toBe(DEFAULT_TASKS_LIST_SORT);
    expect(resolveTasksListApiSort(" title_asc ")).toBe("title_asc");
  });
});

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

  // AC-PLUGINS-TASKLIST-FACETS-002.6 and 002.8: keep same-base order and ignore empty labels.
  it("keeps incoming order for case-insensitive ties and uses a non-empty sibling", () => {
    const tasks = [{ id: "uppercase" }, { id: "lowercase" }, { id: "empty" }] as never[];
    expect(
      firstFacetLabel([
        { value: "missing", label: "" },
        { value: "a", label: "Alpha" },
      ]),
    ).toBe("Alpha");
    expect(
      sortTasksByFacet(tasks, "facet:plugin:tags", {
        "facet:plugin:tags:uppercase": [{ value: "b", label: "Beta" }],
        "facet:plugin:tags:lowercase": [{ value: "b", label: "beta" }],
        "facet:plugin:tags:empty": [{ value: "missing", label: "" }],
      }).map((task) => task.id),
    ).toEqual(["uppercase", "lowercase", "empty"]);
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
