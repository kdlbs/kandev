import { describe, expect, it } from "vitest";
import { isTaskPriority, TASK_PRIORITY_LABEL_KEYS, TASK_PRIORITY_TOKENS } from "./task-priority";

describe("TASK_PRIORITY_TOKENS", () => {
  it("lists the four tokens in severity order", () => {
    expect(TASK_PRIORITY_TOKENS).toEqual(["critical", "high", "medium", "low"]);
  });

  it("has a label key for every token", () => {
    for (const token of TASK_PRIORITY_TOKENS) {
      expect(TASK_PRIORITY_LABEL_KEYS[token]).toMatch(/^kanban:priority/);
    }
  });
});

describe("isTaskPriority", () => {
  it.each(["critical", "high", "medium", "low"])("accepts %s", (token) => {
    expect(isTaskPriority(token)).toBe(true);
  });

  it.each([undefined, null, "", "urgent", "CRITICAL", 1, {}])("rejects %s", (value) => {
    expect(isTaskPriority(value)).toBe(false);
  });
});
