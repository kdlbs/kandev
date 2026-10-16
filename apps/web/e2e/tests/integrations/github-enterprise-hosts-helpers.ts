import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { GitHubAuthSettingsPage } from "../../pages/github-auth-settings-page";

/** Mock-provider proof of the visible installation and personal authorization destinations. */
export async function exerciseEnterpriseAppAuthorization({
  page,
  apiClient,
  workspaceId,
  touch,
}: {
  page: Page;
  apiClient: ApiClient;
  workspaceId: string;
  touch: boolean;
}) {
  const host = "git.company.example";
  const registrationId = `enterprise-auth-${workspaceId}`;
  const activate = (target: Locator) => (touch ? target.tap() : target.click());
  await apiClient.mockGitHubSetAppRegistration({
    id: registrationId,
    display_name: "Enterprise authorization App",
    app_id: 702,
    github_host: host,
  });
  await apiClient.mockGitHubSetWorkspaceConnection(workspaceId, {
    source: "github_app_installation",
    status: "active",
    github_host: host,
    app_registration_id: registrationId,
    installation_id: 702,
    installation_account_login: "acme",
    installation_account_type: "Organization",
    capabilities: { repository_read: true, pull_request_write: true },
  });
  await page.route(`https://${host}/**`, (route) => {
    const path = new URL(route.request().url()).pathname;
    return route.fulfill({
      status: 200,
      contentType: "text/html",
      body: `<main><h1>${path === "/login/oauth/authorize" ? "Enterprise user authorization" : "Enterprise App installation"}</h1></main>`,
    });
  });
  const settings = new GitHubAuthSettingsPage(page);
  await settings.goto(workspaceId);
  await expect(settings.automation().getByTestId("github-connection-host")).toHaveText(host);
  await activate(settings.automation().getByRole("button", { name: "Change connection" }));
  const surface = settings.connectionSurface();
  await expect(surface.getByTestId("github-host")).toHaveValue(host);
  await activate(surface.locator(`#github-app-${registrationId}`));
  const installationResponse = page.waitForResponse((response) =>
    response.url().includes("/api/v1/github/app/install/start"),
  );
  await activate(surface.getByTestId("github-app-install-button"));
  const installation = await installationResponse;
  expect(installation.ok(), `Installation returned HTTP ${installation.status()}`).toBe(true);
  await expect(
    page.getByRole("heading", { name: "Enterprise App installation", exact: true }),
  ).toBeVisible();
  expect(new URL(page.url()).host).toBe(host);
  expect(new URL(page.url()).pathname).toMatch(/^\/apps\/[^/]+\/installations\/new$/);
  await settings.goto(workspaceId);
  const authorizationResponse = page.waitForResponse((response) =>
    response.url().includes("/api/v1/github/personal-connection/start"),
  );
  await activate(
    settings.personalIdentity().getByRole("button", { name: "Connect identity", exact: true }),
  );
  const authorization = await authorizationResponse;
  expect(authorization.ok(), `Authorization returned HTTP ${authorization.status()}`).toBe(true);
  await expect(
    page.getByRole("heading", { name: "Enterprise user authorization", exact: true }),
  ).toBeVisible();
  expect(new URL(page.url()).host).toBe(host);
  expect(new URL(page.url()).pathname).toBe("/login/oauth/authorize");
  await apiClient.mockGitHubSetPersonalConnection(workspaceId, {
    login: "enterprise-human",
    status: "active",
    github_user_id: 702,
  });
  await settings.goto(
    workspaceId,
    `?github_result=personal_connected&workspace_id=${encodeURIComponent(workspaceId)}`,
  );
  await expect(page.getByTestId("github-callback-notice")).toContainText(
    "GitHub identity connected",
  );
  await expect(settings.personalIdentity()).toContainText("enterprise-human");
  await expect(settings.automation().getByTestId("github-connection-host")).toHaveText(host);
  await page.reload();
  await expect(settings.personalIdentity()).toContainText("enterprise-human");
}
