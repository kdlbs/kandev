import { test, expect } from "../../fixtures/ssh-test-base";
import { SSHSettingsPage } from "../../pages/SSHSettingsPage";

test.describe("SSH binding persistence", () => {
  test("existing executor edits persist only after the floating Save", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const page = new SSHSettingsPage(testPage);
    await page.gotoExisting(seedData.sshExecutorId);

    const originalName = await page.nameInput.inputValue();
    const connection = await page.connectionPicker.inputValue();
    try {
      const renamed = `${originalName} renamed`;
      await page.nameInput.fill(renamed);
      await expect(page.floatingSaveButton).toBeVisible();
      await expect(page.floatingSaveButton).toBeDisabled();

      const beforeSave = (await apiClient.listExecutors()).executors.find(
        (executor) => executor.id === seedData.sshExecutorId,
      );
      expect(beforeSave?.name).toBe(originalName);

      await page.testButton.click();
      await expect(page.executorReady).toBeVisible();
      await expect(page.floatingSaveButton).toBeEnabled();
      const saved = testPage.waitForResponse(
        (response) =>
          response.request().method() === "PATCH" &&
          response.url().endsWith(`/executors/${seedData.sshExecutorId}`),
      );
      await page.floatingSaveButton.click();
      expect((await saved).ok()).toBe(true);
      await expect(testPage.getByTestId("settings-floating-save")).toHaveAttribute(
        "data-status",
        "saved",
        { timeout: 15_000 },
      );

      const afterSave = (await apiClient.listExecutors()).executors.find(
        (executor) => executor.id === seedData.sshExecutorId,
      );
      expect(afterSave?.name).toBe(renamed);
      await testPage.reload();
      await expect(page.nameInput).toHaveValue(renamed);
      await expect(page.connectionPicker).toHaveValue(connection);
    } finally {
      await apiClient.updateExecutor(seedData.sshExecutorId, { name: originalName });
    }
  });
});
