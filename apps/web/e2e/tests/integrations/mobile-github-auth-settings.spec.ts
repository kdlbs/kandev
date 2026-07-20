import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile GitHub authentication settings", () => {
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
    await expect(testPage.getByRole("heading", { name: "Personal GitHub identity" })).toHaveCount(
      0,
    );

    const automation = testPage.getByTestId("github-workspace-automation");
    await automation.getByRole("button", { name: "Change connection" }).click();
    const connectionDialog = testPage.getByRole("dialog", { name: "Change GitHub connection" });
    await connectionDialog.getByRole("combobox", { name: "Connection method" }).click();
    await testPage.getByRole("option", { name: "GitHub App", exact: true }).click();
    await expect(
      connectionDialog.getByText(
        "This Kandev deployment does not have a GitHub App yet. Create it once in System Settings, then return here to install it for this workspace.",
      ),
    ).toBeVisible();
    await expect(
      connectionDialog.getByRole("link", { name: "Set up deployment App" }),
    ).toBeVisible();
    await connectionDialog.getByRole("combobox", { name: "Connection method" }).click();
    await testPage.getByRole("option", { name: "GitHub CLI", exact: true }).click();

    const [accountBox, useAccountBox] = await Promise.all([
      connectionDialog.getByRole("combobox", { name: "GitHub CLI account" }).boundingBox(),
      connectionDialog.getByRole("button", { name: "Use account" }).boundingBox(),
    ]);
    expect(accountBox).not.toBeNull();
    expect(useAccountBox).not.toBeNull();
    expect(accountBox!.height).toBeGreaterThanOrEqual(44);
    expect(accountBox!.height).toBeCloseTo(useAccountBox!.height, 1);

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
    await apiClient.mockGitHubSetAppAvailable(true);
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "github_app_installation",
      status: "active",
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
        "Connect your GitHub user for My GitHub and human-attributed actions. Without it, automation continues as the App.",
        { exact: true },
      )
      .boundingBox();
    const configChatBox = await testPage
      .getByRole("button", { name: "Configuration Chat" })
      .boundingBox();
    expect(identityDescriptionBox).not.toBeNull();
    expect(configChatBox).not.toBeNull();
    expect(identityDescriptionBox!.x + identityDescriptionBox!.width).toBeLessThanOrEqual(
      configChatBox!.x,
    );

    await testPage.screenshot({
      path: testInfo.outputPath("github-app-reconnect-mobile.png"),
      fullPage: true,
    });
  });
});
