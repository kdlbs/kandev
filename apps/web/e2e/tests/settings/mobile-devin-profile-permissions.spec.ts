import { test, expect } from "../../fixtures/test-base";
import {
  chooseSandbox,
  cleanupDevinProfile,
  enableDevinMockProvider,
  expectPersistedDevinProfile,
  openDevinDraft,
  removeDisposableDevinRows,
  sandboxControl,
} from "../../helpers/devin-profile-permissions";

/**
 * Phone parity for the Devin sandbox control: one column, a touch-sized select,
 * accurate inherited/disabled values, and no horizontal document overflow.
 */
test.describe("Mobile Devin profile permissions", () => {
  test("couples the sandbox control to Autonomous on a phone", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(180_000);
    await enableDevinMockProvider(backend);
    try {
      await removeDisposableDevinRows(apiClient);
      await openDevinDraft(testPage);

      const preference = testPage.getByTestId("devin-sandbox-preference");
      await expect(preference).toBeVisible();
      const rowDirection = await preference
        .locator("[data-settings-row='true']")
        .evaluate((element) => getComputedStyle(element).flexDirection);
      expect(rowDirection).toBe("column");

      const control = sandboxControl(testPage);
      await control.scrollIntoViewIfNeeded();
      expect((await control.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
      await expect(control).toContainText("Disabled");

      await testPage.getByTestId("profile-name-input").fill("Mobile Devin");
      await chooseSandbox(testPage, "Enabled");
      await expect(testPage.getByTestId("devin-sandbox-autonomous")).toBeVisible();

      const save = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await expect(save).toBeEnabled({ timeout: 20_000 });
      await save.click();

      const persisted = await expectPersistedDevinProfile(apiClient);
      expect(persisted.mode).toBe("autonomous");
      expect(persisted.sandbox).toBe("true");
      expect(
        await testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);
    } finally {
      await cleanupDevinProfile(apiClient, backend);
    }
  });
});
