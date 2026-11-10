/**
 * E2E: native catalog attribution and installed declaration labels.
 *
 * The fixture package declares `author: kandev`, but its curated repository is
 * owned by `acme`. Browse must show the curated owner and canonical repository;
 * installed details must show the unchanged manifest value as `Declared author`.
 * The fixture index is rebuilt by the real builder before its source is
 * registered, so this flow proves the production projection reaches the browser.
 */
import { expect, test } from "../../fixtures/test-base";
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

test("desktop: catalog shows the curated owner and details label the declared author", async ({
  testPage,
  apiClient,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const scenario = await startAttributionScenario(apiClient);
  try {
    await openBrowseTab(testPage);
    await expectCuratedBrowseAttribution(testPage);
    await expectCatalogApiAttribution(apiClient);
    await prCapture.screenshot("desktop-plugin-attribution-browse", {
      caption: "Browse attributes the fixture to its curated repository owner acme",
    });

    await installFromBrowse(testPage);
    await expectInstalledApiAttribution(apiClient);
    await openInstalledDetail(testPage);
    await expectDeclaredAuthor(testPage);
    await prCapture.screenshot("desktop-plugin-attribution-manifest", {
      caption: "Installed manifest identifies kandev as the declared author",
    });

    await testPage.reload();
    await expect(testPage.getByTestId(`plugin-detail-${PLUGIN_ID}`)).toBeVisible();
    await expectInstalledApiAttribution(apiClient);
    await expectDeclaredAuthor(testPage);
  } finally {
    await scenario.dispose();
  }
});
