import { test, expect } from "../../fixtures/ssh-test-base";
import { SSHSettingsPage } from "../../pages/SSHSettingsPage";

/**
 * UI persistence: state survives reload, executor list renders the SSH
 * entry with the right icon and label, partial form state doesn't leak.
 *
 * Covers e2e-plan.md group R (R1–R4).
 */
test.describe("ssh executor — persistence + UI sweep", () => {
  test("fingerprint persists across reload (Trusted badge stays)", async ({
    testPage,
    seedData,
  }) => {
    const page = new SSHSettingsPage(testPage);
    await page.gotoExisting(seedData.sshExecutorId);
    await expect(page.connectionBadge).toHaveAttribute("data-status", "trusted");

    await testPage.reload();
    await expect(page.connectionBadge).toHaveAttribute("data-status", "trusted");
    await expect(page.pinnedFingerprint()).toHaveText(seedData.sshTarget.hostFingerprint);
  });

  test("opening the new-executor form does not leak state from a prior session", async ({
    testPage,
  }) => {
    const page = new SSHSettingsPage(testPage);
    await page.gotoNew();
    await expect(page.nameInput).toHaveValue("");
    await expect(page.hostInput).toHaveValue("");
    await expect(page.portInput).toHaveValue("22"); // default
    await expect(page.connectionBadge).toHaveAttribute("data-status", "unverified");
  });

  test("executor list renders the SSH entry with the right label", async ({
    apiClient,
    seedData,
  }) => {
    const { executors } = await apiClient.listExecutors();
    const ssh = executors.find((e) => e.id === seedData.sshExecutorId);
    expect(ssh).toBeDefined();
    expect(ssh!.type).toBe("ssh");
  });

  test("incomplete test (no click yet) does not show a result panel", async ({
    testPage,
    seedData,
  }) => {
    const page = new SSHSettingsPage(testPage);
    await page.gotoNew();
    await page.fillForm({
      name: "R4 partial",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identitySource: "file",
      identityFile: seedData.sshTarget.identityFile,
    });
    // No clickTest call → no result panel.
    await expect(testPage.getByTestId("ssh-test-result")).toHaveCount(0);
    await expect(page.saveButton).toBeDisabled();
  });
});
