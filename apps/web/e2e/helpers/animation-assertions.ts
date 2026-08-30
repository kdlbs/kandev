import type { Locator } from "@playwright/test";
import { expect } from "@playwright/test";

export async function expectCompositorGridMotion(status: Locator) {
  await expect(status.locator(".spinner-grid-cube")).toHaveCount(9);
  await expect
    .poll(() =>
      status.locator(".spinner-grid-cube").evaluateAll((cubes) =>
        cubes.every((cube) => {
          const animations = cube.getAnimations();
          return (
            animations.length === 1 &&
            animations[0].playState === "running" &&
            animations[0].effect?.getTiming().iterations === Infinity &&
            animations[0].constructor.name === "Animation"
          );
        }),
      ),
    )
    .toBe(true);
}

export async function expectCompositorPulse(pulse: Locator) {
  await expect(pulse).toBeVisible();
  await expect
    .poll(() =>
      pulse.evaluate((element) => {
        const animations = element.getAnimations();
        return (
          animations.length === 1 &&
          animations[0].playState === "running" &&
          animations[0].effect?.getTiming().iterations === Infinity &&
          animations[0].constructor.name === "Animation"
        );
      }),
    )
    .toBe(true);
}
