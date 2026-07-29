import { test, expect } from "../../fixtures/test-base";

type ReviewWatchesResponse = {
  watches: Array<{ id: string; enabled: boolean }>;
};

test.describe("GitHub workspace settings", () => {
  test("keeps review watch pause and resume visible after save", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "legacy_shared",
      status: "active",
    });
    const watch = await apiClient.createReviewWatch(
      seedData.workspaceId,
      seedData.workflowId,
      seedData.startStepId,
      seedData.agentProfileId,
    );

    try {
      await testPage.goto(`/settings/workspace/${seedData.workspaceId}/integrations/github`);
      const row = testPage.getByRole("row").filter({ hasText: "All repositories" });
      await expect(row).toBeVisible();

      const toggle = row.getByRole("button").nth(0);
      await toggle.click();
      await testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes" })
        .click();

      await expect(row).toContainText("Paused");
      await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();
      await prCapture.screenshot("desktop-review-watch-paused", {
        caption: "Review watch remains paused after saving",
      });

      const pausedResponse = await apiClient.rawRequest(
        "GET",
        `/api/v1/github/watches/review?workspace_id=${encodeURIComponent(seedData.workspaceId)}`,
      );
      const pausedBody = (await pausedResponse.json()) as ReviewWatchesResponse;
      expect(pausedBody.watches.find((item) => item.id === watch.id)?.enabled).toBe(false);

      await toggle.click();
      await testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes" })
        .click();

      await expect(row).toContainText("Active");
      await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();

      const activeResponse = await apiClient.rawRequest(
        "GET",
        `/api/v1/github/watches/review?workspace_id=${encodeURIComponent(seedData.workspaceId)}`,
      );
      const activeBody = (await activeResponse.json()) as ReviewWatchesResponse;
      expect(activeBody.watches.find((item) => item.id === watch.id)?.enabled).toBe(true);
    } finally {
      await apiClient.deleteReviewWatch(watch.id, seedData.workspaceId);
    }
  });

  test("saves the explicit task Git credential policy", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/integrations/github`);
    await expect(testPage.getByRole("heading", { name: "Task Git credentials" })).toBeVisible();

    await testPage.getByRole("radio", { name: "Inherit executor Git credentials" }).click();
    await prCapture.screenshot("desktop-task-git-credentials", {
      caption: "Workspace task Git credential policy",
    });
    await testPage.getByTestId("settings-floating-save").getByRole("button").click();
    await expect(testPage.getByText("Task Git credential settings saved")).toBeVisible({
      timeout: 10_000,
    });

    const response = await apiClient.rawRequest(
      "GET",
      `/api/v1/github/workspace-settings?workspace_id=${seedData.workspaceId}`,
    );
    expect(response.status).toBe(200);
    expect(await response.json()).toMatchObject({ task_git_credentials_mode: "executor" });
  });

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

    const issueWatchesHeading = testPage.getByRole("heading", { name: "Issue Watches" });
    const repositoryScopeHeading = testPage.getByRole("heading", {
      name: "Repository Scope",
      exact: true,
    });
    const [issueWatchesBox, repositoryScopeBox] = await Promise.all([
      issueWatchesHeading.boundingBox(),
      repositoryScopeHeading.boundingBox(),
    ]);
    expect(issueWatchesBox).not.toBeNull();
    expect(repositoryScopeBox).not.toBeNull();
    expect(repositoryScopeBox!.y).toBeGreaterThan(issueWatchesBox!.y);

    await testPage.getByRole("button", { name: "Explain repository scope" }).hover();
    await expect(testPage.getByRole("tooltip")).toContainText(
      "Limits the GitHub pull requests and issues Kandev discovers for this workspace",
    );

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
