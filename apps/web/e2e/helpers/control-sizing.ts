import { expect, type Locator } from "@playwright/test";

const PIXEL_TOLERANCE = 1;

export async function controlHeight(locator: Locator): Promise<number> {
  const box = await locator.boundingBox();
  expect(box, "control should have a rendered bounding box").not.toBeNull();
  return box!.height;
}

export async function expectControlHeight(
  locator: Locator,
  expectedHeight: number,
  tolerance = PIXEL_TOLERANCE,
): Promise<void> {
  const height = await controlHeight(locator);
  expect(Math.abs(height - expectedHeight)).toBeLessThanOrEqual(tolerance);
}

export async function expectControlWidth(
  locator: Locator,
  expectedWidth: number,
  tolerance = PIXEL_TOLERANCE,
): Promise<void> {
  const box = await locator.boundingBox();
  expect(box, "control should have a rendered bounding box").not.toBeNull();
  expect(Math.abs(box!.width - expectedWidth)).toBeLessThanOrEqual(tolerance);
}

export async function expectTouchSquareControl(locator: Locator): Promise<void> {
  const box = await locator.boundingBox();
  expect(box, "touch control should have a rendered bounding box").not.toBeNull();
  expect(box!.height).toBeGreaterThanOrEqual(44);
  expect(box!.width).toBeGreaterThanOrEqual(44);
}

export async function expectTouchControl(locator: Locator): Promise<void> {
  const box = await locator.boundingBox();
  expect(box, "touch control should have a rendered bounding box").not.toBeNull();
  expect(box!.height).toBeGreaterThanOrEqual(44);
}
