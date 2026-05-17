import { test, expect } from "../../fixtures/ssh-test-base";
import { SSHSettingsPage } from "../../pages/SSHSettingsPage";

/**
 * The defining UX guarantee of the SSH executor: Save is locked until the
 * user explicitly trusts the fingerprint they just observed, editing any
 * field that could reach a different machine resets that trust, and
 * fingerprint-change between trust and save is called out in the UI.
 *
 * Covers e2e-plan.md group C (C1–C7).
 */
test.describe("ssh trust gate", () => {
  test("save disabled until trust ticked even after a successful test", async ({
    testPage,
    seedData,
  }) => {
    const page = new SSHSettingsPage(testPage);
    await page.gotoNew();
    await page.fillForm({
      name: "C1",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identitySource: "file",
      identityFile: seedData.sshTarget.identityFile,
    });
    await page.clickTest();
    await page.waitForTestResult();
    await expect(page.saveButton).toBeDisabled();
    await page.tickTrust();
    await expect(page.saveButton).toBeEnabled();
  });

  for (const field of [
    { name: "host", apply: (p: SSHSettingsPage) => p.fillForm({ host: "other.example.com" }) },
    { name: "port", apply: (p: SSHSettingsPage) => p.fillForm({ port: 2222 }) },
    { name: "user", apply: (p: SSHSettingsPage) => p.fillForm({ user: "root" }) },
    {
      name: "identity_source",
      apply: (p: SSHSettingsPage) => p.fillForm({ identitySource: "agent" }),
    },
  ]) {
    test(`editing ${field.name} after a test clears the result and unticks trust`, async ({
      testPage,
      seedData,
    }) => {
      const page = new SSHSettingsPage(testPage);
      await page.gotoNew();
      await page.fillForm({
        name: `C-edit-${field.name}`,
        host: seedData.sshTarget.host,
        port: seedData.sshTarget.port,
        user: seedData.sshTarget.user,
        identitySource: "file",
        identityFile: seedData.sshTarget.identityFile,
      });
      await page.clickTest();
      await page.waitForTestResult();
      await page.tickTrust();
      await expect(page.saveButton).toBeEnabled();

      await field.apply(page);
      await expect(page.saveButton).toBeDisabled();
      await expect(page.trustCheckbox).not.toBeChecked();
    });
  }

  // C7 — fingerprint-change warning when re-testing with already-pinned
  // fingerprint differing from the new one. We simulate by editing the
  // executor's pinned fingerprint via the API and reloading the form.
  test("fingerprint-change warning surfaces when observed fingerprint differs from pinned", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Set a wrong fingerprint on the seeded executor.
    await apiClient.updateExecutor(seedData.sshExecutorId, {
      config: {
        ssh_host: seedData.sshTarget.host,
        ssh_port: String(seedData.sshTarget.port),
        ssh_user: seedData.sshTarget.user,
        ssh_identity_source: "file",
        ssh_identity_file: seedData.sshTarget.identityFile,
        ssh_host_fingerprint: "SHA256:wrong-fingerprint-on-purpose",
      },
    });

    const page = new SSHSettingsPage(testPage);
    await page.gotoExisting(seedData.sshExecutorId);
    await expect(page.pinnedFingerprint()).toHaveText("SHA256:wrong-fingerprint-on-purpose");

    await page.clickTest();
    await page.waitForTestResult();
    await expect(page.fingerprintChangeWarning()).toBeVisible();
    await expect(page.observedFingerprint()).toHaveText(seedData.sshTarget.hostFingerprint);
  });
});
