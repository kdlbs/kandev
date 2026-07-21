import { test, expect } from "../../fixtures/test-base";
import { GitHubAuthSettingsPage } from "../../pages/github-auth-settings-page";

const appConnection = (registrationId: string, account: string, installationId: number) => ({
  source: "github_app_installation" as const,
  status: "active" as const,
  app_registration_id: registrationId,
  installation_id: installationId,
  installation_account_login: account,
  installation_account_type: "Organization",
  capabilities: { repository_read: true, pull_request_write: true },
});

test.describe("Workspace GitHub App onboarding", () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.mockGitHubReset();
  });

  test("selects reusable Apps and explains cross-workspace sharing before installation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const otherWorkspace = await apiClient.createWorkspace("Shared App Workspace");
    await apiClient.mockGitHubSetAppRegistration({
      id: "registration-shared",
      display_name: "Company automation",
      app_id: 101,
    });
    await apiClient.mockGitHubSetAppRegistration({
      id: "registration-personal",
      display_name: "Personal projects",
      app_id: 202,
    });
    await apiClient.mockGitHubSetWorkspaceConnection(
      seedData.workspaceId,
      appConnection("registration-shared", "acme", 41),
    );
    await apiClient.mockGitHubSetWorkspaceConnection(
      otherWorkspace.id,
      appConnection("registration-shared", "acme-sandbox", 42),
    );

    const settings = new GitHubAuthSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    await expect(
      settings.automation().getByText("Company automation", { exact: true }),
    ).toBeVisible({
      timeout: 15_000,
    });
    await expect(settings.automation().getByText(/shared by 2 workspaces/i)).toBeVisible();

    const connection = await settings.openConnection();
    await settings.chooseMethod("GitHub App");
    await expect(connection.getByText("Company automation", { exact: true })).toBeVisible();
    await expect(connection.getByText("Personal projects", { exact: true })).toBeVisible();
    await expect(connection.getByText(/Used by 2 workspaces/)).toBeVisible();
    await expect(
      connection.getByText(/installation access remains workspace-specific/i),
    ).toHaveCount(0);
    await settings.chooseApp("registration-personal");

    let installBody: Record<string, unknown> | undefined;
    await testPage.route("**/api/v1/github/app/install/start", async (route) => {
      installBody = route.request().postDataJSON() as Record<string, unknown>;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          url: "https://github.com/apps/mock-registration-personal/installations/new",
        }),
      });
    });
    await testPage.route(
      "https://github.com/apps/mock-registration-personal/installations/new",
      (route) =>
        route.fulfill({
          status: 200,
          contentType: "text/html",
          body: "<main><h1>Install Personal projects</h1></main>",
        }),
    );
    await connection.getByTestId("github-app-install-button").click();
    await expect(
      testPage.getByRole("heading", { name: "Install Personal projects" }),
    ).toBeVisible();
    expect(installBody).toEqual({
      workspace_id: seedData.workspaceId,
      app_registration_id: "registration-personal",
    });
  });

  test("guides manifest creation and returns to the initiating workspace callback", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const settings = new GitHubAuthSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    const connection = await settings.openConnection();
    await settings.chooseMethod("GitHub App");
    await connection.getByRole("button", { name: "Create new App" }).click();

    await connection.getByLabel("Name in Kandev").fill("Work automation");
    await connection.getByLabel("GitHub owner login").fill("acme");
    await connection.getByLabel("Public Kandev URL").fill("https://1.1.1.1");
    await connection.getByRole("button", { name: "Review permissions" }).click();
    const policy = testPage.getByRole("dialog", { name: "Required GitHub App policy" });
    await expect(policy.getByText("Contents", { exact: true })).toBeVisible();
    await expect(policy.getByText("Workflows", { exact: true })).toBeVisible();
    await policy.getByRole("button", { name: "Close" }).click();

    let manifest: Record<string, unknown> | undefined;
    await testPage.route(
      "https://github.com/organizations/acme/settings/apps/new",
      async (route) => {
        const fields = new URLSearchParams(route.request().postData() ?? "");
        manifest = JSON.parse(fields.get("manifest") ?? "{}") as Record<string, unknown>;
        await route.fulfill({
          status: 200,
          contentType: "text/html",
          body: "<main><h1>Confirm Work automation on GitHub</h1></main>",
        });
      },
    );

    await connection.getByRole("button", { name: "Prepare App on GitHub" }).click();
    await expect(connection.getByText("GitHub is ready to create the App")).toBeVisible();
    await connection.getByRole("button", { name: "Continue on GitHub" }).click();
    await expect(
      testPage.getByRole("heading", { name: "Confirm Work automation on GitHub" }),
    ).toBeVisible();

    const redirectUrl = String(manifest?.redirect_url ?? "");
    const registrationId = redirectUrl.match(
      /\/app\/registrations\/([^/]+)\/manifest\/callback/,
    )?.[1];
    expect(registrationId).toBeTruthy();
    expect(manifest).toMatchObject({
      url: "https://1.1.1.1",
      public: false,
      hook_attributes: {
        url: `https://1.1.1.1/api/v1/github/app/registrations/${registrationId}/webhook`,
        active: true,
      },
      callback_urls: [
        `https://1.1.1.1/api/v1/github/app/registrations/${registrationId}/personal/callback`,
      ],
      setup_url: `https://1.1.1.1/api/v1/github/app/registrations/${registrationId}/install/callback`,
    });

    await apiClient.mockGitHubSetAppRegistration({
      id: registrationId!,
      display_name: "Work automation",
      app_id: 303,
    });
    await settings.goto(
      seedData.workspaceId,
      `?github_result=app_registered&workspace_id=${encodeURIComponent(seedData.workspaceId)}`,
    );
    const notice = testPage.getByTestId("github-callback-notice");
    await expect(notice).toContainText("GitHub App added");
    await expect(notice).toContainText("ready to select and install for this workspace");
    await expect(settings.automation().getByText("No automation connection")).toBeVisible();
  });

  test("prepares exact existing-App instructions and imports the reserved registration", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const settings = new GitHubAuthSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    const connection = await settings.openConnection();
    await settings.chooseMethod("GitHub App");
    await connection.getByRole("button", { name: "Add existing App" }).click();
    await connection.getByLabel("Public Kandev URL").fill("https://1.1.1.1");

    const prepareResponse = testPage.waitForResponse(
      (response) =>
        response.url().endsWith("/api/v1/github/app/registrations/import/prepare") &&
        response.request().method() === "POST",
    );
    await connection.getByRole("button", { name: "Generate setup instructions" }).click();
    const preparation = (await (await prepareResponse).json()) as {
      registration_id: string;
      personal_callback_url: string;
      install_callback_url: string;
      webhook_url: string;
    };
    await expect(
      connection.getByText(preparation.personal_callback_url, { exact: true }),
    ).toBeVisible();
    await expect(
      connection.getByText(preparation.install_callback_url, { exact: true }),
    ).toBeVisible();
    await expect(connection.getByText(preparation.webhook_url, { exact: true })).toBeVisible();
    await expect(connection.getByText(/application\/json/)).toBeVisible();
    await expect(connection.getByText(/SSL verification enabled/)).toBeVisible();

    let importedBody: Record<string, unknown> | undefined;
    await testPage.route("**/api/v1/github/app/registrations/import", async (route) => {
      importedBody = route.request().postDataJSON() as Record<string, unknown>;
      const registration = await apiClient.mockGitHubSetAppRegistration({
        id: preparation.registration_id,
        display_name: "Existing company App",
        app_id: 404,
      });
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify(registration),
      });
    });

    await connection.getByLabel("Name in Kandev").fill("Existing company App");
    await connection.getByLabel("GitHub App slug").fill("company-kandev");
    await connection.getByLabel("GitHub owner login").fill("acme");
    await connection.getByLabel("App ID").fill("404");
    await connection.getByLabel("Client ID").fill("Iv1.client-id");
    await connection.getByLabel("Client secret").fill("client-secret-value");
    await connection.getByLabel("Webhook secret").fill("webhook-secret-value");
    await connection.getByLabel("Private key (.pem)").fill("private-key-value");
    await connection.getByRole("button", { name: "Verify and import App" }).click();
    await expect(testPage.getByText("GitHub App imported. It is ready to install.")).toBeVisible();
    expect(importedBody).toMatchObject({
      registration_id: preparation.registration_id,
      workspace_id: seedData.workspaceId,
      display_name: "Existing company App",
      app_id: 404,
      client_id: "Iv1.client-id",
      client_secret: "client-secret-value",
      private_key: "private-key-value",
      webhook_secret: "webhook-secret-value",
      slug: "company-kandev",
      owner_login: "acme",
      owner_type: "Organization",
      visibility: "private",
      public_base_url: "https://1.1.1.1",
    });
    await expect(connection.getByLabel("Client secret")).toHaveCount(0);
    await expect(connection.getByText("Existing company App", { exact: true })).toBeVisible();
    await expect(connection.locator(`#github-app-${preparation.registration_id}`)).toHaveAttribute(
      "data-state",
      "checked",
    );
  });

  test("resets App selection and onboarding drafts when the workspace changes", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const otherWorkspace = await apiClient.createWorkspace("Personal Workspace");
    for (const registration of [
      { id: "registration-work", display_name: "Work App", app_id: 501 },
      { id: "registration-home", display_name: "Home App", app_id: 502 },
    ]) {
      await apiClient.mockGitHubSetAppRegistration(registration);
    }
    await apiClient.mockGitHubSetWorkspaceConnection(
      seedData.workspaceId,
      appConnection("registration-work", "work-org", 51),
    );
    await apiClient.mockGitHubSetWorkspaceConnection(
      otherWorkspace.id,
      appConnection("registration-home", "home-user", 52),
    );

    const settings = new GitHubAuthSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    let connection = await settings.openConnection();
    await settings.chooseMethod("GitHub App");
    await connection.getByRole("button", { name: "Create new App" }).click();
    await connection.getByLabel("Name in Kandev").fill("Unsaved draft");

    await testPage.evaluate((path) => {
      window.history.pushState({}, "", path);
      window.dispatchEvent(new Event("kandev:navigation"));
    }, `/settings/workspace/${otherWorkspace.id}/integrations/github`);
    await expect(settings.automation().getByText("home-user", { exact: true })).toBeVisible({
      timeout: 15_000,
    });
    await expect(testPage.getByText("Unsaved draft", { exact: true })).toHaveCount(0);

    connection = await settings.openConnection();
    await settings.chooseMethod("GitHub App");
    await expect(connection.locator("#github-app-registration-home")).toHaveAttribute(
      "data-state",
      "checked",
    );
    await connection.getByRole("button", { name: "Create new App" }).click();
    await expect(connection.getByLabel("Name in Kandev")).toHaveValue("");
  });
});
