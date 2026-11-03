import { expect } from "@playwright/test";

export function expectTouchDimension(dimension: number) {
  // Playwright reports fractional CSS pixels; allow only float-rounding noise.
  expect(dimension + 0.01).toBeGreaterThanOrEqual(44);
}
