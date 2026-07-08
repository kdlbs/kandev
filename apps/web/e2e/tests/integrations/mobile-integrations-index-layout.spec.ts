import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";

const INTEGRATION_LABELS = ["GitHub", "GitLab", "Jira", "Linear", "Sentry", "Slack"];

test.describe("integrations settings index layout on mobile", () => {
  test("renders equal-height integration cards", async ({ testPage }) => {
    await testPage.goto("/settings/integrations");

    const heights = await integrationCardHeights(testPage);

    expect(Math.max(...heights) - Math.min(...heights)).toBeLessThanOrEqual(1);
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
