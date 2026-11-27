import { describe, expect, it } from "vitest";
import {
  anchorBelowRect,
  placeFloatingRect,
  type FloatingBounds,
} from "./floating-selection-position";

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

describe("anchorBelowRect", () => {
  it("anchors below the bottom-left corner in viewport pixels", () => {
    expect(
      anchorBelowRect({ x: 20, y: 40, width: 100, height: 30 }, { left: 300, top: 100 }),
    ).toEqual({
      x: 320,
      y: 170,
    });
  });

  it("clamps the anchor to the visible host bounds", () => {
    const bounds: FloatingBounds = { left: 300, top: 100, right: 700, bottom: 400 };
    expect(
      anchorBelowRect({ x: -50, y: 900, width: 100, height: 30 }, { left: 300, top: 100 }, bounds),
    ).toEqual({ x: 300, y: 400 });
  });
});
