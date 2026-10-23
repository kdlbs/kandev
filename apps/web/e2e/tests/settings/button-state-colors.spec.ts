import { test, expect } from "../../fixtures/test-base";
import { setButtonTheme } from "../../helpers/button-state-colors";
import { enableCanvasFeature } from "../canvas/canvas-fixture";
import {
  checkNotificationButton,
  checkSettingsSaveColors,
  expectNeutralCanvasSetup,
} from "./button-state-colors-helpers";

test.describe("Desktop button colors", () => {
  for (const theme of ["light", "dark"] as const) {
    test(`uses neutral unavailable notification actions in ${theme}`, async ({
      testPage,
    }, testInfo) => {
      await testPage.setViewportSize({ width: 1280, height: 900 });
      await checkNotificationButton(
        testPage,
        "unsupported",
        theme === "dark",
        testInfo.outputPath("notification-unsupported.png"),
      );
    });

    test(`retains granted notification success status in ${theme}`, async ({
      testPage,
    }, testInfo) => {
      await testPage.setViewportSize({ width: 1280, height: 900 });
      await checkNotificationButton(
        testPage,
        "granted",
        theme === "dark",
        testInfo.outputPath("notification-granted.png"),
      );
    });

    test(`distinguishes enabled, pending, and completed settings Save in ${theme}`, async ({
      testPage,
      apiClient,
    }, testInfo) => {
      await testPage.setViewportSize({ width: 1280, height: 900 });
      await checkSettingsSaveColors(testPage, apiClient, theme === "dark", (state) =>
        testInfo.outputPath(`settings-save-${state}.png`),
      );
    });

    test(`uses neutral Canvas sidebar navigation in ${theme}`, async ({
      testPage,
      apiClient,
      backend,
      seedData,
    }, testInfo) => {
      await testPage.setViewportSize({ width: 1280, height: 900 });
      const release = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
      try {
        await testPage.goto(`/?workspaceId=${encodeURIComponent(seedData.workspaceId)}`);
        await expect(testPage.getByTestId("kanban-board")).toBeVisible({ timeout: 20000 });

        await testPage
          .getByRole("button", { name: /canvases/i })
          .first()
          .click();
        await expect(testPage.getByTestId("sidebar-canvases-settings")).toBeVisible();
        const setup = testPage.getByTestId("sidebar-canvases-empty");
        await setButtonTheme(testPage, theme === "dark");
        await expectNeutralCanvasSetup(setup);
        await testPage.screenshot({ path: testInfo.outputPath("canvas-sidebar.png") });
        await setup.click();
        const dialog = testPage.getByTestId("create-task-dialog");
        await expect(dialog).toBeVisible();
        await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
        await expect(dialog).toBeHidden();
      } finally {
        await release();
      }
    });
  }
});
