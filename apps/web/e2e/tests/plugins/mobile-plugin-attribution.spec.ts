/**
 * E2E: the phone rendering of curated catalog attribution and declared authors.
 *
 * Same real fixture and builder output as the desktop flow, but every action is
 * a touch tap. The content-only label change must stay inside the existing
 * settings scroll container and must not introduce horizontal document overflow.
 */
import { expect, test } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { PLUGIN_ID } from "../../helpers/plugin-fixture";
import {
  expectCatalogApiAttribution,
  expectCuratedBrowseAttribution,
  expectDeclaredAuthor,
  expectInstalledApiAttribution,
  installFromBrowse,
  openBrowseTab,
  openInstalledDetail,
  startAttributionScenario,
} from "./plugin-attribution-helpers";

async function expectNoHorizontalOverflow(page: Page) {
  const [scrollWidth, clientWidth] = await page.evaluate(() => [
    document.documentElement.scrollWidth,
    document.documentElement.clientWidth,
  ]);
  expect(scrollWidth).toBeLessThanOrEqual(clientWidth);
}

test("phone: curated attribution and declared authors stay readable in settings", async ({
  testPage,
  apiClient,
  prCapture,
}) => {
  test.setTimeout(180_000);
  const scenario = await startAttributionScenario(apiClient);
  try {
    await openBrowseTab(testPage, true);
    await expectCuratedBrowseAttribution(testPage);
    await expectCatalogApiAttribution(apiClient);
    await expectNoHorizontalOverflow(testPage);
    await prCapture.screenshot("mobile-plugin-attribution-browse", {
      caption: "Phone Browse shows the curated owner acme for the fixture",
    });

    await installFromBrowse(testPage, true);
    await expectInstalledApiAttribution(apiClient);
    await openInstalledDetail(testPage, true);

    const settingsPanel = testPage.locator('[data-testid="settings-scroll-container"]:visible');
    const overflowY = await settingsPanel.evaluate(
      (element) => getComputedStyle(element).overflowY,
    );
    expect(overflowY).toMatch(/auto|scroll/);
    await expectNoHorizontalOverflow(testPage);

    await expectDeclaredAuthor(testPage, true);
    await prCapture.screenshot("mobile-plugin-attribution-manifest", {
      caption: "Phone manifest shows kandev as the declared author inside the settings scroll",
    });

    await testPage.reload();
    await expect(testPage.getByTestId(`plugin-detail-${PLUGIN_ID}`)).toBeVisible();
    await expectInstalledApiAttribution(apiClient);
    await expectDeclaredAuthor(testPage, true);
    await expectNoHorizontalOverflow(testPage);
  } finally {
    await scenario.dispose();
  }
});
