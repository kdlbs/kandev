import { describe, expect, it } from "vitest";
import { moveRegion, rectFromPoints, resizeRegion, resizeRegionFromCorner } from "./geometry";

const bounds = { width: 100, height: 80 };

describe("rectFromPoints", () => {
  it("builds an integer rectangle in either drag direction, clamped to the bounds", () => {
    expect(rectFromPoints({ x: 10.4, y: 20.6 }, { x: 30.2, y: 5.1 }, bounds)).toEqual({
      x: 10,
      y: 5,
      width: 21,
      height: 16,
    });
    expect(rectFromPoints({ x: -10, y: -10 }, { x: 500, y: 500 }, bounds)).toEqual({
      x: 0,
      y: 0,
      width: 100,
      height: 80,
    });
  });

  it("returns null for a click without a drag", () => {
    expect(rectFromPoints({ x: 10, y: 10 }, { x: 10.4, y: 10.2 }, bounds)).toBeNull();
  });
});

describe("moveRegion and resizeRegion", () => {
  it("keep the rectangle inside the bounds", () => {
    const rect = { x: 90, y: 70, width: 10, height: 10 };
    expect(moveRegion(rect, 5, 5, bounds)).toEqual(rect);
    expect(moveRegion(rect, -5, -1, bounds)).toEqual({ x: 85, y: 69, width: 10, height: 10 });
    expect(resizeRegion(rect, 5, -20, bounds)).toEqual({ x: 90, y: 70, width: 10, height: 1 });
  });
});

describe("resizeRegionFromCorner", () => {
  const rect = { x: 20, y: 20, width: 40, height: 30 };

  it("moves only the dragged corner's edges", () => {
    expect(resizeRegionFromCorner(rect, "se", { x: 10, y: 5 }, bounds)).toEqual({
      x: 20,
      y: 20,
      width: 50,
      height: 35,
    });
    expect(resizeRegionFromCorner(rect, "nw", { x: -5, y: -10 }, bounds)).toEqual({
      x: 15,
      y: 10,
      width: 45,
      height: 40,
    });
    expect(resizeRegionFromCorner(rect, "ne", { x: 5, y: 5 }, bounds)).toEqual({
      x: 20,
      y: 25,
      width: 45,
      height: 25,
    });
    expect(resizeRegionFromCorner(rect, "sw", { x: 5, y: 5 }, bounds)).toEqual({
      x: 25,
      y: 20,
      width: 35,
      height: 35,
    });
  });

  it("clamps to the bounds", () => {
    expect(resizeRegionFromCorner(rect, "se", { x: 500, y: 500 }, bounds)).toEqual({
      x: 20,
      y: 20,
      width: 80,
      height: 60,
    });
    expect(resizeRegionFromCorner(rect, "nw", { x: -500, y: -500 }, bounds)).toEqual({
      x: 0,
      y: 0,
      width: 60,
      height: 50,
    });
  });

  it("keeps the minimum size instead of flipping", () => {
    expect(resizeRegionFromCorner(rect, "se", { x: -500, y: -500 }, bounds, 8)).toEqual({
      x: 20,
      y: 20,
      width: 8,
      height: 8,
    });
    expect(resizeRegionFromCorner(rect, "nw", { x: 500, y: 500 }, bounds, 8)).toEqual({
      x: 52,
      y: 42,
      width: 8,
      height: 8,
    });
  });
});
