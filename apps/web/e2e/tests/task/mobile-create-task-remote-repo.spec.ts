import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

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

test.describe("Create task Remote repo picker on mobile", () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.mockGitHubReset();
  });

  test("pastes a GitHub issue URL without clipping the picker", async ({ testPage, apiClient }) => {
    await apiClient.mockGitHubAddBranches("issue-owner", "issue-repo", [{ name: "main" }]);
    await apiClient.mockGitHubAddIssues([
      {
        number: 1456,
        title: "Fix remote repo picker clipping",
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

    await expect(testPage.getByTestId("task-title-input")).toHaveValue(
      "Issue #1456: Fix remote repo picker clipping",
      { timeout: 10_000 },
    );
  });

  test("selects an Azure DevOps repository from the unified picker", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
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
    const azureTab = providerTabs.getByRole("tab", { name: "Azure DevOps" });
    await expect(azureTab).toBeVisible();
    await testPage.getByTestId("remote-repo-popover-content").evaluate(async (element) => {
      await Promise.all(
        element.getAnimations().map((animation) => animation.finished.catch(() => undefined)),
      );
    });
    const azureTabBox = await azureTab.boundingBox();
    expect(azureTabBox).not.toBeNull();
    expect(azureTabBox!.height).toBeGreaterThanOrEqual(44);
    const tabOverflow = await providerTabs.evaluate((element) => ({
      overflowY: getComputedStyle(element).overflowY,
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
    }));
    expect(tabOverflow.overflowY).toBe("hidden");
    expect(tabOverflow.scrollHeight).toBeLessThanOrEqual(tabOverflow.clientHeight);
    await azureTab.click();
    const option = testPage.getByTestId("remote-repo-option").filter({ hasText: "Platform/api" });
    await expect(option).toBeVisible({ timeout: 10_000 });
    await option.click();
    await expect(testPage.getByTestId("remote-repo-chip-trigger").first()).toContainText(
      "Platform/api",
    );
    const hasHorizontalOverflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
    );
    expect(hasHorizontalOverflow).toBe(false);
  });
});
