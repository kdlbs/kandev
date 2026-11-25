import { test, expect } from "../../fixtures/ssh-test-base";
import {
  createSSHRoute,
  observeSSHRoute,
  readSSHRoute,
  trustAndTestSSHRoute,
} from "../../helpers/ssh-managed-fixture";
import { SSHSettingsPage } from "../../pages/SSHSettingsPage";

test("executor only offers a shared route after explicit host trust and successful access", async ({
  backend,
  seedData,
  testPage,
}) => {
  const route = await createSSHRoute(backend.baseUrl, "Untrusted connection", seedData.sshTarget);
  const page = new SSHSettingsPage(testPage);
  await page.gotoNew();
  await expect(
    page.connectionPicker.locator(`option[value="${route.destination.id}"]`),
  ).toHaveCount(0);
  await observeSSHRoute(backend.baseUrl, route);
  expect((await readSSHRoute(backend.baseUrl, route.destination.id)).destination.fingerprint).toBe(
    "",
  );

  await trustAndTestSSHRoute(backend.baseUrl, route);
  await testPage.reload();
  await page.connectionPicker.selectOption(route.destination.id);
  await expect(page.testButton).toBeEnabled();
  await expect(page.saveButton).toBeDisabled();
});
