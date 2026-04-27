import { describe, it, expect } from "vitest";
import { lineDiff, diffSummary } from "./task-plan-diff";

describe("lineDiff", () => {
  it("returns all-context for identical strings", () => {
    const result = lineDiff("a\nb\nc", "a\nb\nc");
    expect(result.map((l) => l.kind)).toEqual(["context", "context", "context"]);
    expect(result.map((l) => l.text)).toEqual(["a", "b", "c"]);
  });

  it("marks added lines", () => {
    const result = lineDiff("a\n", "a\nb\n");
    expect(result).toEqual([
      { kind: "context", text: "a" },
      { kind: "add", text: "b" },
    ]);
  });

  it("marks removed lines", () => {
    const result = lineDiff("a\nb\n", "a\n");
    expect(result).toEqual([
      { kind: "context", text: "a" },
      { kind: "remove", text: "b" },
    ]);
  });

  it("handles a mixed diff", () => {
    const result = lineDiff("a\nb\nc\n", "a\nB\nc\n");
    expect(result).toEqual([
      { kind: "context", text: "a" },
      { kind: "remove", text: "b" },
      { kind: "add", text: "B" },
      { kind: "context", text: "c" },
    ]);
  });

  it("empty before", () => {
    const result = lineDiff("", "x\ny\n");
    expect(result.map((l) => l.kind)).toEqual(["add", "add"]);
    expect(result.map((l) => l.text)).toEqual(["x", "y"]);
  });

  it("empty after", () => {
    const result = lineDiff("x\ny\n", "");
    expect(result.map((l) => l.kind)).toEqual(["remove", "remove"]);
  });

  it("does not emit a trailing empty line", () => {
    const result = lineDiff("a\n", "a\n");
    expect(result).toHaveLength(1);
    expect(result[0]).toEqual({ kind: "context", text: "a" });
  });
});

describe("diffSummary", () => {
  it("counts added and removed", () => {
    const result = diffSummary([
      { kind: "context", text: "a" },
      { kind: "add", text: "b" },
      { kind: "add", text: "c" },
      { kind: "remove", text: "d" },
    ]);
    expect(result).toEqual({ added: 2, removed: 1 });
  });

  it("zero on no changes", () => {
    expect(diffSummary([{ kind: "context", text: "a" }])).toEqual({ added: 0, removed: 0 });
  });
});
