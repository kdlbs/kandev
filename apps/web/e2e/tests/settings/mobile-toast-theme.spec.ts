import { test } from "../../fixtures/test-base";
import { TOAST_THEME_CASES, verifyPluginToastTheme } from "../../helpers/toast-theme";
import { uninstallPluginFixture } from "../plugins/plugin-test-helpers";

test.describe("Notification theme on mobile", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallPluginFixture(apiClient);
  });

  // @covers AC-UI-TOAST-THEME-001.1, AC-UI-TOAST-THEME-001.2
  for (const scenario of TOAST_THEME_CASES) {
    test(`plugin install toast follows ${scenario.name} after a cold load`, async ({
      testPage,
    }, testInfo) => {
      await verifyPluginToastTheme(testPage, scenario, true);
      await testPage.screenshot({ path: testInfo.outputPath("toast-theme.png") });
    });
  }
});
