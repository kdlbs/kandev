import { expect, test } from "../../fixtures/test-base";
import { importAndSelectTheme } from "./theme-flow";

test("phone imports and saves a theme without horizontal overflow", async ({
  testPage,
  apiClient,
}, testInfo) => {
  test.setTimeout(120_000);
  const reference = await importAndSelectTheme(testPage);
  try {
    await expect
      .poll(() => testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
      .toBe(true);
    await testPage.screenshot({ path: testInfo.outputPath("theme-phone.png"), fullPage: true });
  } finally {
    await apiClient
      .rawRequest("DELETE", `/api/plugins/${reference.slice(6).split("/")[0]}`)
      .catch(() => undefined);
  }
});
