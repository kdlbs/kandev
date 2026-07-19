import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile GitHub authentication settings", () => {
  test("keeps workspace and personal identity controls usable on a narrow viewport", async ({
    testPage,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/integrations/github`);

    await expect(testPage.getByRole("heading", { name: "Workspace automation" })).toBeVisible({
      timeout: 15_000,
    });
    await expect(testPage.getByRole("heading", { name: "My GitHub identity" })).toBeVisible();

    const automation = testPage.getByTestId("github-workspace-automation");
    await expect(automation.getByRole("tab", { name: "PAT", exact: true })).toBeVisible();
    await expect(automation.getByRole("tab", { name: "gh CLI", exact: true })).toBeVisible();
    await expect(automation.getByLabel("Personal access token")).toBeVisible();

    const tokenHeight = await automation
      .getByLabel("Personal access token")
      .evaluate((element) => element.getBoundingClientRect().height);
    expect(tokenHeight).toBeGreaterThanOrEqual(44);

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
    await expect(automation.getByText("Pull Request Write", { exact: true })).toBeVisible();
    await expect(personal.getByText("revoked", { exact: true })).toBeVisible();
    await expect(personal.getByRole("button", { name: "Reconnect identity" })).toBeVisible();

    for (const control of [
      automation.getByRole("tab", { name: "GitHub App", exact: true }),
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
        "Used for My GitHub and user-triggered actions when a personal actor is required.",
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
