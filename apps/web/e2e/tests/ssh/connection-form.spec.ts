import { test, expect } from "../../fixtures/ssh-test-base";
import { SSHSettingsPage } from "../../pages/SSHSettingsPage";

test("new SSH executor requires a tested shared connection and a name", async ({
  apiClient,
  seedData,
  testPage,
}) => {
  const executor = await apiClient.getExecutor(seedData.sshExecutorId);
  const page = new SSHSettingsPage(testPage);
  await page.gotoNew();
  await page.nameInput.fill("New shared executor");
  await expect(testPage.getByLabel("Host", { exact: true })).toHaveCount(0);
  await expect(page.testButton).toBeDisabled();
  await expect(page.saveButton).toBeDisabled();

  await page.connectionPicker.selectOption(executor.config!.ssh_connection_id);
  await expect(page.testButton).toBeEnabled();
  await expect(page.saveButton).toBeDisabled();
  await page.testButton.click();
  await expect(page.executorReady).toBeVisible();
  await expect(page.saveButton).toBeEnabled();
  await page.nameInput.clear();
  await expect(page.saveButton).toBeDisabled();
});
