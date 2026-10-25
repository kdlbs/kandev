import { describe, expect, it } from "vitest";
import { codePointLength } from "./workspace-pause-controls";

describe("codePointLength", () => {
  it("counts plain ASCII the same as string.length", () => {
    expect(codePointLength("incident")).toBe(8);
  });

  it("counts a single astral-plane emoji as one code point, not two UTF-16 units", () => {
    const emoji = "😀";
    expect(emoji.length).toBe(2);
    expect(codePointLength(emoji)).toBe(1);
  });

  it("matches the backend's utf8.RuneCountInString bound for a long emoji reason", () => {
    const reason = "😀".repeat(500);
    expect(reason.length).toBe(1000);
    expect(codePointLength(reason)).toBe(500);
  });

  it("returns 0 for an empty string", () => {
    expect(codePointLength("")).toBe(0);
  });
});
