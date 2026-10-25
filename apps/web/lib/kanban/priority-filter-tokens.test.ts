import { describe, expect, it } from "vitest";
import {
  parseKanbanPriorityFilterTokens,
  taskMatchesPriorityFilter,
  toggleKanbanPriorityFilterToken,
} from "./priority-filter-tokens";
import type { TaskPriority } from "@/lib/types/http";

// A row written before the priority writer existed, or written outside the
// service's validated path, reads back with a stored value outside the four
// tokens (AC-001.10's "persistent" origin) rather than an absent field (the
// "transient" origin every `undefined` fixture below covers). Both must be
// treated identically.
const PERSISTENT_UNRANKED = "urgent" as TaskPriority;

describe("parseKanbanPriorityFilterTokens", () => {
  it("keeps a fully valid selection, ordered by rank", () => {
    expect(parseKanbanPriorityFilterTokens(["low", "critical", "high"])).toEqual([
      "critical",
      "high",
      "low",
    ]);
  });

  it("drops a value outside the four tokens and keeps the remainder", () => {
    expect(parseKanbanPriorityFilterTokens(["critical", "urgent", "high"])).toEqual([
      "critical",
      "high",
    ]);
  });

  it("resolves an all-invalid selection to empty rather than displaying nothing", () => {
    expect(parseKanbanPriorityFilterTokens(["urgent", "none"])).toEqual([]);
  });

  it("resolves null to the empty selection", () => {
    expect(parseKanbanPriorityFilterTokens(null)).toEqual([]);
  });

  it("resolves a bare string to the empty selection rather than splitting it", () => {
    expect(parseKanbanPriorityFilterTokens("critical")).toEqual([]);
  });

  it("resolves a non-list object to the empty selection", () => {
    expect(parseKanbanPriorityFilterTokens({ critical: true })).toEqual([]);
  });

  it("dedupes a value repeated in the stored list", () => {
    expect(parseKanbanPriorityFilterTokens(["high", "high", "critical"])).toEqual([
      "critical",
      "high",
    ]);
  });

  it("resolves an empty list to the empty selection", () => {
    expect(parseKanbanPriorityFilterTokens([])).toEqual([]);
  });
});

describe("toggleKanbanPriorityFilterToken", () => {
  it("adds a token that is not yet selected", () => {
    expect(toggleKanbanPriorityFilterToken(["critical"], "high")).toEqual(["critical", "high"]);
  });

  it("removes a token that is already selected", () => {
    expect(toggleKanbanPriorityFilterToken(["critical", "high"], "critical")).toEqual(["high"]);
  });

  it("keeps the result in rank order regardless of toggle order", () => {
    expect(toggleKanbanPriorityFilterToken(["low"], "critical")).toEqual(["critical", "low"]);
  });

  it("selecting an already-selected token twice returns to the original selection", () => {
    const once = toggleKanbanPriorityFilterToken(["critical"], "critical");
    expect(once).toEqual([]);
    expect(toggleKanbanPriorityFilterToken(once, "critical")).toEqual(["critical"]);
  });
});

describe("taskMatchesPriorityFilter", () => {
  it("admits every task under the empty selection, including an unranked one", () => {
    expect(taskMatchesPriorityFilter("critical", [])).toBe(true);
    expect(taskMatchesPriorityFilter(undefined, [])).toBe(true);
  });

  it("admits only a member of a non-empty selection", () => {
    expect(taskMatchesPriorityFilter("high", ["critical", "high"])).toBe(true);
    expect(taskMatchesPriorityFilter("low", ["critical", "high"])).toBe(false);
  });

  it("excludes an unranked task under any non-empty selection", () => {
    expect(taskMatchesPriorityFilter(undefined, ["critical"])).toBe(false);
  });

  it("treats a persistent-origin unranked task (stored value outside the vocabulary) identically to the transient (absent) origin", () => {
    expect(taskMatchesPriorityFilter(PERSISTENT_UNRANKED, [])).toBe(true);
    expect(taskMatchesPriorityFilter(PERSISTENT_UNRANKED, ["critical"])).toBe(false);
  });
});
