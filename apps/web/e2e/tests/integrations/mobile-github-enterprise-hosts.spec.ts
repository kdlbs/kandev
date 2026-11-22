// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.1, AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.2, AC-INTEGRATIONS-GITHUB-ENTERPRISE-005.3
import {
  exerciseEnterpriseAppAuthorization,
  exerciseEnterprisePendingSave,
} from "./github-enterprise-hosts-helpers";
import { test, expect } from "../../fixtures/test-base";
import { GitHubAuthSettingsPage } from "../../pages/github-auth-settings-page";

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
    touch: true,
    screenshotPath: testInfo.outputPath("enterprise-pending-save-mobile.png"),
  });
});

test("keeps Enterprise setup drafts across resize with one reachable phone scroll body", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
    source: "pat",
    status: "active",
    login: "public-user",
  });
  const settings = new GitHubAuthSettingsPage(testPage);
  await settings.goto(seedData.workspaceId);
  await settings.automation().getByRole("button", { name: "Change connection" }).tap();
  const surface = settings.connectionSurface();
  await expect(testPage.getByTestId("github-connection-mobile")).toHaveCount(1);
  await surface.getByTestId("github-host").fill("git.company.example");
  await surface.getByLabel("Personal access token", { exact: true }).fill("ghp_enterprise_fixture");
  const hostBox = await surface.getByTestId("github-host").boundingBox();
  const save = surface.getByRole("button", { name: "Save changes", exact: true });
  const saveBox = await save.boundingBox();
  expect(hostBox!.height).toBeGreaterThanOrEqual(44);
  expect(saveBox!.height).toBeGreaterThanOrEqual(44);
  expect(
    await testPage.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  await expect(surface.locator(".overflow-y-auto")).toHaveCount(1);
  await testPage.setViewportSize({ width: 1100, height: 850 });
  await expect(testPage.getByTestId("github-connection-desktop")).toHaveCount(1);
  await expect(surface.getByTestId("github-host")).toHaveValue("git.company.example");
  await testPage.setViewportSize({ width: 393, height: 851 });
  await expect(testPage.getByTestId("github-connection-mobile")).toHaveCount(1);
  await expect(surface.getByLabel("Personal access token", { exact: true })).toHaveValue(
    "ghp_enterprise_fixture",
  );
  await expect(save).toBeInViewport();
  await expect
    .poll(async () => (await testPage.getByTestId("github-connection-mobile").boundingBox())?.y)
    .toBeLessThan(200);
  await testPage.screenshot({ path: testInfo.outputPath("enterprise-pat-setup-mobile.png") });
  await save.tap();
  await expect(settings.automation().getByTestId("github-connection-host")).toHaveText(
    "git.company.example",
  );
  await testPage.reload();
  await expect(settings.automation().getByTestId("github-connection-host")).toHaveText(
    "git.company.example",
  );
  await testPage.screenshot({ path: testInfo.outputPath("enterprise-pat-mobile.png") });
});

test("retains Enterprise App setup fields while switching phone and desktop overlays", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
    source: "pat",
    status: "active",
    login: "public-user",
  });
  const settings = new GitHubAuthSettingsPage(testPage);
  await settings.goto(seedData.workspaceId);
  await settings.automation().getByRole("button", { name: "Change connection" }).tap();
  const surface = settings.connectionSurface();
  await surface.getByTestId("github-host").fill("git.company.example");
  await surface.getByRole("radio", { name: /^GitHub App/ }).tap();
  await surface.getByRole("button", { name: "Create new App", exact: true }).tap();
  await surface.getByLabel("Name in Kandev", { exact: true }).fill("Enterprise automation");
  await testPage.setViewportSize({ width: 1100, height: 850 });
  await expect(testPage.getByTestId("github-connection-desktop")).toHaveCount(1);
  await expect(surface.getByLabel("Name in Kandev", { exact: true })).toHaveValue(
    "Enterprise automation",
  );
  await testPage.setViewportSize({ width: 393, height: 851 });
  await expect(surface.getByLabel("Name in Kandev", { exact: true })).toHaveValue(
    "Enterprise automation",
  );
  await expect(surface.locator(".overflow-y-auto")).toHaveCount(1);
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
    touch: true,
  });
});
