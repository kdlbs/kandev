import { expect, type Route } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { GitHubAuthSettingsPage } from "../../pages/github-auth-settings-page";

async function fulfillProviderUnauthorized(route: Route, error: string) {
  await route.fulfill({
    status: 401,
    contentType: "application/json",
    body: JSON.stringify({ error }),
  });
}

test.describe("integration provider authentication errors", () => {
  test("keeps GitHub on the page and shows the search error", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetAppRegistration({
      id: "provider-auth-errors-app",
      display_name: "Provider auth errors",
      app_id: 2026,
    });
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "github_app_installation",
      status: "active",
      app_registration_id: "provider-auth-errors-app",
      installation_id: 2026,
      installation_account_login: "workspace-github",
      installation_account_type: "Organization",
    });
    await apiClient.mockGitHubSetPersonalConnection(seedData.workspaceId, {
      login: "personal-github",
      status: "active",
      github_user_id: 101,
      access_expires_at: "2030-01-01T00:00:00Z",
    });
    await testPage.route("**/api/v1/github/user/prs**", (route) =>
      fulfillProviderUnauthorized(route, "GitHub credentials expired"),
    );

    await testPage.goto("/github");

    await expect(testPage).toHaveURL(/\/github$/);
    await expect(testPage.getByText("GitHub credentials expired", { exact: true })).toBeVisible();
  });

  test("keeps GitLab on the page and shows the search error", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.configureGitLab(seedData.workspaceId);
    await apiClient.mockGitLabSetUser(seedData.workspaceId, "gitlab-user");
    await testPage.route("**/api/v1/gitlab/user/mrs**", (route) =>
      fulfillProviderUnauthorized(route, "GitLab credentials expired"),
    );

    await testPage.goto("/gitlab");

    await expect(testPage).toHaveURL(/\/gitlab$/);
    await expect(testPage.getByText("GitLab credentials expired", { exact: true })).toBeVisible();
  });

  test("keeps Jira on the page and shows the authentication error", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.setJiraConfig({
      siteUrl: "https://acme.atlassian.net",
      email: "alice@example.com",
      secret: "jira-api-token",
      workspaceId: seedData.workspaceId,
    });
    await apiClient.waitForIntegrationAuthHealthy("jira", { workspaceId: seedData.workspaceId });
    await testPage.route("**/api/v1/jira/tickets**", (route) =>
      fulfillProviderUnauthorized(route, "jira api: status 401: credentials expired"),
    );

    await testPage.goto("/jira");

    await expect(testPage).toHaveURL(/\/jira$/);
    await expect(
      testPage.getByRole("heading", { name: "Jira authentication required" }),
    ).toBeVisible();
  });

  test("keeps Linear on the page and shows the authentication error", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.setLinearConfig({
      secret: "lin_api_expired",
      workspaceId: seedData.workspaceId,
    });
    await apiClient.waitForIntegrationAuthHealthy("linear", {
      workspaceId: seedData.workspaceId,
    });
    await testPage.route("**/api/v1/linear/issues**", (route) =>
      fulfillProviderUnauthorized(route, "linear api: status 401: credentials expired"),
    );

    await testPage.goto("/linear");

    await expect(testPage).toHaveURL(/\/linear$/);
    await expect(
      testPage.getByRole("heading", { name: "Linear authentication required" }),
    ).toBeVisible();
  });

  test("keeps an invalid GitHub PAT draft visible and the connection active", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "pat",
      status: "active",
      login: "existing-github",
    });
    await testPage.route("**/api/v1/github/workspace-connection?*", async (route) => {
      if (route.request().method() === "PUT") {
        await route.fulfill({
          status: 400,
          contentType: "application/json",
          body: JSON.stringify({ error: "GitHub token is invalid" }),
        });
        return;
      }
      await route.continue();
    });

    const settings = new GitHubAuthSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    await expect(
      testPage.getByTestId("github-workspace-automation").getByText("existing-github", {
        exact: true,
      }),
    ).toBeVisible();
    const surface = await settings.openConnection();
    const token = surface.getByRole("textbox", { name: "Personal access token" });
    await token.fill("ghp_invalid");
    await surface.getByRole("button", { name: "Save changes" }).click();

    await expect(testPage).toHaveURL(
      new RegExp(`/settings/workspaces/${seedData.workspaceId}/integrations/github$`),
    );
    await expect(testPage.getByText("GitHub token is invalid", { exact: true })).toBeVisible();
    await expect(token).toHaveValue("ghp_invalid");
    await expect(surface).toHaveAttribute("data-state", "open");
    await expect(
      testPage.getByTestId("github-workspace-automation").getByText("existing-github", {
        exact: true,
      }),
    ).toBeVisible();
  });
});
