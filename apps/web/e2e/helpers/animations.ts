import type { Locator } from "@playwright/test";

export async function waitForFiniteAnimations(surface: Locator): Promise<void> {
  await surface.evaluate(async (element) => {
    const animations = element.getAnimations({ subtree: true }).filter((animation) => {
      const iterations = animation.effect?.getComputedTiming().iterations;
      return typeof iterations === "number" && Number.isFinite(iterations);
    });
    await Promise.all(animations.map((animation) => animation.finished.catch(() => undefined)));
  });
}
