import { expect, type Locator, type Page } from "@playwright/test";

type WalkthroughSurface = {
  locator: Locator;
  name: string;
};

async function zIndex(locator: Locator, name: string): Promise<number> {
  const rawValue = await locator.evaluate((element) => getComputedStyle(element).zIndex);
  const value = Number.parseInt(rawValue, 10);
  if (Number.isNaN(value)) {
    throw new Error(`${name} must have a numeric z-index; received ${JSON.stringify(rawValue)}`);
  }
  return value;
}

export async function expectWalkthroughBehindDialog(
  page: Page,
  dialog: Locator,
  surfaces: WalkthroughSurface[],
): Promise<void> {
  const backdrop = page.locator('[data-slot$="dialog-overlay"]:visible');
  await expect(backdrop).toBeVisible();

  const dialogZIndex = await zIndex(dialog, "dialog content");
  const backdropZIndex = await zIndex(backdrop, "dialog backdrop");

  for (const surface of surfaces) {
    const surfaceZIndex = await zIndex(surface.locator, surface.name);
    expect(surfaceZIndex, `${surface.name} should be below dialog content`).toBeLessThan(
      dialogZIndex,
    );
    expect(surfaceZIndex, `${surface.name} should be below dialog backdrop`).toBeLessThan(
      backdropZIndex,
    );
  }
}
