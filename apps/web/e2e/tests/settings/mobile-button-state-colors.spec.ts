import { test, expect } from "../../fixtures/test-base";
import { setButtonTheme } from "../../helpers/button-state-colors";
import { expectTouchSquareControl } from "../../helpers/control-sizing";
import { enableCanvasFeature } from "../canvas/canvas-fixture";
import {
  checkNotificationButton,
  checkSettingsSaveColors,
  expectPrimaryButton,
} from "./button-state-colors-helpers";

test.describe("Phone button colors", () => {
  for (const theme of ["light", "dark"] as const) {
    test(`uses neutral unavailable notification actions in ${theme}`, async ({
      testPage,
    }, testInfo) => {
      await testPage.setViewportSize({ width: 390, height: 844 });
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
      await testPage.setViewportSize({ width: 390, height: 844 });
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
      await testPage.setViewportSize({ width: 390, height: 844 });
      await checkSettingsSaveColors(testPage, apiClient, theme === "dark", (state) =>
        testInfo.outputPath(`settings-save-${state}.png`),
      );
    });

    test(`opens the primary Canvas action from native phone navigation in ${theme}`, async ({
      testPage,
      apiClient,
      backend,
      seedData,
    }, testInfo) => {
      await testPage.setViewportSize({ width: 390, height: 844 });
      const release = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
      try {
        await testPage.goto(`/?workspaceId=${encodeURIComponent(seedData.workspaceId)}`);
        await expect(testPage.getByTestId("kanban-board")).toBeVisible({ timeout: 20000 });
        await testPage.getByTestId("app-nav-trigger").tap();
        await testPage
          .getByRole("button", { name: /canvases/i })
          .first()
          .click();
        await testPage.getByRole("link", { name: "Open canvas settings", exact: true }).tap();
        await expect(testPage.getByTestId("workspace-canvases-page")).toBeVisible();
        const setup = testPage.getByTestId("settings-create-canvas");
        await expect(setup).toBeVisible();
        await setButtonTheme(testPage, theme === "dark");
        await expectPrimaryButton(setup);
        await expectTouchSquareControl(setup);
        await testPage.screenshot({ path: testInfo.outputPath("canvas-settings.png") });
        await setup.tap();
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
