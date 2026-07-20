import { test, expect } from "../../fixtures/test-base";
import { expectElementsNotToIntersect } from "../../helpers/layout-assertions";

const settingsPath = (workspaceId: string) =>
  `/settings/workspace/${workspaceId}/integrations/github`;

test.describe("GitHub workspace authentication", () => {
  test("keeps actors isolated while switching workspaces and lists named CLI accounts", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const workspaceB = await apiClient.createWorkspace("GitHub Workspace B");
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "pat",
      status: "active",
      login: "alice-automation",
    });
    await apiClient.mockGitHubSetWorkspaceConnection(workspaceB.id, {
      source: "gh_cli",
      status: "active",
      login: "bob-cli",
    });
    await apiClient.mockGitHubSetCLIAccounts([
      { host: "github.com", login: "alice-automation", active: true, state: "active" },
      { host: "github.com", login: "bob-cli", active: false, state: "active" },
    ]);

    await testPage.goto(settingsPath(seedData.workspaceId));
    const automation = testPage.getByTestId("github-workspace-automation");
    await expect(automation.getByText("alice-automation", { exact: true })).toBeVisible({
      timeout: 15_000,
    });
    await expect(
      automation.getByText("Personal access token", { exact: true }).first(),
    ).toBeVisible();

    await automation.getByRole("tab", { name: "gh CLI", exact: true }).click();
    await automation.getByRole("combobox", { name: "GitHub CLI account" }).click();
    await expect(testPage.getByRole("option", { name: "bob-cli (github.com)" })).toBeVisible();
    await testPage.keyboard.press("Escape");

    await testPage.route("**/api/v1/github/status?*", async (route) => {
      const url = new URL(route.request().url());
      if (url.searchParams.get("workspace_id") === workspaceB.id) {
        await new Promise((resolve) => setTimeout(resolve, 350));
      }
      await route.continue();
    });
    await testPage.goto(settingsPath(workspaceB.id));

    await expect(testPage.getByText("Checking GitHub connection...").first()).toBeVisible();
    await expect(automation.getByText("alice-automation", { exact: true })).toHaveCount(0);
    await expect(automation.getByText("bob-cli", { exact: true })).toBeVisible({
      timeout: 15_000,
    });
    await expect(automation.getByText("GitHub CLI", { exact: true })).toBeVisible();
    const accountsResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/github/auth/gh-cli/accounts?workspace_id=${encodeURIComponent(workspaceB.id)}`,
    );
    expect(accountsResponse.status).toBe(200);
    const accountsPayload = (await accountsResponse.json()) as {
      accounts: Array<{ login: string; selected: boolean }>;
    };
    const statusResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/github/status?workspace_id=${encodeURIComponent(workspaceB.id)}`,
    );
    const statusPayload = await statusResponse.json();
    expect(accountsPayload.accounts).toEqual(
      expect.arrayContaining([expect.objectContaining({ login: "bob-cli", selected: true })]),
    );
    expect(statusPayload).toMatchObject({
      automation: { source: "gh_cli", login: "bob-cli", github_host: "github.com" },
    });
    await automation.getByRole("tab", { name: "gh CLI", exact: true }).click();
    await expect(automation.getByRole("combobox", { name: "GitHub CLI account" })).toContainText(
      "bob-cli",
    );

    await testPage.screenshot({
      path: testInfo.outputPath("github-workspace-isolation-desktop.png"),
      fullPage: true,
    });
  });

  test("migrates a legacy connection explicitly and disconnects the replacement", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "legacy_shared",
      status: "active",
    });
    await testPage.goto(settingsPath(seedData.workspaceId));
    const automation = testPage.getByTestId("github-workspace-automation");

    await expect(automation.getByText("Legacy shared connection", { exact: true })).toBeVisible({
      timeout: 15_000,
    });

    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "pat",
      status: "active",
      login: "workspace-user",
    });
    await automation.getByRole("button", { name: "Refresh GitHub connection" }).click();
    await expect(automation.getByText("workspace-user", { exact: true })).toBeVisible();
    await expect(
      automation.getByText("Personal access token", { exact: true }).first(),
    ).toBeVisible();

    await automation.getByRole("button", { name: "Disconnect", exact: true }).click();
    await expect(automation.getByText("No automation connection", { exact: true })).toBeVisible();
  });

  test("shows App capabilities, personal reconnect, and suspended or revoked states", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    await apiClient.mockGitHubSetAppAvailable(true);
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "github_app_installation",
      status: "active",
      installation_id: 42,
      installation_account_login: "acme",
      installation_account_type: "Organization",
      capabilities: {
        repository_read: true,
        pull_request_write: true,
        workflow_write: false,
      },
    });

    await testPage.goto(settingsPath(seedData.workspaceId));
    const automation = testPage.getByTestId("github-workspace-automation");
    const personal = testPage.getByTestId("github-personal-identity");
    await expect(automation.getByText("acme", { exact: true })).toBeVisible({ timeout: 15_000 });
    await expect(automation.getByText("GitHub App", { exact: true }).first()).toBeVisible();
    await expect(automation.getByText("Pull Request Write", { exact: true })).toBeVisible();
    await expect(automation.getByText("Workflow Write", { exact: true })).toBeVisible();
    await expect(personal.getByText("Not connected", { exact: true })).toBeVisible();
    await expect(personal.getByRole("button", { name: /Connect identity/ })).toBeVisible();

    await testPage.goto("/github");
    await expect(
      testPage.getByText(/Connect your personal GitHub identity to see pull requests/),
    ).toBeVisible({ timeout: 15_000 });

    await apiClient.mockGitHubSetPersonalConnection(seedData.workspaceId, {
      login: "alice-personal",
      status: "active",
      github_user_id: 7,
      access_expires_at: "2030-01-01T00:00:00Z",
    });
    await testPage.goto(settingsPath(seedData.workspaceId));
    await expect(personal.getByText("alice-personal", { exact: true })).toBeVisible({
      timeout: 15_000,
    });
    await expect(personal.getByRole("button", { name: /Reconnect identity/ })).toBeVisible();

    await apiClient.mockGitHubSetWorkspaceConnectionStatus(seedData.workspaceId, "suspended");
    await apiClient.mockGitHubSetPersonalConnection(seedData.workspaceId, {
      login: "alice-personal",
      status: "revoked",
      github_user_id: 7,
      access_expires_at: "2026-01-01T00:00:00Z",
    });
    await automation.getByRole("button", { name: "Refresh GitHub connection" }).click();
    await expect(automation.getByText("suspended", { exact: true })).toBeVisible();
    await expect(personal.getByText("revoked", { exact: true })).toBeVisible();

    await testPage.getByTestId("github-scope-mode").click();
    await testPage.getByRole("option", { name: "Organizations" }).click();
    const floatingSave = testPage.getByTestId("settings-floating-save");
    await expect(floatingSave).toBeVisible();

    await expectElementsNotToIntersect(
      floatingSave,
      testPage.getByRole("button", { name: "Configuration Chat" }),
    );

    await testPage.screenshot({
      path: testInfo.outputPath("github-app-reconnect-desktop.png"),
      fullPage: true,
    });
  });

  test("rejects invalid App and personal callback state", async ({ apiClient }) => {
    const appResponse = await apiClient.rawRequest(
      "GET",
      "/api/v1/github/app/install/callback?state=invalid&code=secret-code&installation_id=42",
      undefined,
      { redirect: "manual" },
    );
    expect(appResponse.status).toBe(303);
    expectInvalidCallbackLocation(appResponse);

    const personalResponse = await apiClient.rawRequest(
      "GET",
      "/api/v1/github/personal-connection/callback?state=invalid&code=secret-code",
      undefined,
      { redirect: "manual" },
    );
    expect(personalResponse.status).toBe(303);
    expectInvalidCallbackLocation(personalResponse);
  });
});

function expectInvalidCallbackLocation(response: Response) {
  const location = response.headers.get("location");
  expect(location).not.toBeNull();
  const redirect = new URL(location!, "http://kandev.test");
  expect(redirect.pathname).toBe("/settings/integrations/github");
  expect(redirect.searchParams.get("github_result")).toBe("github_invalid_callback");
  expect(redirect.searchParams.has("state")).toBe(false);
  expect(redirect.searchParams.has("code")).toBe(false);
  expect(location).not.toContain("state=invalid");
  expect(location).not.toContain("secret-code");
  expect(location).not.toMatch(/token/i);
}
