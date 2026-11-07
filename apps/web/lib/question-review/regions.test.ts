import { describe, expect, it } from "vitest";
import {
  MAX_REGIONS,
  centerRegion,
  moveRegion,
  rectFromPoints,
  regionCommentsReady,
  resizeRegion,
  stageToImage,
} from "./regions";

const image = { width: 100, height: 80 };

describe("stageToImage", () => {
  it("converts stage pixels to image pixels through the view", () => {
    expect(stageToImage({ scale: 2, x: 10, y: 5 }, { x: 40, y: 20 })).toEqual({ x: 30, y: 15 });
  });
});

describe("rectFromPoints", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-007.2
  it("builds an integer rectangle in either drag direction, clamped to the image", () => {
    expect(rectFromPoints({ x: 10.4, y: 20.6 }, { x: 30.2, y: 5.1 }, image)).toEqual({
      x: 10,
      y: 5,
      width: 21,
      height: 16,
    });
    expect(rectFromPoints({ x: -10, y: -10 }, { x: 500, y: 500 }, image)).toEqual({
      x: 0,
      y: 0,
      width: 100,
      height: 80,
    });
  });

  it("returns null for a click without a drag", () => {
    expect(rectFromPoints({ x: 10, y: 10 }, { x: 10.4, y: 10.2 }, image)).toBeNull();
  });
});

describe("keyboard regions", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-007.7
  it("creates a region at the visible center", () => {
    const rect = centerRegion({ scale: 1, x: 0, y: 0 }, { width: 100, height: 80 }, image);
    expect(rect).toEqual({ x: 40, y: 32, width: 20, height: 16 });
  });

  it("moves and resizes within the image", () => {
    const rect = { x: 90, y: 70, width: 10, height: 10 };
    expect(moveRegion(rect, 5, 5, image)).toEqual(rect);
    expect(moveRegion(rect, -5, -1, image)).toEqual({ x: 85, y: 69, width: 10, height: 10 });
    expect(resizeRegion(rect, 5, -20, image)).toEqual({ x: 90, y: 70, width: 10, height: 1 });
  });
});

describe("regionCommentsReady", () => {
  it("requires a non-empty comment within the limit on every region", () => {
    const region = {
      image_id: "a",
      x: 0,
      y: 0,
      width: 1,
      height: 1,
      image_width: 10,
      image_height: 10,
      comment: "ok",
    };
    expect(regionCommentsReady({ q1: [region] })).toBe(true);
    expect(regionCommentsReady({ q1: [{ ...region, comment: "  " }] })).toBe(false);
    expect(regionCommentsReady({ q1: [{ ...region, comment: "x".repeat(501) }] })).toBe(false);
    expect(regionCommentsReady({})).toBe(true);
    expect(MAX_REGIONS).toBe(8);
  });
});
