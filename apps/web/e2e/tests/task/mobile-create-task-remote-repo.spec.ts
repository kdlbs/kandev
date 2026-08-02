import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

function expectedRemoteTitle(title: string): string {
  const characters = Array.from(title);
  return characters.length <= 60 ? title : `${characters.slice(0, 59).join("")}…`;
}

async function openRemotePicker(testPage: Page): Promise<void> {
  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await mobile.mobileFab.click();
  await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();
  await testPage.getByTestId("source-mode-remote").click();
  await testPage.getByTestId("remote-repo-chip-trigger").first().click();
}

async function expectPopoverFitsViewport(testPage: Page): Promise<void> {
  const viewport = testPage.viewportSize();
  const input = testPage.getByTestId("remote-repo-input");
  const [box, inputBox] = await Promise.all([
    testPage.getByTestId("remote-repo-popover-content").boundingBox(),
    input.boundingBox(),
  ]);
  expect(viewport).not.toBeNull();
  expect(box).not.toBeNull();
  expect(inputBox).not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(viewport!.width);
  expect(box!.y + box!.height).toBeLessThanOrEqual(viewport!.height);
  expect(inputBox!.y - box!.y).toBeLessThan(16);
  await expect(input).toHaveCSS("height", "44px");
}

async function expectNoDocumentHorizontalOverflow(testPage: Page): Promise<void> {
  await expect
    .poll(() =>
      testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    )
    .toBe(true);
}

async function expectLocatorFitsViewport(testPage: Page, testId: string): Promise<void> {
  const viewport = testPage.viewportSize();
  const box = await testPage.getByTestId(testId).boundingBox();
  expect(viewport).not.toBeNull();
  expect(box).not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(viewport!.width);
  expect(box!.y).toBeGreaterThanOrEqual(0);
  expect(box!.y + box!.height).toBeLessThanOrEqual(viewport!.height);
}

test.describe("Create task Remote repo picker on mobile", () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.mockGitHubReset();
  });

  test("stages a pasted URL until Enter without resolving it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const owner = "phone-entry-owner";
    const repo = "phone-entry-repo";
    const url = `https://github.com/${owner}/${repo}`;
    const branchPattern = new RegExp(`/api/v1/github/repos/${owner}/${repo}/branches\\?.+$`);
    let branchRequests = 0;
    await apiClient.mockGitHubAddBranches(owner, repo, [{ name: "main" }]);
    await testPage.route(branchPattern, async (route) => {
      expect(new URL(route.request().url()).searchParams.get("workspace_id")).toBe(
        seedData.workspaceId,
      );
      branchRequests += 1;
      await route.continue();
    });

    await openRemotePicker(testPage);
    const input = testPage.getByTestId("remote-repo-input");
    await input.fill(`${url}-draft`);
    await expect(input).toHaveValue(`${url}-draft`);
    await expect(testPage.getByText("Remote URL", { exact: true })).toBeVisible();
    await expect(testPage.getByText(/press Enter to submit/i)).toBeVisible();
    await expectPopoverFitsViewport(testPage);
    expect(branchRequests).toBe(0);

    await input.fill(url);
    await input.press("Enter");

    await expect(testPage.getByTestId("remote-repo-chip")).toHaveAttribute("data-remote-url", url);
    await expect(testPage.getByTestId("remote-branch-chip-trigger")).toContainText("main");
    await expect.poll(() => branchRequests).toBe(1);
    await expectNoDocumentHorizontalOverflow(testPage);
  });

  test("keeps a failed URL row touch-usable and retries its branch resolution", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const owner = "phone-retry-owner";
    const repo = "phone-retry-repo";
    const url = `https://github.com/${owner}/${repo}`;
    const branchPattern = new RegExp(`/api/v1/github/repos/${owner}/${repo}/branches\\?.+$`);
    let branchRequests = 0;
    await apiClient.mockGitHubAddBranches(owner, repo, [{ name: "main" }]);
    await testPage.route(branchPattern, async (route) => {
      expect(new URL(route.request().url()).searchParams.get("workspace_id")).toBe(
        seedData.workspaceId,
      );
      branchRequests += 1;
      if (branchRequests === 1) {
        await route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ error: "Temporary provider outage" }),
        });
        return;
      }
      await route.continue();
    });

    await openRemotePicker(testPage);
    const input = testPage.getByTestId("remote-repo-input");
    await input.fill(url);
    await input.press("Enter");

    const row = testPage.getByTestId("remote-repo-chip");
    const retry = testPage.getByRole("button", { name: "Retry remote repository resolution" });
    await expect(row).toHaveAttribute("data-remote-url", url);
    await expect(testPage.getByRole("alert")).toContainText(/Temporary provider outage/i);
    await expect(retry).toBeVisible();
    await expectLocatorFitsViewport(testPage, "remote-repo-chip-wrapper");
    const retryBox = await retry.boundingBox();
    expect(retryBox).not.toBeNull();
    expect(retryBox!.height).toBeGreaterThanOrEqual(44);
    await expectNoDocumentHorizontalOverflow(testPage);

    await retry.tap();

    await expect.poll(() => branchRequests).toBe(2);
    await expect(testPage.getByRole("alert")).not.toBeVisible();
    await expect(testPage.getByTestId("remote-branch-chip-trigger")).toContainText("main");
    await expectNoDocumentHorizontalOverflow(testPage);
  });

  test("pastes a GitHub issue URL without clipping the picker", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    await apiClient.mockGitHubAddBranches("issue-owner", "issue-repo", [{ name: "main" }]);
    await apiClient.mockGitHubAddIssues([
      {
        number: 1456,
        title:
          "Fix remote repo picker clipping while preserving a concise task title for the mobile flow",
        body: "The picker overlaps the dialog footer.",
        state: "open",
        author_login: "mock-user",
        repo_owner: "issue-owner",
        repo_name: "issue-repo",
        html_url: "https://github.com/issue-owner/issue-repo/issues/1456",
      },
    ]);

    await openRemotePicker(testPage);
    await expectPopoverFitsViewport(testPage);
    const pasteInput = testPage.getByTestId("remote-repo-input").last();
    await pasteInput.fill("https://github.com/issue-owner/issue-repo/issues/1456");
    await pasteInput.press("Enter");

    const titleInput = testPage.getByTestId("task-title-input");
    await expect(titleInput).toHaveValue(
      expectedRemoteTitle(
        "Issue #1456: Fix remote repo picker clipping while preserving a concise task title for the mobile flow",
      ),
      { timeout: 10_000 },
    );
    await expect(titleInput).not.toHaveAttribute("maxlength");

    await prCapture.screenshot("mobile-task-title-limit", {
      caption: "Mobile remote issue task title truncated to the 60-character limit",
    });

    await titleInput.fill("x".repeat(80));
    await expect(titleInput).toHaveValue("x".repeat(60));

    const emojiTitle = "😀".repeat(60);
    await titleInput.fill(emojiTitle);
    await expect(titleInput).toHaveValue(emojiTitle);
  });

  test("selects a GitLab repository from the unified provider picker", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "legacy_shared",
      status: "active",
    });
    await apiClient.configureGitLab(seedData.workspaceId);
    await apiClient.mockAzureDevOpsSeed({
      authenticated: true,
      projects: [{ id: "project-1", name: "Platform", url: "https://dev.azure.com/acme/Platform" }],
      repositories: [
        {
          id: "azure-repo-1",
          name: "api",
          projectId: "project-1",
          projectName: "Platform",
          defaultBranch: "refs/heads/main",
          webUrl: "https://dev.azure.com/acme/Platform/_git/api",
        },
      ],
    });
    await apiClient.setAzureDevOpsConfig(seedData.workspaceId, {
      organizationUrl: "https://dev.azure.com/acme",
      pat: "azure-test-pat",
    });

    await openRemotePicker(testPage);
    const providerTabs = testPage.getByTestId("remote-repo-provider-tabs");
    await expect(providerTabs).toBeVisible();
    await expect(providerTabs.getByRole("tab", { name: "GitHub" })).toBeVisible();
    const gitLabTab = providerTabs.getByRole("tab", { name: "GitLab" });
    await expect(gitLabTab).toBeVisible();
    const azureTab = providerTabs.getByRole("tab", { name: "Azure DevOps" });
    await expect(azureTab).toBeVisible();
    await testPage.getByTestId("remote-repo-popover-content").evaluate(async (element) => {
      await Promise.all(
        element.getAnimations().map((animation) => animation.finished.catch(() => undefined)),
      );
    });
    const tabBoxes = await Promise.all([gitLabTab.boundingBox(), azureTab.boundingBox()]);
    for (const tabBox of tabBoxes) {
      expect(tabBox).not.toBeNull();
      expect(tabBox!.height).toBeGreaterThanOrEqual(44);
    }
    const tabOverflow = await providerTabs.evaluate((element) => ({
      overflowY: getComputedStyle(element).overflowY,
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
    }));
    expect(tabOverflow.overflowY).toBe("hidden");
    expect(tabOverflow.scrollHeight).toBeLessThanOrEqual(tabOverflow.clientHeight);
    await gitLabTab.click();
    const option = testPage.getByTestId("remote-repo-option").filter({ hasText: "kandev/sample" });
    await expect(option).toBeVisible({ timeout: 10_000 });
    await option.click();
    await expect(testPage.getByTestId("remote-repo-chip-trigger").first()).toContainText(
      "kandev/sample",
    );
    const hasHorizontalOverflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
    );
    expect(hasHorizontalOverflow).toBe(false);
  });

  test("marks an already selected provider repository without disabling its touch selection", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    // mockGitHubReset clears workspace-owned credentials. Reconnect this
    // workspace before seeding the provider response so the picker exercises
    // the same workspace-scoped auth boundary as production.
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "pat",
      status: "active",
      login: "mock-user",
    });
    await apiClient.mockGitHubAddRepos("mock-user", [
      { full_name: "mock-user/duplicate", owner: "mock-user", name: "duplicate", private: false },
    ]);

    await openRemotePicker(testPage);
    const firstOption = testPage
      .getByTestId("remote-repo-option")
      .filter({ hasText: "mock-user/duplicate" });
    await expect(firstOption).toBeVisible({ timeout: 10_000 });
    await firstOption.tap();

    await testPage.getByTestId("remote-add-row").tap();
    await testPage.getByTestId("remote-repo-chip-trigger").nth(1).tap();
    const duplicateOption = testPage
      .getByTestId("remote-repo-option")
      .filter({ hasText: "mock-user/duplicate" });
    await expect(duplicateOption.getByTestId("already-added-repository-marker")).toBeVisible();

    const [optionBox, viewport] = await Promise.all([
      duplicateOption.boundingBox(),
      testPage.viewportSize(),
    ]);
    expect(optionBox).not.toBeNull();
    expect(viewport).not.toBeNull();
    expect(optionBox!.x).toBeGreaterThanOrEqual(0);
    expect(optionBox!.x + optionBox!.width).toBeLessThanOrEqual(viewport!.width);
    expect(optionBox!.y + optionBox!.height).toBeLessThanOrEqual(viewport!.height);

    await duplicateOption.tap();
    await expect(testPage.getByTestId("remote-repo-chip-trigger").nth(1)).toContainText(
      "mock-user/duplicate",
    );
  });
});
