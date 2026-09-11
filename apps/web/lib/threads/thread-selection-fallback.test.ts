import { describe, expect, it } from "vitest";
import { resolveRemainingThreadId } from "./thread-selection-fallback";

// @covers AC-TASKS-THREADS-ACTIONS-003.1, AC-TASKS-THREADS-ACTIONS-003.2, AC-TASKS-THREADS-ACTIONS-003.3
describe("Threads selection recovery", () => {
  it.each([
    { next: ["A", "B", "C"], current: "B", expected: "B" },
    { next: ["B", "C"], current: "B", expected: "B" },
    { next: ["A", "C"], current: "B", expected: "C" },
    { next: ["A"], current: "B", expected: "A" },
    { next: ["D", "E"], current: "B", expected: "D" },
    { next: [], current: "B", expected: null },
    { next: ["A", "C"], current: null, expected: "A" },
  ])("selects $expected from $next after $current", ({ next, current, expected }) => {
    expect(resolveRemainingThreadId(["A", "B", "C"], next, current)).toBe(expected);
  });
});
