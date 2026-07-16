import { describe, expect, it } from "vitest";
import { normalizeRepoValue, shouldShowCustomEntry } from "./repo-entry";

describe("normalizeRepoValue", () => {
  it("normalizes separators, trailing slashes, and case", () => {
    expect(normalizeRepoValue("C:\\Work\\App\\")).toBe("c:/work/app");
    expect(normalizeRepoValue("  /work/app/ ")).toBe("/work/app");
    expect(normalizeRepoValue("https://GitHub.com/Org/Repo")).toBe("https://github.com/org/repo");
  });
});

describe("shouldShowCustomEntry", () => {
  it("keeps the custom row when a suggestion only partially matches", () => {
    // /work/app must stay addable even though /work/app-old is listed —
    // selecting the near-miss suggestion would attach the wrong repo.
    expect(shouldShowCustomEntry("/work/app", ["/work/app-old"], [])).toBe(true);
    expect(shouldShowCustomEntry("app", ["/work/app-old"], [])).toBe(true);
  });

  it("hides the custom row on an exact match with an option", () => {
    expect(shouldShowCustomEntry("/work/app", ["/work/app"], [])).toBe(false);
  });

  it("treats normalization variants as exact matches", () => {
    expect(shouldShowCustomEntry("C:\\Work\\App\\", ["c:/work/app"], [])).toBe(false);
    expect(shouldShowCustomEntry("/work/app/", ["/work/app"], [])).toBe(false);
  });

  it("hides the custom row when the value is already attached", () => {
    expect(shouldShowCustomEntry("/work/app", [], ["/work/app"])).toBe(false);
  });

  it("returns false for empty or whitespace-only queries", () => {
    expect(shouldShowCustomEntry("", ["/work/app"], [])).toBe(false);
    expect(shouldShowCustomEntry("   ", [], [])).toBe(false);
  });
});
