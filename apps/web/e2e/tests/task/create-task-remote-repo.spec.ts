import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { Page } from "@playwright/test";
import { KanbanPage } from "../../pages/kanban-page";

// E2E coverage for the multi-row chip-based Remote tab in the create-task
// dialog. Tests cover: picker selection, paste URL, mixed picker+paste
// rows, add/remove rows, the "GitHub not configured" banner, and mode-switch
// preservation. Modeled on create-task-github-url.spec.ts but exercises the
// new chip-popover UI (testids: source-mode-remote, remote-repo-chip,
// remote-repo-chip-trigger, remote-repo-input, remote-add-row,
// remote-chip-remove, remote-repo-option).

type TaskRepoAPI = {
  repositories?: Array<{
    repository_id: string;
    base_branch?: string;
    position: number;
  }>;
};

type RepoAPI = {
  id: string;
  name: string;
  provider?: string;
  provider_owner?: string;
  provider_name?: string;
};

async function fetchTaskRepos(
  apiClient: ApiClient,
  taskId: string,
): Promise<NonNullable<TaskRepoAPI["repositories"]>> {
  const res = await apiClient.rawRequest("GET", `/api/v1/tasks/${taskId}`);
  const body = (await res.json()) as TaskRepoAPI;
  return body.repositories ?? [];
}

async function fetchRepoById(apiClient: ApiClient, repoId: string): Promise<RepoAPI> {
  const res = await apiClient.rawRequest("GET", `/api/v1/repositories/${repoId}`);
  return (await res.json()) as RepoAPI;
}

async function openCreateDialog(testPage: Page, kanban: KanbanPage): Promise<void> {
  await kanban.goto();
  await kanban.createTaskButton.first().click();
  await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();
}

async function clickRemoteMode(testPage: Page): Promise<void> {
  const remoteBtn = testPage.getByTestId("source-mode-remote");
  await expect(remoteBtn).toBeVisible();
  await remoteBtn.click();
}

async function seedAccessibleRepos(apiClient: ApiClient): Promise<void> {
  // The mock's ListUserRepos returns repos under the authenticated user's
  // login (default "mock-user"). Seeding under that key surfaces the repos
  // in the Remote-tab picker without needing to also add orgs.
  await apiClient.mockGitHubAddRepos("mock-user", [
    { full_name: "mock-user/alpha", owner: "mock-user", name: "alpha", private: false },
    { full_name: "mock-user/beta", owner: "mock-user", name: "beta", private: true },
    { full_name: "mock-user/gamma", owner: "mock-user", name: "gamma", private: false },
  ]);
  await apiClient.mockGitHubAddBranches("mock-user", "alpha", [
    { name: "main" },
    { name: "develop" },
  ]);
  await apiClient.mockGitHubAddBranches("mock-user", "beta", [{ name: "main" }]);
  await apiClient.mockGitHubAddBranches("mock-user", "gamma", [{ name: "main" }]);
}

async function pickRepoInChip(testPage: Page, repoFullName: string, chipIndex = 0): Promise<void> {
  const triggers = testPage.getByTestId("remote-repo-chip-trigger");
  await triggers.nth(chipIndex).click();
  // Pick by repo full_name instead of relying on list order — the picker's
  // sort can change without notice and we want each call site to spell out
  // which repo it expects to land on (later assertions reference the same
  // name). Filtering the option testid by text scopes the click to the
  // intended row even when the description / private badge add extra text.
  const option = testPage.getByTestId("remote-repo-option").filter({ hasText: repoFullName });
  await expect(option).toBeVisible({ timeout: 10_000 });
  await option.first().click();
}

async function pasteUrlInChip(testPage: Page, url: string, chipIndex = 0): Promise<void> {
  const triggers = testPage.getByTestId("remote-repo-chip-trigger");
  await triggers.nth(chipIndex).click();
  // The chip-popover content is rendered inline with `portal=false`; when
  // multiple chips have popovers open (or Radix is mid-transition between
  // opens), the same testid can briefly resolve to more than one element,
  // so target the last (currently focused) one.
  const pasteInput = testPage.getByTestId("remote-repo-input").last();
  await expect(pasteInput).toBeVisible();
  await pasteInput.fill(url);
  await pasteInput.press("Enter");
}

async function expectPopoverFitsDialog(testPage: Page): Promise<void> {
  const dialogBox = await testPage.getByTestId("create-task-dialog").boundingBox();
  const popoverBox = await testPage.getByTestId("remote-repo-popover-content").boundingBox();
  expect(dialogBox).not.toBeNull();
  expect(popoverBox).not.toBeNull();
  expect(popoverBox!.y + popoverBox!.height).toBeLessThanOrEqual(
    dialogBox!.y + dialogBox!.height + 1,
  );
}

test.describe("Task creation from Remote tab (chip picker)", () => {
  test.describe.configure({ retries: 1 });

  test.beforeEach(async ({ apiClient }) => {
    // Reset mock state so a previous test's seeded repos / 503 toggle
    // don't bleed into this one. mockGitHubReset clears the toggle and
    // mock-controller.reset on the backend additionally clears the
    // service's accessible-repos / user-orgs caches.
    await apiClient.mockGitHubReset();
  });

  test("keeps the unified input fixed while repositories load", async ({ testPage, apiClient }) => {
    await seedAccessibleRepos(apiClient);
    let releaseRepos = () => undefined;
    const reposGate = new Promise<void>((resolve) => {
      releaseRepos = resolve;
    });
    await testPage.route("**/api/v1/github/repos?*", async (route) => {
      await reposGate;
      await route.continue();
    });

    const kanban = new KanbanPage(testPage);
    await openCreateDialog(testPage, kanban);
    await clickRemoteMode(testPage);
    await testPage.getByTestId("remote-repo-chip-trigger").first().click();

    const popover = testPage.getByTestId("remote-repo-popover-content");
    const input = testPage.getByTestId("remote-repo-input");
    await expect(testPage.getByTestId("remote-repo-picker-loading")).toBeVisible();
    // Measure the loading state after the finite popover entrance animation.
    // Otherwise its scale transform makes the first box a few pixels smaller
    // than the loaded-state box even though the layout itself never shifts.
    await popover.evaluate(async (element) => {
      await Promise.all(
        element.getAnimations().map((animation) => animation.finished.catch(() => undefined)),
      );
    });
    const [loadingPopoverBox, loadingInputBox] = await Promise.all([
      popover.boundingBox(),
      input.boundingBox(),
    ]);

    releaseRepos();
    await expect(
      testPage.getByTestId("remote-repo-option").filter({ hasText: "mock-user/alpha" }),
    ).toBeVisible();
    const [loadedPopoverBox, loadedInputBox] = await Promise.all([
      popover.boundingBox(),
      input.boundingBox(),
    ]);

    expect(loadingPopoverBox).not.toBeNull();
    expect(loadingInputBox).not.toBeNull();
    expect(loadedPopoverBox).not.toBeNull();
    expect(loadedInputBox).not.toBeNull();
    expect(Math.abs(loadedPopoverBox!.height - loadingPopoverBox!.height)).toBeLessThanOrEqual(2);
    expect(Math.abs(loadedInputBox!.y - loadingInputBox!.y)).toBeLessThanOrEqual(2);
  });

  test("scenario 1: picker selection submits one repo", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    await seedAccessibleRepos(apiClient);

    const kanban = new KanbanPage(testPage);
    await openCreateDialog(testPage, kanban);
    await clickRemoteMode(testPage);

    await pickRepoInChip(testPage, "mock-user/alpha");

    // After picking, the chip trigger shows owner/name (full_name).
    const trigger = testPage.getByTestId("remote-repo-chip-trigger").first();
    await expect(trigger).toContainText("mock-user/alpha", { timeout: 5_000 });

    // The branch pill auto-loads — once branches resolve, it becomes enabled.
    const branchPill = testPage.getByTestId("remote-branch-chip-trigger").first();
    await expect(branchPill).toBeEnabled({ timeout: 10_000 });

    await testPage.getByTestId("task-title-input").fill("Remote picker happy path");
    await testPage.getByTestId("task-description-input").fill("test");

    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });
    await startBtn.click();

    await expect(testPage.getByTestId("create-task-dialog")).not.toBeVisible({
      timeout: 10_000,
    });

    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    // Verify the task carries exactly one repository row resolved from the
    // picked URL.
    const taskList = await apiClient.listTasks(seedData.workspaceId);
    const created = taskList.tasks.find((t) => t.title === "Remote picker happy path");
    expect(created).toBeDefined();
    const repos = await fetchTaskRepos(apiClient, created!.id);
    expect(repos).toHaveLength(1);
    const repoRow = await fetchRepoById(apiClient, repos[0].repository_id);
    expect(repoRow.provider_owner).toBe("mock-user");
    expect(repoRow.provider_name).toBe("alpha");
  });

  test("scenario 2: single paste URL submits one repo", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    // Seed the branches for the URL we'll paste so the branch pill can
    // resolve. The picker is not exercised here so the repo list doesn't
    // need to contain this repo.
    await apiClient.mockGitHubAddBranches("paste-owner", "paste-repo", [{ name: "main" }]);

    const kanban = new KanbanPage(testPage);
    await openCreateDialog(testPage, kanban);
    await clickRemoteMode(testPage);

    await pasteUrlInChip(testPage, "https://github.com/paste-owner/paste-repo");

    // After paste, the chip trigger displays a middle-truncated form of the
    // URL (e.g. "github.com/pas…ner/paste-repo"). The trailing repo name
    // survives the truncation, so assert on that.
    const trigger = testPage.getByTestId("remote-repo-chip-trigger").first();
    await expect(trigger).toContainText("paste-repo", { timeout: 5_000 });
    await expect(testPage.getByTestId("remote-branch-chip-trigger").first()).toContainText("main", {
      timeout: 10_000,
    });

    await testPage.getByTestId("task-title-input").fill("Remote paste happy path");
    await testPage.getByTestId("task-description-input").fill("test");

    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });
    await startBtn.click();

    await expect(testPage.getByTestId("create-task-dialog")).not.toBeVisible({
      timeout: 10_000,
    });

    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const taskList = await apiClient.listTasks(seedData.workspaceId);
    const created = taskList.tasks.find((t) => t.title === "Remote paste happy path");
    expect(created).toBeDefined();
    const repos = await fetchTaskRepos(apiClient, created!.id);
    expect(repos).toHaveLength(1);
    const repoRow = await fetchRepoById(apiClient, repos[0].repository_id);
    expect(repoRow.provider_owner).toBe("paste-owner");
    expect(repoRow.provider_name).toBe("paste-repo");
  });

  test("pasted GitHub issue URL fills the title and keeps the picker inside the dialog", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);
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

    const kanban = new KanbanPage(testPage);
    await openCreateDialog(testPage, kanban);
    await clickRemoteMode(testPage);

    await testPage.getByTestId("remote-repo-chip-trigger").first().click();
    await expectPopoverFitsDialog(testPage);
    const pasteInput = testPage.getByTestId("remote-repo-input").last();
    await pasteInput.fill("https://github.com/issue-owner/issue-repo/issues/1456");
    await pasteInput.press("Enter");

    await expect(testPage.getByTestId("task-title-input")).toHaveValue(
      "Issue #1456: Fix remote repo picker clipping",
      { timeout: 10_000 },
    );
    await expect(testPage.getByTestId("remote-repo-chip-trigger").first()).toContainText(
      "issues/1456",
      { timeout: 5_000 },
    );
  });

  test("scenario 3: three mixed rows (picker + paste-repo + paste-PR)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    await seedAccessibleRepos(apiClient);
    // Seed branches + a PR for the paste-PR row so PR auto-resolution works.
    await apiClient.mockGitHubAddBranches("paste-owner", "paste-repo", [{ name: "main" }]);
    await apiClient.mockGitHubAddBranches("pr-owner", "pr-repo", [
      { name: "main" },
      { name: "feature/pr-branch" },
    ]);
    await apiClient.mockGitHubAddPRs([
      {
        number: 42,
        title: "PR for scenario 3",
        state: "open",
        head_branch: "feature/pr-branch",
        base_branch: "main",
        author_login: "mock-user",
        repo_owner: "pr-owner",
        repo_name: "pr-repo",
      },
    ]);

    const kanban = new KanbanPage(testPage);
    await openCreateDialog(testPage, kanban);
    await clickRemoteMode(testPage);

    // Row 0: picker selection (mock-user/alpha)
    await pickRepoInChip(testPage, "mock-user/alpha", 0);
    await expect(testPage.getByTestId("remote-repo-chip-trigger").nth(0)).toContainText(
      "mock-user/alpha",
      { timeout: 5_000 },
    );

    // Add row 1 and paste a repo URL. Trigger label is middle-truncated
    // (e.g. "github.com/pas…ner/paste-repo"), so assert on the trailing
    // repo name.
    await testPage.getByTestId("remote-add-row").click();
    await pasteUrlInChip(testPage, "https://github.com/paste-owner/paste-repo", 1);
    await expect(testPage.getByTestId("remote-repo-chip-trigger").nth(1)).toContainText(
      "paste-repo",
      { timeout: 5_000 },
    );

    // Add row 2 and paste a PR URL.
    await testPage.getByTestId("remote-add-row").click();
    await pasteUrlInChip(testPage, "https://github.com/pr-owner/pr-repo/pull/42", 2);
    await expect(testPage.getByTestId("remote-repo-chip-trigger").nth(2)).toContainText("pull/42", {
      timeout: 5_000,
    });

    // All three chips visible.
    await expect(testPage.getByTestId("remote-repo-chip")).toHaveCount(3);

    await testPage.getByTestId("task-title-input").fill("Remote multi-row");
    await testPage.getByTestId("task-description-input").fill("test");

    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });
    await startBtn.click();

    await expect(testPage.getByTestId("create-task-dialog")).not.toBeVisible({
      timeout: 10_000,
    });

    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    // Verify the task carries three TaskRepository rows in the order they
    // were added.
    const taskList = await apiClient.listTasks(seedData.workspaceId);
    const created = taskList.tasks.find((t) => t.title === "Remote multi-row");
    expect(created).toBeDefined();
    const repos = await fetchTaskRepos(apiClient, created!.id);
    expect(repos).toHaveLength(3);

    const sorted = [...repos].sort((a, b) => a.position - b.position);
    const ownersInOrder = await Promise.all(
      sorted.map(async (r) => {
        const repo = await fetchRepoById(apiClient, r.repository_id);
        return `${repo.provider_owner}/${repo.provider_name}`;
      }),
    );
    expect(ownersInOrder).toEqual([
      "mock-user/alpha",
      "paste-owner/paste-repo",
      "pr-owner/pr-repo",
    ]);
  });

  test("scenario 4: add then immediately remove the new row", async ({ testPage, apiClient }) => {
    await seedAccessibleRepos(apiClient);

    const kanban = new KanbanPage(testPage);
    await openCreateDialog(testPage, kanban);
    await clickRemoteMode(testPage);

    await pickRepoInChip(testPage, "mock-user/alpha", 0);

    // Add a second row, then remove it via the per-chip × button.
    await testPage.getByTestId("remote-add-row").click();
    await expect(testPage.getByTestId("remote-repo-chip")).toHaveCount(2);

    await testPage.getByTestId("remote-chip-remove").nth(1).click();
    await expect(testPage.getByTestId("remote-repo-chip")).toHaveCount(1);

    // The original chip still carries its picker selection.
    await expect(testPage.getByTestId("remote-repo-chip-trigger").first()).toContainText(
      "mock-user/alpha",
    );

    await testPage.getByTestId("task-title-input").fill("Add then remove");
    await testPage.getByTestId("task-description-input").fill("test");

    // Form is still submittable.
    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });
  });

  test("scenario 5: no repository provider shows banner, paste still works", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    // Flip the mock so /api/v1/github/repos responds with 503
    // `github_not_configured`. Paste-URL paths (branch fetch) still work —
    // the GET /repos/:owner/:repo/branches endpoint uses the mock client's
    // ListRepoBranches, which we leave unaffected by the toggle.
    await apiClient.mockGitHubSetReposUnavailable(true);
    await apiClient.mockGitHubAddBranches("banner-owner", "banner-repo", [{ name: "main" }]);

    const kanban = new KanbanPage(testPage);
    await openCreateDialog(testPage, kanban);
    await clickRemoteMode(testPage);

    // Open the chip popover — picker section should show the provider-neutral
    // CTA pointing at the integrations settings page.
    const trigger = testPage.getByTestId("remote-repo-chip-trigger").first();
    await trigger.click();

    const banner = testPage.getByText("Connect a source control provider", { exact: false });
    await expect(banner).toBeVisible({ timeout: 10_000 });

    const settingsLink = testPage.locator('a[href="/settings/integrations"]');
    await expect(settingsLink).toBeVisible();

    // The paste input is still rendered and usable — paste a URL.
    const pasteInput = testPage.getByTestId("remote-repo-input");
    await expect(pasteInput).toBeVisible();
    await pasteInput.fill("https://github.com/banner-owner/banner-repo");
    await pasteInput.press("Enter");

    await expect(trigger).toContainText("banner-repo", { timeout: 5_000 });

    await testPage.getByTestId("task-title-input").fill("Banner paste fallback");
    await testPage.getByTestId("task-description-input").fill("test");

    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });
    await startBtn.click();
    await expect(testPage.getByTestId("create-task-dialog")).not.toBeVisible({
      timeout: 10_000,
    });

    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    // Confirm the task was created with the pasted repo.
    const taskList = await apiClient.listTasks(seedData.workspaceId);
    const created = taskList.tasks.find((t) => t.title === "Banner paste fallback");
    expect(created).toBeDefined();
    const repos = await fetchTaskRepos(apiClient, created!.id);
    expect(repos).toHaveLength(1);
    const repoRow = await fetchRepoById(apiClient, repos[0].repository_id);
    expect(repoRow.provider_owner).toBe("banner-owner");
    expect(repoRow.provider_name).toBe("banner-repo");
  });

  test("scenario 6: mode-switch preserves remote rows", async ({ testPage, apiClient }) => {
    await seedAccessibleRepos(apiClient);
    await apiClient.mockGitHubAddBranches("paste-owner", "paste-repo", [{ name: "main" }]);

    const kanban = new KanbanPage(testPage);
    await openCreateDialog(testPage, kanban);
    await clickRemoteMode(testPage);

    await pickRepoInChip(testPage, "mock-user/alpha", 0);
    await testPage.getByTestId("remote-add-row").click();
    await pasteUrlInChip(testPage, "https://github.com/paste-owner/paste-repo", 1);

    await expect(testPage.getByTestId("remote-repo-chip")).toHaveCount(2);

    // Switch to workspace mode (Repo) and back to Remote — both chips
    // should still be present with their selections intact.
    await testPage.getByTestId("source-mode-workspace").click();
    // Remote chips should not be rendered in workspace mode.
    await expect(testPage.getByTestId("remote-repo-chip")).toHaveCount(0);

    await clickRemoteMode(testPage);
    await expect(testPage.getByTestId("remote-repo-chip")).toHaveCount(2);

    await expect(testPage.getByTestId("remote-repo-chip-trigger").nth(0)).toContainText(
      "mock-user/alpha",
    );
    await expect(testPage.getByTestId("remote-repo-chip-trigger").nth(1)).toContainText(
      "paste-repo",
    );
  });

  test("switches between configured repository providers with bottom tabs", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedAccessibleRepos(apiClient);
    await apiClient.configureGitLab(seedData.workspaceId);
    await apiClient.mockAzureDevOpsSeed({
      authenticated: true,
      projects: [{ id: "project-1", name: "Platform" }],
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

    const kanban = new KanbanPage(testPage);
    await openCreateDialog(testPage, kanban);
    await clickRemoteMode(testPage);
    await testPage.getByTestId("remote-repo-chip-trigger").first().click();

    const tabs = testPage.getByTestId("remote-repo-provider-tabs");
    await expect(tabs.getByRole("tab", { name: "GitHub" })).toBeVisible();
    await expect(tabs.getByRole("tab", { name: "GitLab" })).toBeVisible();
    await expect(tabs.getByRole("tab", { name: "Azure DevOps" })).toBeVisible();
    const tabOverflow = await tabs.evaluate((element) => ({
      overflowY: getComputedStyle(element).overflowY,
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
    }));
    expect(tabOverflow.overflowY).toBe("hidden");
    expect(tabOverflow.scrollHeight).toBeLessThanOrEqual(tabOverflow.clientHeight);
    await expect(
      testPage.getByTestId("remote-repo-option").filter({ hasText: "mock-user/alpha" }),
    ).toBeVisible();
    await expect(
      testPage.getByTestId("remote-repo-option").filter({ hasText: "Platform/api" }),
    ).toHaveCount(0);

    await tabs.getByRole("tab", { name: "GitLab" }).click();
    await expect(
      testPage.getByTestId("remote-repo-option").filter({ hasText: "kandev/sample" }),
    ).toBeVisible();

    await tabs.getByRole("tab", { name: "Azure DevOps" }).click();
    const azureOption = testPage
      .getByTestId("remote-repo-option")
      .filter({ hasText: "Platform/api" });
    await expect(azureOption).toBeVisible();
    await azureOption.click();
    await expect(testPage.getByTestId("remote-repo-chip-trigger").first()).toContainText(
      "Platform/api",
    );
  });
});
