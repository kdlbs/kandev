import type { Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";

const INTEGRATION_LABELS = ["GitHub", "GitLab", "Jira", "Linear", "Sentry", "Slack"];

// Cards own the vertical padding (py-4 = 16px); allow border/subpixel slack, but catch extra content top padding.
const MAX_ICON_TOP_INSET_PX = 22;

export async function expectStableIntegrationCardLayout(page: Page) {
  const heights = await integrationCardHeights(page);
  const topInsets = await integrationCardIconTopInsets(page);

  expect(Math.max(...heights) - Math.min(...heights)).toBeLessThanOrEqual(1);
  expect(Math.max(...topInsets)).toBeLessThanOrEqual(MAX_ICON_TOP_INSET_PX);
}

async function integrationCardHeights(page: Page) {
  const content = page.getByTestId("settings-scroll-container");
  const heights = await Promise.all(
    INTEGRATION_LABELS.map(async (label) => {
      const card = content
        .getByRole("link", { name: new RegExp(`^${label}\\b`) })
        .locator("xpath=./*[1]");
      await expect(card).toBeVisible();
      const box = await card.boundingBox();
      if (!box) throw new Error(`Missing integration card bounds for ${label}`);
      return box.height;
    }),
  );
  return heights;
}

async function integrationCardIconTopInsets(page: Page) {
  const content = page.getByTestId("settings-scroll-container");
  const topInsets = await Promise.all(
    INTEGRATION_LABELS.map(async (label) => {
      const card = content
        .getByRole("link", { name: new RegExp(`^${label}\\b`) })
        .locator("xpath=./*[1]");
      const icon = card.locator("svg").first();
      const [cardBox, iconBox] = await Promise.all([card.boundingBox(), icon.boundingBox()]);
      if (!cardBox || !iconBox) throw new Error(`Missing integration card bounds for ${label}`);
      return iconBox.y - cardBox.y;
    }),
  );
  return topInsets;
}
