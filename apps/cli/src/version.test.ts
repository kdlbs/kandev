import { describe, expect, it } from "vitest";

import { compareVersions, parseVersion, sortVersionsDesc } from "./version";

describe("parseVersion", () => {
  it("parses simple version", () => {
    const v = parseVersion("1.2.3");
    expect(v.segments).toEqual([1, 2, 3]);
    expect(v.raw).toBe("1.2.3");
  });

  it("strips v prefix", () => {
    expect(parseVersion("v1.2").segments).toEqual([1, 2]);
  });

  it("handles single segment", () => {
    expect(parseVersion("3").segments).toEqual([3]);
  });

  it("handles two segments", () => {
    expect(parseVersion("v0.9").segments).toEqual([0, 9]);
  });

  it("returns empty segments for non-numeric input", () => {
    expect(parseVersion("latest").segments).toEqual([]);
  });

  it("returns empty segments for partially numeric input", () => {
    expect(parseVersion("1.2.beta").segments).toEqual([]);
  });
});

describe("compareVersions", () => {
  it("returns 0 for equal versions", () => {
    expect(compareVersions("1.0.0", "1.0.0")).toBe(0);
  });

  it("returns 1 when a > b (patch)", () => {
    expect(compareVersions("1.0.1", "1.0.0")).toBe(1);
  });

  it("returns -1 when a < b (patch)", () => {
    expect(compareVersions("1.0.0", "1.0.1")).toBe(-1);
  });

  it("returns 1 when a > b (minor)", () => {
    expect(compareVersions("1.1.0", "1.0.0")).toBe(1);
  });

  it("returns 1 when a > b (major)", () => {
    expect(compareVersions("2.0.0", "1.9.9")).toBe(1);
  });

  it("strips v prefix", () => {
    expect(compareVersions("v1.2.0", "1.2.0")).toBe(0);
  });

  it("strips v prefix on both sides", () => {
    expect(compareVersions("v2.0.0", "v1.0.0")).toBe(1);
  });

  it("treats missing segments as 0", () => {
    expect(compareVersions("1.0", "1.0.0")).toBe(0);
  });

  it("compares different length versions", () => {
    expect(compareVersions("1.0.1", "1.0")).toBe(1);
  });

  it("handles single-segment versions", () => {
    expect(compareVersions("2", "1")).toBe(1);
  });

  it("handles single vs multi-segment", () => {
    expect(compareVersions("1", "1.0.0")).toBe(0);
  });
});

describe("sortVersionsDesc", () => {
  it("sorts versions newest first", () => {
    expect(sortVersionsDesc(["v0.1", "v1.2", "v0.9"])).toEqual(["v1.2", "v0.9", "v0.1"]);
  });

  it("sorts major versions correctly", () => {
    expect(sortVersionsDesc(["v1.9", "v2.0"])).toEqual(["v2.0", "v1.9"]);
  });

  it("sorts numerically not lexicographically", () => {
    expect(sortVersionsDesc(["v1.9", "v1.10", "v1.2"])).toEqual(["v1.10", "v1.9", "v1.2"]);
  });

  it("handles single element", () => {
    expect(sortVersionsDesc(["v1.0"])).toEqual(["v1.0"]);
  });

  it("handles empty array", () => {
    expect(sortVersionsDesc([])).toEqual([]);
  });

  it("does not mutate the original array", () => {
    const input = ["v0.2", "v0.1"];
    sortVersionsDesc(input);
    expect(input).toEqual(["v0.2", "v0.1"]);
  });
});
