import { test, expect } from "../../fixtures/test-base";
import { stubGitHubRateLimits } from "./github-rate-limit-fixture";

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

  test("configures task Git access from the workspace connection dialog", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    const workspace = await apiClient.createWorkspace("GitHub task defaults workspace");
    const workspaceId = workspace.id;
    await apiClient.mockGitHubSetWorkspaceConnection(workspaceId, {
      source: "legacy_shared",
      status: "active",
    });
    await apiClient.mockGitHubSetCLIAccounts([
      { host: "github.com", login: "workspace-cli", active: true, state: "active" },
    ]);
    await stubGitHubRateLimits(testPage, workspaceId);
    await testPage.goto(`/settings/workspace/${workspaceId}/integrations/github`);
    const automation = testPage.getByTestId("github-workspace-automation");
    await expect(automation.getByTestId("github-task-access-summary")).toContainText(
      "Inherit executor Git credentials",
    );
    await expect(testPage.getByRole("heading", { name: "My GitHub identity" })).toHaveCount(0);
    await expect(testPage.getByRole("heading", { name: "Task Git credentials" })).toHaveCount(0);
    const identityHelp = automation.getByRole("button", {
      name: "Explain workspace GitHub identity",
    });
    await expect(identityHelp).not.toHaveAttribute("aria-haspopup");
    await expect(identityHelp).not.toHaveAttribute("aria-expanded");
    await identityHelp.hover();
    await expect(
      testPage.getByRole("tooltip", {
        name: /repository sync, watches, background jobs, and managed agent GitHub commands/,
      }),
    ).toBeVisible();
    const taskAccessHelp = automation.getByRole("button", { name: "Explain task Git access" });
    await expect(taskAccessHelp).not.toHaveAttribute("aria-haspopup");
    await expect(taskAccessHelp).not.toHaveAttribute("aria-expanded");
    await taskAccessHelp.hover();
    await expect(
      testPage.getByRole("tooltip", {
        name: /newly launched tasks authenticate to GitHub/,
      }),
    ).toBeVisible();
    const rateLimitHelp = automation.getByRole("button", { name: "Show GitHub API limits" });
    await expect(rateLimitHelp).toBeVisible();
    await rateLimitHelp.hover();
    const rateLimitTooltip = testPage.getByRole("tooltip", { name: /API rate limit/ });
    await expect(rateLimitTooltip).toContainText(
      "API rate limit: 4,321 of 5,000 requests remaining",
    );
    await expect(rateLimitTooltip).toContainText(
      "GraphQL query limit: 4,900 of 5,000 points remaining",
    );

    await automation.getByRole("button", { name: "Change connection" }).click();
    const dialog = testPage.getByRole("dialog", { name: "Change GitHub connection" });
    const dialogBox = await dialog.boundingBox();
    expect(dialogBox).not.toBeNull();
    expect(dialogBox!.width).toBeGreaterThanOrEqual(800);
    const methodGroup = dialog.getByRole("radiogroup", { name: "Connection method" });
    await expect(methodGroup.getByRole("radio").first()).toHaveAttribute("id", "github-method-cli");
    const scrollBody = dialog.getByTestId("github-connection-scroll");
    const scrollFade = dialog.getByTestId("github-connection-scroll-fade");
    const footer = dialog.getByTestId("github-connection-footer");
    const fixedSaveButton = footer.getByRole("button", { name: "Save changes" });
    await expect(fixedSaveButton).toBeVisible();
    const [scrollBox, fadeBox, footerBox, initialSaveBox] = await Promise.all([
      scrollBody.boundingBox(),
      scrollFade.boundingBox(),
      footer.boundingBox(),
      fixedSaveButton.boundingBox(),
    ]);
    expect(scrollBox).not.toBeNull();
    expect(fadeBox).not.toBeNull();
    expect(footerBox).not.toBeNull();
    expect(initialSaveBox).not.toBeNull();
    expect(fadeBox!.y + fadeBox!.height).toBeCloseTo(scrollBox!.y + scrollBox!.height, 1);
    expect(footerBox!.y + footerBox!.height).toBeLessThanOrEqual(dialogBox!.y + dialogBox!.height);
    await dialog.getByRole("radio", { name: /^GitHub CLI account/ }).click();
    await expect(dialog.getByRole("combobox", { name: "GitHub CLI account" })).toContainText(
      "workspace-cli",
    );
    await expect(dialog.getByRole("button", { name: "Use account" })).toHaveCount(0);
    await expect(dialog.getByRole("heading", { name: "Task Git access" })).toBeVisible();
    const taskHelp = dialog.getByRole("button", {
      name: "Explain how managed task credentials work",
    });
    await taskHelp.hover();
    await expect(
      testPage.getByRole("tooltip", {
        name: /agentctl as Git's credential helper/,
      }),
    ).toBeVisible();
    const cliDescription = dialog.getByText(
      "Choose the exact account. Kandev does not silently follow the CLI's active-account change.",
      { exact: true },
    );
    const managedDescription = dialog.getByText(
      /Kandev brokers the workspace PAT, named GitHub CLI account, or App identity/,
    );
    const [cliFontSize, managedFontSize] = await Promise.all([
      cliDescription.evaluate((element) => getComputedStyle(element).fontSize),
      managedDescription.evaluate((element) => getComputedStyle(element).fontSize),
    ]);
    expect(managedFontSize).toBe(cliFontSize);
    const managedOption = dialog.getByTestId("github-task-access-option-managed");
    const executorOption = dialog.getByTestId("github-task-access-option-executor");
    const [managedBox, executorBox] = await Promise.all([
      managedOption.boundingBox(),
      executorOption.boundingBox(),
    ]);
    expect(managedBox).not.toBeNull();
    expect(executorBox).not.toBeNull();
    expect(executorBox!.y - (managedBox!.y + managedBox!.height)).toBeLessThanOrEqual(9);
    await dialog.getByRole("radio", { name: "Inherit executor Git credentials" }).click();
    await testPage.waitForTimeout(300);
    await prCapture.screenshot("desktop-task-git-access-dialog", {
      caption: "Task Git access is configured alongside the workspace connection",
    });
    await expect(dialog.getByRole("button", { name: "Save task access" })).toHaveCount(0);
    const saveButton = dialog.getByRole("button", { name: "Save changes" });
    await expect(saveButton).toHaveCount(1);
    const beforeScrollSaveBox = await fixedSaveButton.boundingBox();
    expect(beforeScrollSaveBox).not.toBeNull();
    await scrollBody.evaluate((element) => element.scrollTo(0, element.scrollHeight));
    const scrolledSaveBox = await fixedSaveButton.boundingBox();
    expect(scrolledSaveBox).not.toBeNull();
    expect(scrolledSaveBox!.y).toBeCloseTo(beforeScrollSaveBox!.y, 1);
    await saveButton.click();
    await expect(testPage.getByText("GitHub access settings saved")).toBeVisible({
      timeout: 10_000,
    });
    await expect(dialog).not.toBeVisible();
    await expect(automation.getByTestId("github-task-access-summary")).toContainText(
      "Inherit executor Git credentials",
    );

    const response = await apiClient.rawRequest(
      "GET",
      `/api/v1/github/workspace-settings?workspace_id=${workspaceId}`,
    );
    expect(response.status).toBe(200);
    expect(await response.json()).toMatchObject({ task_git_credentials_mode: "executor" });
    const statusResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/github/status?workspace_id=${workspaceId}`,
    );
    expect(await statusResponse.json()).toMatchObject({
      automation: { source: "gh_cli", login: "workspace-cli" },
    });

    await automation.getByRole("button", { name: "Change connection" }).click();
    const reopenedDialog = testPage.getByRole("dialog", { name: "Change GitHub connection" });
    await expect(
      reopenedDialog.getByRole("radio", { name: "Inherit executor Git credentials" }),
    ).toBeChecked();
    await expect(reopenedDialog.locator("#github-method-cli")).toHaveAttribute(
      "data-state",
      "checked",
    );
    await expect(
      reopenedDialog.getByRole("combobox", { name: "GitHub CLI account" }),
    ).toContainText("workspace-cli");
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
