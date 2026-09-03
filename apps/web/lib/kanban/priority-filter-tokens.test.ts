import { describe, expect, it } from "vitest";
import {
  parseKanbanPriorityFilterTokens,
  taskMatchesPriorityFilter,
  toggleKanbanPriorityFilterToken,
} from "./priority-filter-tokens";

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
});
