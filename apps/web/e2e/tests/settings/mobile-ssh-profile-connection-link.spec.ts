import { test, expect } from "../../fixtures/test-base";

test.describe("ssh profile connection settings on mobile", () => {
  test("opens the connection test page from the SSH profile editor", async ({
    apiClient,
    testPage,
  }) => {
    const executor = await apiClient.createSSHExecutor("Mobile SSH link target", {
      ssh_host: "127.0.0.1",
      ssh_port: "22",
      ssh_user: "kandev",
      ssh_identity_source: "agent",
      ssh_host_fingerprint: "SHA256:test",
    });
    const profile = await apiClient.createExecutorProfile(executor.id, {
      name: "Mobile SSH profile with link",
      config: {},
      prepare_script: "",
      cleanup_script: "",
      env_vars: [],
    });

    await testPage.goto(`/settings/executors/${profile.id}`);

    const link = testPage.getByTestId("ssh-connection-settings-link");
    await expect(link).toBeVisible();
    await link.click();
    await expect(testPage).toHaveURL(new RegExp(`/settings/executors/ssh/${executor.id}$`));
    await expect(testPage.getByTestId("ssh-test-button")).toBeVisible();
  });
});
