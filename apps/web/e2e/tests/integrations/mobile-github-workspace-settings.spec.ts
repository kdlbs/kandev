import { test, expect } from "../../fixtures/test-base";

test.describe("GitHub workspace settings on mobile", () => {
  test("configures task Git access in the connection drawer", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "legacy_shared",
      status: "active",
    });
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/integrations/github`);
    const automation = testPage.getByTestId("github-workspace-automation");
    await expect(automation.getByTestId("github-task-access-summary")).toContainText(
      "Managed workspace credentials",
    );
    await expect(testPage.getByRole("heading", { name: "My GitHub identity" })).toHaveCount(0);
    await expect(testPage.getByRole("heading", { name: "Task Git credentials" })).toHaveCount(0);
    const identityHelp = automation.getByRole("button", {
      name: "Explain workspace GitHub identity",
    });
    const taskAccessHelp = automation.getByRole("button", { name: "Explain task Git access" });
    const [identityHelpBox, taskAccessHelpBox] = await Promise.all([
      identityHelp.boundingBox(),
      taskAccessHelp.boundingBox(),
    ]);
    expect(identityHelpBox?.height).toBeGreaterThanOrEqual(44);
    expect(taskAccessHelpBox?.height).toBeGreaterThanOrEqual(44);
    await identityHelp.tap();
    await expect(testPage.getByRole("dialog", { name: "Workspace GitHub identity" })).toContainText(
      "repository sync, watches, background jobs, and managed agent GitHub commands",
    );
    await testPage.keyboard.press("Escape");
    await taskAccessHelp.tap();
    await expect(testPage.getByRole("dialog", { name: "Task Git access" })).toContainText(
      "newly launched tasks authenticate to GitHub",
    );
    await testPage.keyboard.press("Escape");

    await automation.getByRole("button", { name: "Change connection" }).tap();
    const drawer = testPage.getByTestId("github-connection-mobile");
    await expect(drawer.getByRole("heading", { name: "Task Git access" })).toBeVisible();
    await expect(drawer.locator(".overflow-y-auto")).toHaveCount(1);
    const executorOption = drawer.getByTestId("github-task-access-option-executor");
    const saveButton = drawer.getByRole("button", { name: "Save task access" });
    const [optionBox, saveButtonBox] = await Promise.all([
      executorOption.boundingBox(),
      saveButton.boundingBox(),
    ]);
    expect(optionBox).not.toBeNull();
    expect(saveButtonBox).not.toBeNull();
    expect(optionBox!.height).toBeGreaterThanOrEqual(44);
    expect(saveButtonBox!.height).toBeGreaterThanOrEqual(44);

    await executorOption.tap();
    await prCapture.screenshot("mobile-task-git-access-drawer", {
      caption: "Task Git access is configured in the mobile connection drawer",
    });
    await saveButton.tap();
    await expect(testPage.getByText("Task Git access saved")).toBeVisible({
      timeout: 10_000,
    });
    await expect(automation.getByTestId("github-task-access-summary")).toContainText(
      "Inherit executor Git credentials",
    );

    const response = await apiClient.rawRequest(
      "GET",
      `/api/v1/github/workspace-settings?workspace_id=${seedData.workspaceId}`,
    );
    expect(await response.json()).toMatchObject({ task_git_credentials_mode: "executor" });
  });

  test("explains repository scope below issue watches without requiring hover", async ({
    testPage,
    seedData,
  }) => {
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/integrations/github`);

    const issueWatchesHeading = testPage.getByRole("heading", { name: "Issue Watches" });
    const repositoryScopeHeading = testPage.getByRole("heading", {
      name: "Repository Scope",
      exact: true,
    });
    const scopeDescription = testPage.getByText(
      "Limits GitHub pull requests and issues shown or imported in this workspace.",
      { exact: true },
    );

    await expect(scopeDescription).toBeVisible();
    const scopeHelpButton = testPage.getByRole("button", { name: "Explain repository scope" });
    await expect(scopeHelpButton).toBeVisible();

    const [issueWatchesBox, repositoryScopeBox] = await Promise.all([
      issueWatchesHeading.boundingBox(),
      repositoryScopeHeading.boundingBox(),
    ]);
    expect(issueWatchesBox).not.toBeNull();
    expect(repositoryScopeBox).not.toBeNull();
    expect(repositoryScopeBox!.y).toBeGreaterThan(issueWatchesBox!.y);

    await scopeHelpButton.click();
    await expect(testPage.getByRole("dialog", { name: "Repository Scope" })).toContainText(
      "including My GitHub results and review and issue watches",
    );
  });
});
