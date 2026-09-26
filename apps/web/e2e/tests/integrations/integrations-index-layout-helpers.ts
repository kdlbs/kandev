import type { Locator, Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";

// Cards own the vertical padding (py-4 = 16px); allow border/subpixel slack, but catch extra content top padding.
const MAX_ICON_TOP_INSET_PX = 22;

/**
 * Asserts the integrations index page's per-integration cards render with a
 * stable layout: every card has equal height (row stretch is not masking a
 * content-length difference) and the leading icon sits within its
 * original top-inset budget (catches the header row silently growing,
 * e.g. from a squeezed/wrapped label).
 */
export async function expectStableIntegrationCardLayout(page: Page) {
  const cards = await integrationCards(page);
  const heights = await integrationCardHeights(cards);
  const topInsets = await integrationCardIconTopInsets(cards);

  expect(Math.max(...heights) - Math.min(...heights)).toBeLessThanOrEqual(1);
  expect(Math.max(...topInsets)).toBeLessThanOrEqual(MAX_ICON_TOP_INSET_PX);
}

/** Measured height (px) of each integration card, in DOM order. */
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

/** Vertical offset (px) of each card's leading icon from its card's top edge. */
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

/** Locates every rendered integration card on the settings index page. */
async function integrationCards(page: Page): Promise<Locator[]> {
  const cards = page
    .getByTestId("settings-scroll-container")
    .locator('[data-testid^="integration-card-"]');
  await expect(cards.first()).toBeVisible();
  const count = await cards.count();
  expect(count).toBeGreaterThan(0);

  return Array.from({ length: count }, (_, index) => cards.nth(index));
}
