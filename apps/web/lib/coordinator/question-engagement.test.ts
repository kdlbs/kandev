import { describe, it } from "vitest";
import assert from "node:assert/strict";
import { shouldEngageQuestion } from "./question-engagement";

describe("question engagement", () => {
  const target = { closest: () => null } as unknown as HTMLElement;
  it("leaves Escape collapse available without holding a reply", () => {
    assert.equal(shouldEngageQuestion({ key: "Escape", target }), false);
  });
  it("ignores the collapse toggle but holds ordinary choices and keystrokes", () => {
    const toggle = { closest: () => ({}) } as unknown as HTMLElement;
    assert.equal(shouldEngageQuestion({ target: toggle }), false);
    assert.equal(shouldEngageQuestion({ target }), true);
    assert.equal(shouldEngageQuestion({ key: "a", target }), true);
  });
});
