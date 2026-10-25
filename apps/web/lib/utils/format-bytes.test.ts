import { describe, it, expect } from "vitest";
import { formatBytes, formatDistinctByteSizes } from "./format-bytes";

describe("formatBytes", () => {
  it("returns '-' for nullish input", () => {
    expect(formatBytes(null)).toBe("-");
    expect(formatBytes(undefined)).toBe("-");
  });

  it("returns '0 B' for zero or negative input", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(-1)).toBe("0 B");
  });

  it("renders integer bytes below 1 KB", () => {
    expect(formatBytes(1)).toBe("1 B");
    expect(formatBytes(1023)).toBe("1023 B");
  });

  it("renders KB with one decimal", () => {
    expect(formatBytes(1024)).toBe("1.0 KB");
    expect(formatBytes(2048)).toBe("2.0 KB");
  });

  it("renders MB / GB / TB", () => {
    expect(formatBytes(1024 * 1024)).toBe("1.0 MB");
    expect(formatBytes(1024 * 1024 * 1024)).toBe("1.0 GB");
    expect(formatBytes(1024 ** 4)).toBe("1.0 TB");
  });

  it("clamps to TB above the largest unit", () => {
    expect(formatBytes(1024 ** 5)).toBe("1024.0 TB");
    expect(formatBytes(1024 ** 6)).toBe("1048576.0 TB");
  });

  it("returns '-' for non-finite input", () => {
    expect(formatBytes(NaN)).toBe("-");
    expect(formatBytes(Infinity)).toBe("-");
    expect(formatBytes(-Infinity)).toBe("-");
  });
});

describe("formatDistinctByteSizes", () => {
  it("uses formatBytes for both values when they render distinctly", () => {
    expect(formatDistinctByteSizes(262144, 300000)).toEqual(["256.0 KB", "293.0 KB"]);
  });

  it("falls back to exact byte counts when the rounded forms would collide", () => {
    // Both round to "256.0 KB" under formatBytes' one-decimal precision.
    expect(formatBytes(262144)).toBe(formatBytes(262150));
    expect(formatDistinctByteSizes(262144, 262150)).toEqual(["262144 B", "262150 B"]);
  });

  it("does not fall back when the two values are exactly equal", () => {
    expect(formatDistinctByteSizes(262144, 262144)).toEqual(["256.0 KB", "256.0 KB"]);
  });
});
