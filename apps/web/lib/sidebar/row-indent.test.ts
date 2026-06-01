import { describe, it, expect } from "vitest";
import { computeRowIndent, resolveRowDepth } from "./row-indent";

describe("resolveRowDepth", () => {
  it("uses an explicit depth when provided", () => {
    expect(resolveRowDepth(3, false)).toBe(3);
    expect(resolveRowDepth(0, true)).toBe(0);
  });

  it("falls back to isSubTask (legacy single-level) when depth is absent", () => {
    expect(resolveRowDepth(undefined, true)).toBe(1);
    expect(resolveRowDepth(undefined, false)).toBe(0);
    expect(resolveRowDepth(undefined, undefined)).toBe(0);
  });
});

describe("computeRowIndent", () => {
  it("root rows (depth 0) get base padding and no nesting", () => {
    const indent = computeRowIndent(0);
    expect(indent.depth).toBe(0);
    expect(indent.paddingLeftPx).toBe(12);
  });

  it("depth 1 matches the legacy subtask indent (pl-8 / connector at 14px)", () => {
    const indent = computeRowIndent(1);
    expect(indent.depth).toBe(1);
    expect(indent.paddingLeftPx).toBe(32);
    expect(indent.connectorLeftPx).toBe(14);
  });

  it("each deeper level adds a fixed step", () => {
    const d2 = computeRowIndent(2);
    expect(d2.paddingLeftPx).toBe(52);
    expect(d2.connectorLeftPx).toBe(34);
  });

  it("caps visual indent so deep trees don't squeeze the sidebar", () => {
    const capped = computeRowIndent(6);
    const deeper = computeRowIndent(20);
    expect(deeper.paddingLeftPx).toBe(capped.paddingLeftPx);
    expect(deeper.connectorLeftPx).toBe(capped.connectorLeftPx);
    // Still flagged as nested so the connector renders.
    expect(deeper.depth).toBeGreaterThan(0);
  });
});
