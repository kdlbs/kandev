import { test, expect } from "../../fixtures/test-base";
import { stubGitHubRateLimits } from "./github-rate-limit-fixture";

test.describe("GitHub workspace settings on mobile", () => {
  test("configures task Git access in the connection drawer", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    const workspace = await apiClient.createWorkspace("Mobile GitHub task defaults workspace");
    const workspaceId = workspace.id;
    await apiClient.mockGitHubSetWorkspaceConnection(workspaceId, {
      source: "legacy_shared",
      status: "active",
    });
    await apiClient.mockGitHubSetCLIAccounts([
      { host: "github.com", login: "mobile-cli", active: true, state: "active" },
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
    const taskAccessHelp = automation.getByRole("button", { name: "Explain task Git access" });
    await expect(identityHelp).toHaveAttribute("aria-haspopup", "dialog");
    await expect(identityHelp).toHaveAttribute("aria-expanded", "false");
    await expect(taskAccessHelp).toHaveAttribute("aria-haspopup", "dialog");
    await expect(taskAccessHelp).toHaveAttribute("aria-expanded", "false");
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
    const rateLimitHelp = automation.getByRole("button", { name: "Show GitHub API limits" });
    await expect(rateLimitHelp).toBeVisible();
    const rateLimitHelpBox = await rateLimitHelp.boundingBox();
    expect(rateLimitHelpBox?.height).toBeGreaterThanOrEqual(44);
    await rateLimitHelp.tap();
    const rateLimitDrawer = testPage.getByRole("dialog", { name: "GitHub API limits" });
    await expect(rateLimitDrawer).toContainText(
      "API rate limit: 4,321 of 5,000 requests remaining",
    );
    await expect(rateLimitDrawer).toContainText(
      "GraphQL query limit: 4,900 of 5,000 points remaining",
    );
    await testPage.keyboard.press("Escape");

    await automation.getByRole("button", { name: "Change connection" }).tap();
    const drawer = testPage.getByTestId("github-connection-mobile");
    const drawerBox = await drawer.boundingBox();
    const scrollBody = drawer.getByTestId("github-connection-scroll");
    const scrollFade = drawer.getByTestId("github-connection-scroll-fade");
    const footer = drawer.getByTestId("github-connection-footer");
    const fixedSaveButton = footer.getByRole("button", { name: "Save changes" });
    await expect(fixedSaveButton).toBeVisible();
    const [scrollBox, fadeBox, footerBox, initialSaveBox] = await Promise.all([
      scrollBody.boundingBox(),
      scrollFade.boundingBox(),
      footer.boundingBox(),
      fixedSaveButton.boundingBox(),
    ]);
    expect(drawerBox).not.toBeNull();
    expect(scrollBox).not.toBeNull();
    expect(fadeBox).not.toBeNull();
    expect(footerBox).not.toBeNull();
    expect(initialSaveBox).not.toBeNull();
    expect(
      Math.abs(fadeBox!.y + fadeBox!.height - (scrollBox!.y + scrollBox!.height)),
    ).toBeLessThanOrEqual(2);
    expect(footerBox!.y + footerBox!.height).toBeLessThanOrEqual(drawerBox!.y + drawerBox!.height);
    const methodGroup = drawer.getByRole("radiogroup", { name: "Connection method" });
    await expect(methodGroup.getByRole("radio").first()).toHaveAttribute("id", "github-method-cli");
    await drawer.getByRole("radio", { name: /^GitHub CLI account/ }).tap();
    await expect(drawer.getByRole("combobox", { name: "GitHub CLI account" })).toContainText(
      "mobile-cli",
    );
    await expect(drawer.getByRole("button", { name: "Use account" })).toHaveCount(0);
    await expect(drawer.getByRole("heading", { name: "Task Git access" })).toBeVisible();
    await expect(drawer.locator(".overflow-y-auto")).toHaveCount(1);
    const taskHelp = drawer.getByRole("button", {
      name: "Explain how managed task credentials work",
    });
    const taskHelpBox = await taskHelp.boundingBox();
    expect(taskHelpBox?.height).toBeGreaterThanOrEqual(44);
    await taskHelp.tap();
    await expect(
      testPage.getByRole("dialog", { name: "How managed task credentials work" }),
    ).toContainText("agentctl as Git's credential helper");
    await testPage.keyboard.press("Escape");
    await expect(drawer).toBeVisible();
    const executorOption = drawer.getByTestId("github-task-access-option-executor");
    await expect(drawer.getByRole("button", { name: "Save task access" })).toHaveCount(0);
    const saveButton = drawer.getByRole("button", { name: "Save changes" });
    const [optionBox, saveButtonBox] = await Promise.all([
      executorOption.boundingBox(),
      saveButton.boundingBox(),
    ]);
    expect(optionBox).not.toBeNull();
    expect(saveButtonBox).not.toBeNull();
    expect(optionBox!.height).toBeGreaterThanOrEqual(44);
    expect(saveButtonBox!.height).toBeGreaterThanOrEqual(44);

    const managedOption = drawer.getByTestId("github-task-access-option-managed");
    const [managedBox, compactExecutorBox] = await Promise.all([
      managedOption.boundingBox(),
      executorOption.boundingBox(),
    ]);
    expect(managedBox).not.toBeNull();
    expect(compactExecutorBox).not.toBeNull();
    expect(compactExecutorBox!.y - (managedBox!.y + managedBox!.height)).toBeLessThanOrEqual(9);
    const beforeScrollSaveBox = await fixedSaveButton.boundingBox();
    expect(beforeScrollSaveBox).not.toBeNull();
    await scrollBody.evaluate((element) => element.scrollTo(0, element.scrollHeight));
    const scrolledSaveBox = await fixedSaveButton.boundingBox();
    expect(scrolledSaveBox).not.toBeNull();
    expect(Math.abs(scrolledSaveBox!.y - beforeScrollSaveBox!.y)).toBeLessThanOrEqual(2);

    await executorOption.tap();
    await prCapture.screenshot("mobile-task-git-access-drawer", {
      caption: "Task Git access is configured in the mobile connection drawer",
    });
    await saveButton.tap();
    await expect(testPage.getByText("GitHub access settings saved")).toBeVisible({
      timeout: 10_000,
    });
    await expect(automation.getByTestId("github-task-access-summary")).toContainText(
      "Inherit executor Git credentials",
    );

    const response = await apiClient.rawRequest(
      "GET",
      `/api/v1/github/workspace-settings?workspace_id=${workspaceId}`,
    );
    expect(await response.json()).toMatchObject({ task_git_credentials_mode: "executor" });
    const statusResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/github/status?workspace_id=${workspaceId}`,
    );
    expect(await statusResponse.json()).toMatchObject({
      automation: { source: "gh_cli", login: "mobile-cli" },
    });
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
