import { test, expect } from "../../fixtures/ssh-test-base";
import { SSHSettingsPage } from "../../pages/SSHSettingsPage";

/**
 * UI form validation for the SSH connection card. None of these tests need
 * to *connect* to the host; they assert pure form behaviour. Live on the
 * `containers` project anyway so the SSH UI lives in one place.
 *
 * Covers e2e-plan.md group A (A1–A7).
 */
test.describe("ssh connection form validation", () => {
  test("renders all expected fields", async ({ testPage }) => {
    const page = new SSHSettingsPage(testPage);
    await page.gotoNew();

    await expect(page.nameInput).toBeVisible();
    await expect(page.hostAliasInput).toBeVisible();
    await expect(page.hostInput).toBeVisible();
    await expect(page.portInput).toBeVisible();
    await expect(page.userInput).toBeVisible();
    await expect(page.identitySourceTrigger).toBeVisible();
    await expect(page.proxyJumpInput).toBeVisible();
    // Identity file field is conditional: agent default hides it.
    await expect(page.identityFileInput).toBeHidden();
  });

  test("save disabled without a name", async ({ testPage }) => {
    const page = new SSHSettingsPage(testPage);
    await page.gotoNew();
    await expect(page.saveButton).toBeDisabled();

    await page.fillForm({ host: "dev.example.com" });
    await expect(page.saveButton).toBeDisabled(); // no name still
    await expect(page.testButton).toBeDisabled();
  });

  test("save and test disabled without host or alias", async ({ testPage }) => {
    const page = new SSHSettingsPage(testPage);
    await page.gotoNew();
    await page.fillForm({ name: "blank-host" });
    await expect(page.testButton).toBeDisabled();
    await expect(page.saveButton).toBeDisabled();
  });

  test("test enabled once name + host present", async ({ testPage }) => {
    const page = new SSHSettingsPage(testPage);
    await page.gotoNew();
    await page.fillForm({ name: "ready", host: "dev.example.com" });
    await expect(page.testButton).toBeEnabled();
    // Save still disabled until a test result + trust tick.
    await expect(page.saveButton).toBeDisabled();
  });

  test("test enabled with host_alias alone (no explicit host)", async ({ testPage }) => {
    const page = new SSHSettingsPage(testPage);
    await page.gotoNew();
    await page.fillForm({ name: "alias-only", hostAlias: "prod" });
    await expect(page.testButton).toBeEnabled();
  });

  test("identity_source=agent hides the identity file field; switching to file reveals it", async ({
    testPage,
  }) => {
    const page = new SSHSettingsPage(testPage);
    await page.gotoNew();
    // Default is "agent" — file input hidden.
    await expect(page.identityFileInput).toBeHidden();

    await page.fillForm({ identitySource: "file" });
    await expect(page.identityFileInput).toBeVisible();

    await page.fillForm({ identitySource: "agent" });
    await expect(page.identityFileInput).toBeHidden();
  });

  test("default port is 22", async ({ testPage }) => {
    const page = new SSHSettingsPage(testPage);
    await page.gotoNew();
    await expect(page.portInput).toHaveValue("22");
  });
});
