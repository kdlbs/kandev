import { test, expect } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";

/**
 * On a phone the workspace settings tab strip is a horizontally scrolling row
 * of pills. Every tab is its own route, so navigating remounts the strip —
 * without the shell's scroll restoration the strip snaps back to its start
 * and the pill you just tapped ends up off-screen.
 */
async function expectPillInsideStrip(strip: Locator, pill: Locator) {
  const [stripBox, pillBox] = await Promise.all([strip.boundingBox(), pill.boundingBox()]);
  if (!stripBox || !pillBox) throw new Error("tab strip or pill has no layout box");
  // Fully inside the strip's viewport, not clipped at either edge.
  expect(pillBox.x).toBeGreaterThanOrEqual(stripBox.x - 1);
  expect(pillBox.x + pillBox.width).toBeLessThanOrEqual(stripBox.x + stripBox.width + 1);
}

async function expectActivePillVisible(testPage: Page, label: string) {
  const strip = testPage.getByTestId("workspace-settings-tabs");
  const pill = strip.locator('[aria-current="page"]');
  await expect(pill).toContainText(label);
  await expectPillInsideStrip(strip, pill);
}

test.describe("Mobile workspace settings tab strip", () => {
  test("keeps the selected pill in view when landing on and switching tabs", async ({
    testPage,
    seedData,
  }) => {
    // Landing directly on the last tab: a fresh strip would sit at its start
    // with the Secrets pill clipped off the right edge.
    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/secrets`);
    await expectActivePillVisible(testPage, "Secrets");

    // Switching pills remounts the strip on the new route; the pill just
    // tapped must still be in view rather than the strip jumping to its start.
    const strip = testPage.getByTestId("workspace-settings-tabs");
    await strip.getByRole("link", { name: "Automations" }).tap();
    await expect(testPage).toHaveURL(
      new RegExp(`/settings/workspaces/${seedData.workspaceId}/automations$`),
    );
    await expectActivePillVisible(testPage, "Automations");
  });
});
