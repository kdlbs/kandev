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
 * Devin permission controls through the real settings API and the disposable
 * mock ACP descriptor. Mock capabilities prove the UI contract; the backend
 * defaults and mode/sandbox validation have their own unit and controller tests.
 */
test.describe("Devin profile permissions", () => {
  test("defaults new profiles to Smart with an explicit disabled sandbox", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(180_000);
    await enableDevinMockProvider(backend);
    try {
      await removeDisposableDevinRows(apiClient);
      await openDevinDraft(testPage);

      await expect(sandboxControl(testPage)).toContainText("Disabled");

      await testPage.getByTestId("profile-name-input").fill("Devin permissions");
      const save = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await expect(save).toBeEnabled({ timeout: 20_000 });
      await save.click();

      const firstSave = await expectPersistedDevinProfile(apiClient);
      expect(firstSave.mode).toBe("smart");
      expect(firstSave.sandbox).toBe("false");
      expect(firstSave.model).not.toBe("");
    } finally {
      await cleanupDevinProfile(apiClient, backend);
    }
  });

  test("enabling the sandbox requests Autonomous and keeps the pair consistent", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(180_000);
    await enableDevinMockProvider(backend);
    try {
      await removeDisposableDevinRows(apiClient);
      await openDevinDraft(testPage);

      await testPage.getByTestId("profile-name-input").fill("Devin sandbox");
      await chooseSandbox(testPage, "Enabled");
      await expect(testPage.getByTestId("devin-sandbox-autonomous")).toBeVisible();
      await expect(sandboxControl(testPage)).toContainText("Enabled");

      const save = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await expect(save).toBeEnabled({ timeout: 20_000 });
      await save.click();

      const persisted = await expectPersistedDevinProfile(apiClient);
      expect(persisted.mode).toBe("autonomous");
      expect(persisted.sandbox).toBe("true");
    } finally {
      await cleanupDevinProfile(apiClient, backend);
    }
  });
});
