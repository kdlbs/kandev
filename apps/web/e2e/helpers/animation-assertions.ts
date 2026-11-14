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

export async function expectCompositorMotionPaused(target: Locator) {
  await expect
    .poll(
      () =>
        target.evaluate((element) => {
          const animations = element.getAnimations();
          return (
            (element as HTMLElement).style.animationPlayState === "paused" &&
            animations.every((animation) => animation.playState !== "running")
          );
        }),
      { timeout: 5_000 },
    )
    .toBe(true);
}

export async function expectReducedStatusMotion(page: import("@playwright/test").Page) {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const targets = page.locator(
    ".spinner-grid-cube, [data-compositor-spin], [data-compositor-pulse]",
  );
  await expect.poll(() => targets.count()).toBeGreaterThan(0);
  await expect
    .poll(() =>
      targets.evaluateAll((elements) =>
        elements.every((element) =>
          element.getAnimations().every((animation) => animation.playState !== "running"),
        ),
      ),
    )
    .toBe(true);
  const cubes = page.locator(".spinner-grid-cube:visible");
  await expect.poll(() => cubes.count()).toBeGreaterThan(0);
  expect(
    await cubes.evaluateAll((elements) =>
      elements.every((element) => {
        const style = getComputedStyle(element);
        return style.animationName === "none" && style.transform === "matrix(0.5, 0, 0, 0.5, 0, 0)";
      }),
    ),
  ).toBe(true);
  await page.emulateMedia({ reducedMotion: "no-preference" });
}
