import { describe, expect, it } from "vitest";
import { placeFloatingRect, type FloatingBounds } from "./floating-selection-position";

describe("placeFloatingRect", () => {
  const dialogBounds: FloatingBounds = { left: 100, top: 50, right: 500, bottom: 450 };

  it("clamps horizontal placement to the portal container", () => {
    expect(
      placeFloatingRect({
        left: 480,
        topCandidates: [100],
        width: 340,
        height: 180,
        bounds: dialogBounds,
      }),
    ).toEqual({ left: 152, top: 100 });
  });

  it("uses the first fitting vertical candidate within the container", () => {
    expect(
      placeFloatingRect({
        left: 120,
        topCandidates: [430, 246],
        width: 40,
        height: 180,
        bounds: dialogBounds,
      }),
    ).toEqual({ left: 120, top: 246 });
  });
});
