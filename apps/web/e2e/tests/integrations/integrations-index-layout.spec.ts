import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";

const INTEGRATION_LABELS = ["GitHub", "GitLab", "Jira", "Linear", "Sentry", "Slack"];

test.describe("integrations settings index layout", () => {
  test("renders equal-height integration cards on desktop", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 1024, height: 900 });
    await testPage.goto("/settings/integrations");

    const heights = await integrationCardHeights(testPage);
    const topInsets = await integrationCardIconTopInsets(testPage);

    expect(Math.max(...heights) - Math.min(...heights)).toBeLessThanOrEqual(1);
    expect(Math.max(...topInsets)).toBeLessThanOrEqual(22);
  });
});

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
