import type { Locator } from "@playwright/test";

export async function waitForFiniteAnimations(surface: Locator): Promise<void> {
  await surface.evaluate(async (element) => {
    const activeFinite = element
      .getAnimations({ subtree: true })
      .filter(
        (animation) =>
          animation.playState === "running" &&
          Number.isFinite(animation.effect?.getComputedTiming().iterations),
      );
    await Promise.allSettled(activeFinite.map((animation) => animation.finished));
  });
}
