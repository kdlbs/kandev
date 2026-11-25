// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.1, AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.2, AC-INTEGRATIONS-GITHUB-ENTERPRISE-005.3
import {
  exerciseEnterpriseAppAuthorization,
  exerciseEnterprisePendingSave,
} from "./github-enterprise-hosts-helpers";
import { test, expect } from "../../fixtures/test-base";
import { GitHubAuthSettingsPage } from "../../pages/github-auth-settings-page";

const enterpriseHost = "git.company.example";

test.beforeEach(async ({ apiClient }) => {
  await apiClient.mockGitHubReset();
});
test.afterEach(async ({ apiClient }) => {
  await apiClient.mockGitHubReset();
});

test("locks the connection method during a pending save and permits retry after failure", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  await exerciseEnterprisePendingSave({
    page: testPage,
    apiClient,
    workspaceId: seedData.workspaceId,
    touch: false,
    screenshotPath: testInfo.outputPath("enterprise-pending-save-desktop.png"),
  });
});

test("saves the existing Enterprise CLI account selection and retains its host after reload", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  await apiClient.mockGitHubSetCLIAccounts([
    { host: "github.com", login: "public-user", active: true, state: "active" },
    { host: enterpriseHost, login: "enterprise-user", active: false, state: "active" },
  ]);
  await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
    source: "pat",
    status: "active",
    login: "public-user",
  });
  const settings = new GitHubAuthSettingsPage(testPage);
  await settings.goto(seedData.workspaceId);
  const surface = await settings.openConnection();
  await settings.chooseMethod("GitHub CLI account");
  await surface.getByRole("combobox", { name: "GitHub CLI account" }).click();
  await testPage.getByRole("option", { name: `enterprise-user (${enterpriseHost})` }).click();
  await expect(surface.getByTestId("github-selected-host")).toContainText(enterpriseHost);
  await expect(surface.getByTestId("github-host")).toHaveCount(0);
  await expect(surface.getByTestId("github-host-replacement")).toContainText(
    "keep their original host",
  );
  const accountBounds = await surface
    .getByRole("combobox", { name: "GitHub CLI account" })
    .boundingBox();
  const hostBounds = await surface.getByTestId("github-selected-host").boundingBox();
  expect(accountBounds!.y).toBeLessThan(hostBounds!.y);
  await testPage.screenshot({ path: testInfo.outputPath("enterprise-cli-setup-desktop.png") });
  await surface.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(settings.automation().getByTestId("github-connection-host")).toHaveText(
    enterpriseHost,
  );
  await testPage.reload();
  await expect(settings.automation().getByTestId("github-connection-host")).toHaveText(
    enterpriseHost,
  );
  await expect(settings.automation()).toContainText("enterprise-user");
  await testPage.screenshot({ path: testInfo.outputPath("enterprise-cli-desktop.png") });
});

test("retains a non-secret host draft after failed PAT verification and filters App choices", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
    source: "pat",
    status: "active",
    login: "public-user",
  });
  await apiClient.mockGitHubSetAppRegistration({
    id: "public-enterprise-test",
    display_name: "Public automation",
    app_id: 701,
    github_host: "github.com",
  });
  await apiClient.mockGitHubSetAppRegistration({
    id: "private-enterprise-test",
    display_name: "Private automation",
    app_id: 701,
    github_host: enterpriseHost,
  });
  const settings = new GitHubAuthSettingsPage(testPage);
  await settings.goto(seedData.workspaceId);
  const surface = await settings.openConnection();
  await surface.getByTestId("github-host").fill(enterpriseHost);
  await surface.getByLabel("Personal access token", { exact: true }).fill("ghp_fixture_invalid");
  await testPage.route("**/api/v1/github/workspace-connection?*", (route) =>
    route.fulfill({
      status: 400,
      contentType: "application/json",
      body: JSON.stringify({ error: "Host certificate is not trusted" }),
    }),
  );
  await surface.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(
    testPage.getByText("Host certificate is not trusted", { exact: true }),
  ).toBeVisible();
  await expect(surface.getByTestId("github-host")).toHaveValue(enterpriseHost);
  await settings.chooseMethod("GitHub App");
  await expect(surface.getByText("Private automation", { exact: true })).toBeVisible();
  await expect(surface.getByText("Public automation", { exact: true })).toHaveCount(0);
});

test("keeps App installation and personal authorization on the Enterprise host", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await exerciseEnterpriseAppAuthorization({
    page: testPage,
    apiClient,
    workspaceId: seedData.workspaceId,
    touch: false,
  });
});
