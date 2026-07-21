import { describe, it, expect } from "vitest";
import { resolveActiveId } from "./resolve-active-id";

const items = [{ id: "sky" }, { id: "test" }];

describe("resolveActiveId", () => {
  it("returns the first candidate that exists in items", () => {
    expect(resolveActiveId(items, "test")).toBe("test");
  });

  it("honours candidate priority order (earlier wins)", () => {
    // URL param > cookie > setting: the cookie ("test") should win over the
    // saved setting ("sky") when no URL param is given.
    expect(resolveActiveId(items, undefined, "test", "sky")).toBe("test");
  });

  it("falls through to the next candidate when an earlier one is missing", () => {
    expect(resolveActiveId(items, "ghost", "test")).toBe("test");
  });

  it("skips null/undefined candidates", () => {
    expect(resolveActiveId(items, null, undefined, "sky")).toBe("sky");
  });

  it("falls back to the first item when no candidate matches", () => {
    expect(resolveActiveId(items, "ghost", null)).toBe("sky");
  });

  it("returns null when there are no items", () => {
    expect(resolveActiveId([], "test")).toBeNull();
  });
});
