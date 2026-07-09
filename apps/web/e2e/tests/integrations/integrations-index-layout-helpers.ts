import type { Locator, Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";

// Cards own the vertical padding (py-4 = 16px); allow border/subpixel slack, but catch extra content top padding.
const MAX_ICON_TOP_INSET_PX = 22;

export async function expectStableIntegrationCardLayout(page: Page) {
  const cards = await integrationCards(page);
  const heights = await integrationCardHeights(cards);
  const topInsets = await integrationCardIconTopInsets(cards);

  expect(Math.max(...heights) - Math.min(...heights)).toBeLessThanOrEqual(1);
  expect(Math.max(...topInsets)).toBeLessThanOrEqual(MAX_ICON_TOP_INSET_PX);
}

async function integrationCardHeights(cards: Locator[]) {
  const heights = await Promise.all(
    cards.map(async (card, index) => {
      await expect(card).toBeVisible();
      const box = await card.boundingBox();
      if (!box) throw new Error(`Missing integration card bounds at index ${index}`);
      return box.height;
    }),
  );
  return heights;
}

async function integrationCardIconTopInsets(cards: Locator[]) {
  const topInsets = await Promise.all(
    cards.map(async (card, index) => {
      const icon = card.locator("svg").first();
      const [cardBox, iconBox] = await Promise.all([card.boundingBox(), icon.boundingBox()]);
      if (!cardBox || !iconBox) {
        throw new Error(`Missing integration card icon bounds at index ${index}`);
      }
      return iconBox.y - cardBox.y;
    }),
  );
  return topInsets;
}

async function integrationCards(page: Page): Promise<Locator[]> {
  const links = page.getByTestId("settings-scroll-container").locator('a[href*="/integrations/"]');
  await expect(links.first()).toBeVisible();
  const count = await links.count();
  expect(count).toBeGreaterThan(0);

  return Promise.all(
    Array.from({ length: count }, async (_, index) => {
      const card = links.nth(index).locator(':scope > [data-slot="card"]');
      await expect(card).toHaveCount(1);
      return card;
    }),
  );
}
