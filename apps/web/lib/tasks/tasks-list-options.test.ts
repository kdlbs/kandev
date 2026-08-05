import { describe, expect, it } from "vitest";
import {
  GROUP_OPTION_LABEL_KEYS,
  SORT_OPTION_LABEL_KEYS,
  TASKS_LIST_GROUP_OPTIONS,
  TASKS_LIST_SORT_OPTIONS,
} from "./tasks-list-options";

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
