import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { SessionPage } from "../../pages/session-page";
import { waitForHttp } from "../../helpers/causal-waits";
import { switchToTerminalPanel, waitForShellReady } from "../terminal/mobile-terminal-helpers";
import {
  cleanupPRLinkForkLaunchFixture,
  createPRLinkForkLaunchFixture,
  expectForkPRLaunchState,
  waitForForkPRInfo,
} from "./pr-link-fork-launch-helpers";
import { openTaskRepositoryPicker } from "../../helpers/task-repository-picker";

function expectedRemoteTitle(title: string): string {
  const characters = Array.from(title);
  return characters.length <= 60 ? title : `${characters.slice(0, 59).join("")}…`;
}

async function openRemotePicker(testPage: Page, provider?: string): Promise<void> {
  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await mobile.mobileFab.click();
  await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();
  const manager = testPage.getByTestId("mobile-repository-manager");
  if ((await manager.count()) > 0 && (await manager.isVisible().catch(() => false))) {
    await manager.tap();
    const removeButtons = testPage.getByTestId("remove-repo-chip");
    while ((await removeButtons.count()) > 0) {
      await removeButtons.first().tap();
    }
    const remoteRemoveButtons = testPage.getByTestId("remote-chip-remove");
    while ((await remoteRemoveButtons.count()) > 0) {
      await remoteRemoveButtons.first().tap();
    }
  }
  await openTaskRepositoryPicker(testPage, { mobile: true, provider });
}

async function openRepositoryManagement(testPage: Page): Promise<void> {
  const sheet = testPage.getByTestId("mobile-repository-sheet-content");
  const management = testPage.getByTestId("mobile-repository-management");
  if (!(await management.isVisible().catch(() => false))) {
    await expect
      .poll(() => sheet.isVisible().catch(() => false), {
        timeout: 10_000,
        message: "mobile repository picker did not finish closing",
      })
      .toBe(false);
    await testPage.getByTestId("mobile-repository-manager").tap();
  }
  await expect(management).toBeVisible();
}

async function expectPopoverFitsViewport(testPage: Page): Promise<void> {
  const viewport = testPage.viewportSize();
  const input = testPage.getByTestId("task-repository-picker-input");
  const [box, inputBox] = await Promise.all([
    testPage.getByTestId("task-repository-picker").boundingBox(),
    input.boundingBox(),
  ]);
  expect(viewport).not.toBeNull();
  expect(box).not.toBeNull();
  expect(inputBox).not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(viewport!.width);
  expect(box!.y + box!.height).toBeLessThanOrEqual(viewport!.height);
  expect(inputBox!.y).toBeGreaterThanOrEqual(box!.y);
  expect(inputBox!.y + inputBox!.height).toBeLessThanOrEqual(box!.y + box!.height);
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
    const input = testPage.getByTestId("task-repository-picker-input");
    await input.fill(`${url}-draft`);
    await expect(input).toHaveValue(`${url}-draft`);
    await expect(testPage.getByTestId("task-repository-paste-url")).toBeVisible();
    await expectPopoverFitsViewport(testPage);
    expect(branchRequests).toBe(0);

    await input.fill(url);
    await input.press("Enter");

    await openRepositoryManagement(testPage);
    await expect(testPage.getByTestId("remote-repo-chip")).toHaveAttribute("data-remote-url", url);
    await expect(testPage.getByTestId("remote-branch-chip-trigger")).toContainText("main");
    await expect.poll(() => branchRequests).toBe(1);
    await expectNoDocumentHorizontalOverflow(testPage);
  });

  test("starts a target-attached fork PR from its URL", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    const fixture = await createPRLinkForkLaunchFixture(
      apiClient,
      seedData.workspaceId,
      backend.tmpDir,
    );
    let taskId: string | undefined;

    try {
      const { executors } = await apiClient.listExecutors();
      const worktreeExec = executors.find((executor) => executor.type === "worktree");
      if (!worktreeExec?.profiles?.[0]) {
        test.skip(true, "No worktree executor profile available");
        return;
      }

      const taskTitle = `Phone fork PR ${fixture.repositoryName}`;
      const mobile = new MobileKanbanPage(testPage);
      await openRemotePicker(testPage);
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      const prInfoResponse = waitForForkPRInfo(testPage, fixture);
      const urlInput = testPage.getByTestId("task-repository-picker-input");
      await expect(urlInput).toBeVisible();
      await urlInput.fill(fixture.prURL);
      await urlInput.press("Enter");
      expect((await prInfoResponse).status()).toBe(200);
      await openRepositoryManagement(testPage);
      await expect(testPage.getByTestId("remote-branch-chip-trigger").first()).toContainText(
        fixture.headBranch,
      );
      await testPage.getByTestId("mobile-repository-done").tap();
      await testPage.getByTestId("task-title-input").fill(taskTitle);
      await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");

      const startButton = testPage.getByTestId("submit-start-agent");
      await expect(startButton).toBeEnabled();
      await testPage.getByTestId("executor-profile-selector").tap();
      await testPage.getByRole("option", { name: /Worktree/i }).tap();
      const createdTaskResponse = waitForHttp(testPage, "POST", /\/api\/v1\/tasks$/);
      await startButton.tap();
      const response = await createdTaskResponse;
      const responseBody = await response.text();
      expect(response.status(), responseBody).toBe(200);
      const created = JSON.parse(responseBody) as { id: string };
      taskId = created.id;
      const requestBody = response.request().postDataJSON() as {
        workspace_sources?: Array<Record<string, unknown>>;
        repositories?: Array<Record<string, unknown>>;
      };
      const repositorySource =
        requestBody.workspace_sources?.find((source) => source.kind === "repository") ??
        requestBody.repositories?.[0];
      expect(repositorySource).toBeDefined();
      expect(repositorySource).toMatchObject({
        base_branch: "main",
        checkout_branch: fixture.headBranch,
      });
      expect(repositorySource).not.toHaveProperty("remote_contribution");
      expect(repositorySource).not.toHaveProperty("comparison_target");

      await expect(dialog).not.toBeVisible();
      await expect(mobile.taskCard(taskId)).toBeVisible({ timeout: 15_000 });
      await mobile.taskCard(taskId).tap();
      await expect(testPage).toHaveURL(new RegExp(`/t/${taskId}$`));
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible();
      await expect(session.idleInput()).toBeVisible();

      await switchToTerminalPanel(testPage);
      await waitForShellReady(testPage);
      await expectForkPRLaunchState(testPage, session, apiClient, fixture, taskId);
    } finally {
      await cleanupPRLinkForkLaunchFixture(apiClient, fixture, taskId);
    }
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
    const input = testPage.getByTestId("task-repository-picker-input");
    await input.fill(url);
    await input.press("Enter");

    await openRepositoryManagement(testPage);
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
    const pasteInput = testPage.getByTestId("task-repository-picker-input").last();
    await pasteInput.fill("https://github.com/issue-owner/issue-repo/issues/1456");
    await pasteInput.press("Enter");
    await openRepositoryManagement(testPage);
    await waitForFiniteAnimations(testPage.getByTestId("mobile-repository-sheet-content"));
    await testPage.getByTestId("mobile-repository-done").dispatchEvent("click");

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
    const providerTabs = testPage.getByTestId("task-repository-source-tabs");
    await expect(providerTabs).toBeVisible();
    await expect(providerTabs.getByRole("tab", { name: "GitHub" })).toBeVisible();
    const gitLabTab = providerTabs.getByRole("tab", { name: "GitLab" });
    await expect(gitLabTab).toBeVisible();
    const azureTab = providerTabs.getByRole("tab", { name: "Azure DevOps" });
    await expect(azureTab).toBeVisible();
    const tabBoxes = await Promise.all([gitLabTab.boundingBox(), azureTab.boundingBox()]);
    for (const tabBox of tabBoxes) {
      expect(tabBox).not.toBeNull();
      expect(tabBox!.height).toBeGreaterThanOrEqual(44);
    }
    const tabOverflow = await providerTabs.evaluate(
      (element) => getComputedStyle(element).overflowX,
    );
    expect(["auto", "scroll"]).toContain(tabOverflow);
    await gitLabTab.click();
    const option = testPage
      .getByTestId("task-repository-remote-option")
      .filter({ hasText: "kandev/sample" });
    await expect(option).toBeVisible({ timeout: 10_000 });
    await option.click();
    await openRepositoryManagement(testPage);
    await expect(testPage.getByTestId("remote-repo-chip-trigger").first()).toContainText(
      "kandev/sample",
    );
    const hasHorizontalOverflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
    );
    expect(hasHorizontalOverflow).toBe(false);
  });

  test("allows selecting the same provider repository in a second row", async ({
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

    await openRemotePicker(testPage, "github");
    const firstOption = testPage
      .getByTestId("task-repository-remote-option")
      .filter({ hasText: "mock-user/duplicate" });
    await expect(firstOption).toBeVisible({ timeout: 10_000 });
    await firstOption.tap();

    await openTaskRepositoryPicker(testPage, { mobile: true, provider: "github" });
    await waitForFiniteAnimations(testPage.locator('[data-slot="drawer-content"]:visible').last());
    const duplicateOption = testPage
      .getByTestId("task-repository-remote-option")
      .filter({ hasText: "mock-user/duplicate" });

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
    await openRepositoryManagement(testPage);
    await expect(testPage.getByTestId("remote-repo-chip-trigger").nth(1)).toContainText(
      "mock-user/duplicate",
    );
  });

  test("keeps an unconfigured provider out of the touch picker", async ({
    apiClient,
    seedData,
    testPage,
    prCapture,
  }) => {
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "legacy_shared",
      status: "active",
    });
    await apiClient.mockGitHubAddRepos("mock-user", [
      {
        full_name: "mock-user/phone-alpha",
        owner: "mock-user",
        name: "phone-alpha",
        private: false,
      },
    ]);
    let gitLabProjectRequests = 0;
    await testPage.route("**/api/v1/gitlab/projects?*", async (route) => {
      gitLabProjectRequests += 1;
      await route.continue();
    });

    await openRemotePicker(testPage, "github");

    await expect(testPage.getByTestId("task-repository-source-github")).toHaveCount(1);
    await expect(testPage.getByTestId("task-repository-source-gitlab")).toHaveCount(0);
    await expect(testPage.getByText(/Could not load repositories/i)).toHaveCount(0);
    await expect(testPage.getByTestId("task-repository-picker-input")).toBeVisible();
    expect(gitLabProjectRequests).toBe(0);
    await prCapture.screenshot("remote-repository-picker-mobile", {
      caption: "Mobile remote picker with an unconfigured provider hidden",
    });

    const input = testPage.getByTestId("task-repository-picker-input");
    await input.fill("https://github.com/mock-user/phone-alpha");
    await input.press("Enter");
    await openRepositoryManagement(testPage);
    await expect(testPage.getByTestId("remote-repo-chip").first()).toHaveAttribute(
      "data-remote-url",
      "https://github.com/mock-user/phone-alpha",
    );
    await expectNoDocumentHorizontalOverflow(testPage);
  });
});
