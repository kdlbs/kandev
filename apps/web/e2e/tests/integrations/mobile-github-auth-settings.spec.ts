import { test, expect } from "../../fixtures/test-base";
import { GitHubAuthSettingsPage } from "../../pages/github-auth-settings-page";

test.describe("Mobile GitHub authentication settings", () => {
  test("keeps loaded settings visible during refresh", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "pat",
      status: "active",
      login: "mobile-refresh-user",
    });
    let holdStatus = false;
    let releaseStatus!: () => void;
    const statusGate = new Promise<void>((resolve) => {
      releaseStatus = resolve;
    });
    await testPage.route("**/api/v1/github/status?*", async (route) => {
      if (holdStatus) await statusGate;
      await route.continue();
    });

    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/integrations/github`);
    const automation = testPage.getByTestId("github-workspace-automation");
    await expect(automation.getByText("mobile-refresh-user", { exact: true })).toBeVisible({
      timeout: 15_000,
    });

    holdStatus = true;
    const refresh = automation.getByRole("button", { name: "Refresh GitHub connection" });
    await refresh.tap();
    await expect(automation.getByText("mobile-refresh-user", { exact: true })).toBeVisible();
    await expect(testPage.getByText("Checking GitHub connection...", { exact: true })).toHaveCount(
      0,
    );
    await expect(refresh).toBeDisabled();
    await expect(refresh).toHaveAttribute("aria-busy", "true");
    const refreshBox = await refresh.boundingBox();
    expect(refreshBox).not.toBeNull();
    expect(refreshBox!.height).toBeGreaterThanOrEqual(44);

    const hasHorizontalOverflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
    );
    expect(hasHorizontalOverflow).toBe(false);

    holdStatus = false;
    releaseStatus();
    await expect(refresh).toBeEnabled();
  });

  test("keeps workspace and personal identity controls usable on a narrow viewport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "pat",
      status: "active",
      login: "mobile-user",
    });

    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/integrations/github`);

    await expect(testPage.getByRole("heading", { name: "Workspace GitHub access" })).toBeVisible({
      timeout: 15_000,
    });
    const automation = testPage.getByTestId("github-workspace-automation");
    await expect(testPage.getByText("My GitHub identity", { exact: true })).toHaveCount(0);
    await expect(
      automation.getByText("This account also powers My GitHub and user-triggered actions.", {
        exact: true,
      }),
    ).toBeVisible();
    await automation.getByRole("button", { name: "Change connection" }).click();
    const githubSettings = new GitHubAuthSettingsPage(testPage);
    const connectionDialog = githubSettings.connectionSurface();
    await githubSettings.chooseMethod("GitHub App");
    await expect(
      connectionDialog.getByText(
        "No GitHub Apps are registered yet. Add an existing App or create one below.",
      ),
    ).toBeVisible();
    await githubSettings.chooseMethod("GitHub CLI account");

    const [accountBox, saveBox] = await Promise.all([
      connectionDialog.getByRole("combobox", { name: "GitHub CLI account" }).boundingBox(),
      connectionDialog.getByRole("button", { name: "Save changes" }).boundingBox(),
    ]);
    expect(accountBox).not.toBeNull();
    expect(saveBox).not.toBeNull();
    expect(accountBox!.height).toBeGreaterThanOrEqual(44);
    expect(saveBox!.height).toBeGreaterThanOrEqual(44);

    const hasHorizontalOverflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
    );
    expect(hasHorizontalOverflow).toBe(false);
  });

  test("fits App capability and reconnect states without horizontal overflow", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    await apiClient.mockGitHubSetAppRegistration({
      id: "registration-mobile-acme",
      display_name: "Acme mobile automation",
      app_id: 4242,
    });
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "github_app_installation",
      status: "active",
      app_registration_id: "registration-mobile-acme",
      installation_id: 42,
      installation_account_login: "acme-engineering",
      installation_account_type: "Organization",
      capabilities: {
        repository_read: true,
        pull_request_write: true,
        workflow_write: false,
      },
    });
    await apiClient.mockGitHubSetPersonalConnection(seedData.workspaceId, {
      login: "mobile-user-with-a-long-login",
      status: "revoked",
      github_user_id: 17,
      access_expires_at: "2026-01-01T00:00:00Z",
    });

    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/integrations/github`);

    const automation = testPage.getByTestId("github-workspace-automation");
    const personal = testPage.getByTestId("github-personal-identity");
    await expect(automation.getByText("acme-engineering", { exact: true })).toBeVisible({
      timeout: 15_000,
    });
    await automation.getByRole("button", { name: "View permissions" }).click();
    const permissionsDialog = testPage.getByRole("dialog", { name: "GitHub App permissions" });
    await expect(permissionsDialog.getByText("Pull Request Write", { exact: true })).toBeVisible();
    await permissionsDialog.getByRole("button", { name: "Close" }).click();
    await expect(personal.getByText("revoked", { exact: true })).toBeVisible();
    await expect(personal.getByRole("button", { name: "Reconnect identity" })).toBeVisible();

    for (const control of [
      automation.getByRole("button", { name: "Change connection" }),
      personal.getByRole("button", { name: "Reconnect identity" }),
    ]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(390);
    }

    const hasHorizontalOverflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
    );
    expect(hasHorizontalOverflow).toBe(false);

    const identityDescriptionBox = await testPage
      .getByText(
        "Optionally connect your GitHub user for My GitHub and human-attributed actions. This identity is never given to managed agents; workspace automation continues as the App.",
        { exact: true },
      )
      .boundingBox();
    const configChatBox = await testPage
      .getByRole("button", { name: "Configuration Chat" })
      .boundingBox();
    expect(identityDescriptionBox).not.toBeNull();
    expect(configChatBox).not.toBeNull();
    const boxesOverlap = !(
      identityDescriptionBox!.x + identityDescriptionBox!.width <= configChatBox!.x ||
      configChatBox!.x + configChatBox!.width <= identityDescriptionBox!.x ||
      identityDescriptionBox!.y + identityDescriptionBox!.height <= configChatBox!.y ||
      configChatBox!.y + configChatBox!.height <= identityDescriptionBox!.y
    );
    expect(boxesOverlap).toBe(false);

    await testPage.screenshot({
      path: testInfo.outputPath("github-app-reconnect-mobile.png"),
      fullPage: true,
    });
  });
});
