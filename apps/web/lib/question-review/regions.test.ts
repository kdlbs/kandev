import { describe, expect, it } from "vitest";
import {
  MAX_ANNOTATION_IMAGE_PIXELS,
  MAX_REGIONS,
  fitAnnotationToPixelLimit,
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

describe("fitAnnotationToPixelLimit", () => {
  const base = { image_id: "big", comment: "Spacing", x: 0, y: 0, width: 10, height: 10 };

  it("leaves regions on images within the limit unchanged", () => {
    const region = { ...base, image_width: 4000, image_height: 4000 };
    expect(fitAnnotationToPixelLimit(region)).toBe(region);
  });

  it("scales the reported size and rectangle of a larger image to fit the limit", () => {
    const region = {
      ...base,
      x: 4999,
      y: 2000,
      width: 1,
      height: 2000,
      image_width: 5000,
      image_height: 8000,
    };
    const fitted = fitAnnotationToPixelLimit(region);
    expect(fitted.image_width * fitted.image_height).toBeLessThanOrEqual(
      MAX_ANNOTATION_IMAGE_PIXELS,
    );
    expect(fitted.image_width / fitted.image_height).toBeCloseTo(5000 / 8000, 3);
    expect(fitted.width).toBeGreaterThanOrEqual(1);
    expect(fitted.x + fitted.width).toBeLessThanOrEqual(fitted.image_width);
    expect(fitted.y + fitted.height).toBeLessThanOrEqual(fitted.image_height);
    expect(fitted.y / fitted.image_height).toBeCloseTo(0.25, 2);
    expect(fitted.height / fitted.image_height).toBeCloseTo(0.25, 2);
    expect(fitted.comment).toBe("Spacing");
  });
});
