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
    await expect(settings.saveButton).toHaveCount(0);
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

    await expect(settings.saveButton).toHaveCount(0);
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

  test("server / data center save flow with PAT persists across reload", async ({
    testPage,
    apiClient,
  }) => {
    const settings = new JiraSettingsPage(testPage);
    await settings.goto();

    await settings.selectInstance("server");
    // Switching to Server auto-selects PAT — the dropdown should now show it
    // as the (only) option, and the email input is no longer rendered.
    await expect(settings.authSelect).toContainText(/Personal Access Token/i);
    await expect(settings.emailInput).toHaveCount(0);

    await settings.siteInput.fill("https://jira.acme.com");
    await settings.secretInput.fill("pat-token-value");
    await expect(settings.saveButton).toBeEnabled();
    await settings.saveButton.click();
    await expect(settings.saveButton).toHaveCount(0);
    await apiClient.waitForIntegrationAuthHealthy("jira");

    await testPage.reload();
    await settings.siteInput.waitFor();
    await expect(settings.siteInput).toHaveValue("https://jira.acme.com");
    await expect(settings.instanceSelect).toContainText(/Server \/ Data Center/i);
    await expect(settings.authSelect).toContainText(/Personal Access Token/i);
    await expect(settings.emailInput).toHaveCount(0);
    await expect(settings.statusBanner).toHaveAttribute("data-state", "ok");
  });

  test("switching instance type swaps the auth-method options", async ({ testPage }) => {
    const settings = new JiraSettingsPage(testPage);
    await settings.goto();

    // Cloud default: api_token + session_cookie options, no PAT.
    await expect(settings.authSelect).toContainText(/API token/i);
    await settings.authSelect.click();
    await expect(testPage.getByRole("option", { name: /API token/i })).toBeVisible();
    await expect(testPage.getByRole("option", { name: /session cookie/i })).toBeVisible();
    await expect(testPage.getByRole("option", { name: /Personal Access Token/i })).toHaveCount(0);
    // Dismiss the listbox before opening another select.
    await testPage.keyboard.press("Escape");

    await settings.selectInstance("server");
    await expect(settings.authSelect).toContainText(/Personal Access Token/i);
    await settings.authSelect.click();
    await expect(testPage.getByRole("option", { name: /Personal Access Token/i })).toBeVisible();
    await expect(testPage.getByRole("option", { name: /API token/i })).toHaveCount(0);
    await expect(testPage.getByRole("option", { name: /session cookie/i })).toHaveCount(0);
    await testPage.keyboard.press("Escape");

    // Round-trip back to Cloud restores the canonical default.
    await settings.selectInstance("cloud");
    await expect(settings.authSelect).toContainText(/API token/i);
  });

  test("email field hides for session cookie and PAT, returns for cloud + api_token", async ({
    testPage,
  }) => {
    const settings = new JiraSettingsPage(testPage);
    await settings.goto();

    // Cloud + api_token (default) shows email.
    await expect(settings.emailInput).toBeVisible();

    // Cloud + session_cookie hides email — the secret is the cookie itself.
    await settings.selectAuth("Browser session cookie");
    await expect(settings.emailInput).toHaveCount(0);

    // Server + PAT also hides email.
    await settings.selectInstance("server");
    await expect(settings.emailInput).toHaveCount(0);

    // Back to Cloud + api_token brings email back.
    await settings.selectInstance("cloud");
    await settings.selectAuth("API token (recommended)");
    await expect(settings.emailInput).toBeVisible();
  });

  test("saved secret is not reused after switching identity fields", async ({
    testPage,
    apiClient,
  }) => {
    const settings = new JiraSettingsPage(testPage);
    await settings.goto();
    await settings.fillForm({
      siteUrl: "https://acme.atlassian.net",
      email: "alice@example.com",
      secret: "api-token-value",
    });
    await settings.saveButton.click();
    await expect(settings.saveButton).toHaveCount(0);
    await apiClient.waitForIntegrationAuthHealthy("jira");

    await testPage.reload();
    await settings.siteInput.waitFor();
    // Saved secret reuse: placeholder shows the masked dots. A clean form has
    // no route-level save action until the user changes an identity field.
    await expect(settings.secretInput).toHaveAttribute("placeholder", /•/);
    await expect(settings.saveButton).toHaveCount(0);

    // Change the site URL — the saved secret no longer applies to this host.
    await settings.siteInput.fill("https://other.atlassian.net");
    await expect(settings.secretInput).toHaveAttribute("placeholder", /paste/i);
    await expect(settings.saveButton).toBeDisabled();

    // Restoring the host re-enables reuse without re-typing the token.
    await settings.siteInput.fill("https://acme.atlassian.net");
    await expect(settings.secretInput).toHaveAttribute("placeholder", /•/);
    await expect(settings.saveButton).toHaveCount(0);

    // Switching instance type also invalidates reuse.
    await settings.selectInstance("server");
    await expect(settings.secretInput).toHaveAttribute("placeholder", /paste/i);
    await expect(settings.saveButton).toBeDisabled();
  });

  test("PAT help link targets the configured site", async ({ testPage }) => {
    const settings = new JiraSettingsPage(testPage);
    await settings.goto();

    await settings.selectInstance("server");
    await settings.siteInput.fill("https://jira.acme.com/");
    // patHref strips trailing slashes, so the link should point at the bare
    // host + /secure/ViewProfile.jspa.
    const helpLink = testPage.getByRole("link", {
      name: "https://jira.acme.com/secure/ViewProfile.jspa",
    });
    await expect(helpLink).toBeVisible();
    await expect(helpLink).toHaveAttribute("href", "https://jira.acme.com/secure/ViewProfile.jspa");
  });
});
