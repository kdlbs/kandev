import { describe, expect, it } from "vitest";
import {
  MAX_SCALE,
  MIN_SCALE,
  actualSizeView,
  clampView,
  fitView,
  formatZoom,
  minScaleFor,
  nextZoomStep,
  panBy,
  zoomAt,
} from "./viewport";

const stage = { width: 800, height: 600 };
const image = { width: 1600, height: 900 };

describe("fitView", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-004.1
  it("contains the whole image and centers it", () => {
    const view = fitView(stage, image);
    expect(view.scale).toBe(0.5);
    expect(view.x).toBe(0);
    expect(view.y).toBe(-150);
  });

  it("returns an identity view when sizes are unknown", () => {
    expect(fitView({ width: 0, height: 0 }, image)).toEqual({ scale: 1, x: 0, y: 0 });
  });
});

describe("zoom steps", () => {
  it("moves to the next documented step and clamps at the limits", () => {
    expect(nextZoomStep(1, 1)).toBe(2);
    expect(nextZoomStep(1, -1)).toBe(0.5);
    expect(nextZoomStep(0.7, 1)).toBe(1);
    expect(nextZoomStep(0.7, -1)).toBe(0.5);
    expect(nextZoomStep(MAX_SCALE, 1)).toBe(MAX_SCALE);
    expect(nextZoomStep(MIN_SCALE, -1)).toBe(MIN_SCALE);
  });

  it("formats the zoom level as a percentage", () => {
    expect(formatZoom(0.5)).toBe("50%");
    expect(formatZoom(8)).toBe("800%");
  });
});

describe("zoomAt", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-004.2
  it("keeps the image point under the focal point fixed", () => {
    const view = { scale: 1, x: 100, y: 50 };
    const focal = { x: 200, y: 100 };
    const next = zoomAt(view, 4, focal);
    const imagePointBefore = { x: view.x + focal.x / view.scale, y: view.y + focal.y / view.scale };
    const imagePointAfter = { x: next.x + focal.x / next.scale, y: next.y + focal.y / next.scale };
    expect(next.scale).toBe(4);
    expect(imagePointAfter).toEqual(imagePointBefore);
  });

  it("clamps the scale to the supported range", () => {
    expect(zoomAt({ scale: 1, x: 0, y: 0 }, 50, { x: 0, y: 0 }).scale).toBe(MAX_SCALE);
    expect(zoomAt({ scale: 1, x: 0, y: 0 }, 0.01, { x: 0, y: 0 }).scale).toBe(MIN_SCALE);
  });
});

describe("panBy and clampView", () => {
  it("pans in image pixels for a stage-pixel drag", () => {
    expect(panBy({ scale: 2, x: 100, y: 100 }, 40, -20)).toEqual({ scale: 2, x: 80, y: 110 });
  });

  it("keeps a large image covering the stage", () => {
    const clamped = clampView({ scale: 1, x: -500, y: 5000 }, stage, image);
    expect(clamped).toEqual({ scale: 1, x: 0, y: 300 });
  });

  it("keeps a small image inside the stage", () => {
    const clamped = clampView({ scale: 1, x: 10, y: -1000 }, stage, { width: 400, height: 300 });
    expect(clamped).toEqual({ scale: 1, x: 0, y: -300 });
  });
});

describe("actualSizeView", () => {
  it("shows one image pixel per CSS pixel around the current center", () => {
    const view = actualSizeView(stage, image, fitView(stage, image));
    expect(view.scale).toBe(1);
    expect(view.x).toBe(400);
    expect(view.y).toBe(150);
  });
});

describe("fit below the manual zoom range", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-004.1
  it("always contains the image, even below 25%", () => {
    const view = fitView({ width: 366, height: 87 }, { width: 640, height: 400 });
    expect(view.scale).toBeCloseTo(87 / 400);
    expect(400 * view.scale).toBeLessThanOrEqual(87);
  });

  it("does not zoom out past the smaller of Fit and 25%", () => {
    expect(minScaleFor(0.1)).toBe(0.1);
    expect(minScaleFor(0.5)).toBe(MIN_SCALE);
    expect(nextZoomStep(0.25, -1, 0.1)).toBe(0.1);
    expect(nextZoomStep(0.1, -1, 0.1)).toBe(0.1);
    expect(nextZoomStep(0.1, 1, 0.1)).toBe(0.25);
    expect(zoomAt({ scale: 0.2, x: 0, y: 0 }, 0.01, { x: 0, y: 0 }, 0.1).scale).toBe(0.1);
  });
});
