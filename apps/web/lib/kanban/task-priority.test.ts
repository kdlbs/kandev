import { describe, expect, it } from "vitest";
import {
  isKanbanPriority,
  KANBAN_PRIORITY_LABEL_KEYS,
  KANBAN_PRIORITY_TOKENS,
} from "./task-priority";

describe("KANBAN_PRIORITY_TOKENS", () => {
  it("lists the four tokens in severity order", () => {
    expect(KANBAN_PRIORITY_TOKENS).toEqual(["critical", "high", "medium", "low"]);
  });

  it("has a label key for every token", () => {
    for (const token of KANBAN_PRIORITY_TOKENS) {
      expect(KANBAN_PRIORITY_LABEL_KEYS[token]).toMatch(/^kanban:priority/);
    }
  });
});

describe("isKanbanPriority", () => {
  it.each(["critical", "high", "medium", "low"])("accepts %s", (token) => {
    expect(isKanbanPriority(token)).toBe(true);
  });

  it.each([undefined, null, "", "urgent", "CRITICAL", 1, {}])("rejects %s", (value) => {
    expect(isKanbanPriority(value)).toBe(false);
  });
});
