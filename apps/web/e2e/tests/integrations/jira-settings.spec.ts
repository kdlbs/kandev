import { test, expect } from "../../fixtures/test-base";
import { JiraSettingsPage } from "../../pages/jira-settings-page";

test.describe("Jira settings", () => {
  test("empty workspace shows form with disabled save/test until secret is filled", async ({
    testPage,
  }) => {
    const settings = new JiraSettingsPage(testPage);
    await settings.goto();

    await expect(settings.siteInput).toHaveValue("");
    await expect(settings.secretInput).toHaveValue("");
    await expect(settings.statusBanner).toHaveCount(0);
    await expect(settings.saveButton).toBeDisabled();
    await expect(settings.testButton).toBeDisabled();

    await settings.siteInput.fill("https://acme.atlassian.net");
    await settings.emailInput.fill("alice@example.com");
    await expect(settings.saveButton).toBeDisabled();

    await settings.secretInput.fill("api-token-value");
    await expect(settings.saveButton).toBeEnabled();
    await expect(settings.testButton).toBeEnabled();
  });

  test("saving the config persists across reload and shows the auth banner", async ({
    testPage,
    apiClient,
  }) => {
    const settings = new JiraSettingsPage(testPage);
    await settings.goto();

    await settings.fillForm({
      siteUrl: "https://acme.atlassian.net",
      email: "alice@example.com",
      secret: "api-token-value",
      projectKey: "PROJ",
    });
    await settings.saveButton.click();

    // After save the button label flips from "Save" to "Update" (config exists).
    await expect(settings.saveButton).toHaveText(/Update/i);
    // The post-save probe runs async; await it before reloading so the new
    // banner state is in the DB by the time the page re-fetches the config.
    await apiClient.waitForIntegrationAuthHealthy("jira");

    await testPage.reload();
    await settings.siteInput.waitFor();
    await expect(settings.siteInput).toHaveValue("https://acme.atlassian.net");
    await expect(settings.emailInput).toHaveValue("alice@example.com");
    await expect(settings.projectInput).toHaveValue("PROJ");
    await expect(settings.statusBanner).toHaveAttribute("data-state", "ok");
  });

  test("test connection surfaces inline success and failure", async ({ testPage, apiClient }) => {
    const settings = new JiraSettingsPage(testPage);
    await settings.goto();

    await apiClient.mockJiraSetAuthResult({
      ok: true,
      displayName: "Alice from Jira",
      email: "alice@example.com",
    });
    await settings.fillForm({
      siteUrl: "https://acme.atlassian.net",
      email: "alice@example.com",
      secret: "api-token-value",
    });
    await settings.testButton.click();
    await expect(testPage.getByText(/Connected as Alice from Jira/i)).toBeVisible();

    await apiClient.mockJiraSetAuthResult({ ok: false, error: "401 Unauthorized" });
    await settings.testButton.click();
    await expect(testPage.getByText(/Failed: 401 Unauthorized/)).toBeVisible();
  });

  test("seeded auth-health failure renders the failed banner on load", async ({
    testPage,
    apiClient,
  }) => {
    const settings = new JiraSettingsPage(testPage);
    // Save first so a config row exists, then simulate the poller writing
    // a failure status onto it.
    await settings.goto();
    await settings.fillForm({
      siteUrl: "https://acme.atlassian.net",
      email: "alice@example.com",
      secret: "api-token-value",
    });
    await settings.saveButton.click();
    // Wait for the post-save probe to land BEFORE forcing the failure: the
    // probe goroutine could otherwise overwrite our forced lastOk=false back
    // to true a few ms after the mockJiraSetAuthHealth call, flipping the
    // banner to "ok" right when the assertion expects "failed".
    await apiClient.waitForIntegrationAuthHealthy("jira");

    await apiClient.mockJiraSetAuthHealth({
      ok: false,
      error: "session expired",
    });
    await testPage.reload();
    await settings.statusBanner.waitFor();
    await expect(settings.statusBanner).toHaveAttribute("data-state", "failed");
    await expect(settings.statusBanner).toContainText(/session expired/i);
  });

  test("delete clears the saved configuration", async ({ testPage }) => {
    const settings = new JiraSettingsPage(testPage);
    await settings.goto();
    await settings.fillForm({
      siteUrl: "https://acme.atlassian.net",
      email: "alice@example.com",
      secret: "api-token-value",
    });
    await settings.saveButton.click();
    await expect(settings.deleteButton).toBeVisible();

    // confirm() is a native dialog — auto-accept so the click proceeds.
    testPage.once("dialog", (d) => void d.accept());
    await settings.deleteButton.click();
    await expect(settings.deleteButton).toHaveCount(0);
    await expect(settings.siteInput).toHaveValue("");
    await expect(settings.secretInput).toHaveValue("");
    await expect(settings.statusBanner).toHaveCount(0);
  });
});
