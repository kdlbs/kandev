import { describe, expect, it } from "vitest";
import { pixelLimitFactor } from "./pixel-limit";

describe("pixelLimitFactor", () => {
  it("returns the linear factor that brings an area down to the pixel limit", () => {
    expect(pixelLimitFactor(4_000, 4_000, 16_000_000)).toBe(1);
    expect(pixelLimitFactor(8_000, 4_000, 16_000_000)).toBeCloseTo(Math.SQRT1_2);
    expect(pixelLimitFactor(100, 100, 16_000_000)).toBe(40);
  });

  it("returns zero for an empty area", () => {
    expect(pixelLimitFactor(0, 100, 16_000_000)).toBe(0);
    expect(pixelLimitFactor(100, -1, 16_000_000)).toBe(0);
  });
});
