import { test, expect } from "../../fixtures/test-base";

test.describe("GitHub workspace settings", () => {
  test("repository scope is saved per workspace and filters the GitHub PR list", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 6101,
        title: "Scoped PR",
        state: "open",
        head_branch: "feature/scoped",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "kdlbs",
        repo_name: "kandev",
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
      {
        number: 6102,
        title: "Out of scope PR",
        state: "open",
        head_branch: "feature/out-of-scope",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "other",
        repo_name: "repo",
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
    ]);

    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/integrations/github`);
    await expect(testPage.getByTestId("github-integration-heading")).toBeVisible();

    await testPage.getByTestId("github-scope-mode").click();
    await testPage.getByRole("option", { name: "Selected repositories" }).click();
    await testPage.getByTestId("github-scope-repos-input").fill("kdlbs/kandev");
    await testPage.getByTestId("settings-floating-save").getByRole("button").click();
    await expect(testPage.getByText("GitHub workspace settings saved").last()).toBeVisible({
      timeout: 10_000,
    });

    const settingsResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/github/workspace-settings?workspace_id=${seedData.workspaceId}`,
    );
    expect(settingsResponse.status).toBe(200);
    const settings = (await settingsResponse.json()) as {
      repo_scope_mode: string;
      repo_scope_repos: Array<{ owner: string; name: string }>;
    };
    expect(settings.repo_scope_mode).toBe("repos");
    expect(settings.repo_scope_repos).toEqual([{ owner: "kdlbs", name: "kandev" }]);

    await testPage.goto("/github");

    await expect(testPage.getByTestId("pr-row").filter({ hasText: "Scoped PR" })).toBeVisible({
      timeout: 15_000,
    });
    await expect(testPage.getByText("kdlbs/kandev#6101")).toBeVisible();
    await expect(testPage.getByText("Out of scope PR")).toHaveCount(0);
    await expect(testPage.getByText("other/repo#6102")).toHaveCount(0);
  });

  test("repository scope save only submits fields for the active mode", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    await testPage.goto("/settings/integrations/github");
    await expect(testPage.getByTestId("github-integration-heading")).toBeVisible();

    await testPage.getByTestId("github-scope-mode").click();
    await testPage.getByRole("option", { name: "Selected repositories" }).click();
    await testPage.getByTestId("github-scope-repos-input").fill("kdlbs/kandev");
    await testPage.getByTestId("github-scope-mode").click();
    await testPage.getByRole("option", { name: "Organizations" }).click();
    await testPage.getByTestId("github-scope-orgs-input").fill("kdlbs");
    await testPage.getByTestId("settings-floating-save").getByRole("button").click();
    await expect(testPage.getByText("GitHub workspace settings saved").last()).toBeVisible({
      timeout: 10_000,
    });

    const firstSettingsResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/github/workspace-settings?workspace_id=${seedData.workspaceId}`,
    );
    expect(firstSettingsResponse.status).toBe(200);
    const firstSettings = (await firstSettingsResponse.json()) as {
      repo_scope_mode: string;
      repo_scope_orgs: string[];
      repo_scope_repos: Array<{ owner: string; name: string }>;
    };
    expect(firstSettings.repo_scope_mode).toBe("orgs");
    expect(firstSettings.repo_scope_orgs).toEqual(["kdlbs"]);
    expect(firstSettings.repo_scope_repos).toEqual([]);

    await testPage.getByTestId("github-scope-mode").click();
    await testPage.getByRole("option", { name: "Selected repositories" }).click();
    await testPage.getByTestId("github-scope-repos-input").fill("not-a-repo");
    await testPage.getByTestId("github-scope-mode").click();
    await testPage.getByRole("option", { name: "Organizations" }).click();
    await testPage.getByTestId("settings-floating-save").getByRole("button").click();
    await expect(testPage.getByText("GitHub workspace settings saved").last()).toBeVisible({
      timeout: 10_000,
    });

    const secondSettingsResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/github/workspace-settings?workspace_id=${seedData.workspaceId}`,
    );
    expect(secondSettingsResponse.status).toBe(200);
    const secondSettings = (await secondSettingsResponse.json()) as {
      repo_scope_mode: string;
      repo_scope_orgs: string[];
      repo_scope_repos: Array<{ owner: string; name: string }>;
    };
    expect(secondSettings.repo_scope_mode).toBe("orgs");
    expect(secondSettings.repo_scope_orgs).toEqual(["kdlbs"]);
    expect(secondSettings.repo_scope_repos).toEqual([]);
  });
});
