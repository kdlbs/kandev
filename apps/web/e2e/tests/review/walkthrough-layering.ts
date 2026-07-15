import { expect, type Locator, type Page } from "@playwright/test";

type WalkthroughSurface = {
  locator: Locator;
  name: string;
};

async function zIndex(locator: Locator): Promise<number> {
  return locator.evaluate((element) => Number.parseInt(getComputedStyle(element).zIndex, 10));
}

export async function expectWalkthroughBehindDialog(
  page: Page,
  dialog: Locator,
  surfaces: WalkthroughSurface[],
): Promise<void> {
  const backdrop = page.locator('[data-slot$="dialog-overlay"]:visible');
  await expect(backdrop).toBeVisible();

  const dialogZIndex = await zIndex(dialog);
  const backdropZIndex = await zIndex(backdrop);

  for (const surface of surfaces) {
    const surfaceZIndex = await zIndex(surface.locator);
    expect(surfaceZIndex, `${surface.name} should be below dialog content`).toBeLessThan(
      dialogZIndex,
    );
    expect(surfaceZIndex, `${surface.name} should be below dialog backdrop`).toBeLessThan(
      backdropZIndex,
    );
  }
}
