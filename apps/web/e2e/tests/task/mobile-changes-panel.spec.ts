import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import type { Page } from "@playwright/test";
import path from "node:path";

async function openMobileChangesPanel(testPage: Page) {
  await testPage.getByRole("button", { name: "Changes" }).tap();
  await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible({ timeout: 15_000 });
}

async function expandSection(testPage: Page, sectionTestId: string) {
  const toggle = testPage.getByTestId(`${sectionTestId}-collapse-toggle`);
  await expect(toggle).toBeVisible({ timeout: 10_000 });
  if ((await toggle.getAttribute("aria-expanded")) === "false") {
    await toggle.tap();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
  }
}

async function expectDiffText(testPage: Page, text: string, timeout = 45_000) {
  await testPage.waitForFunction(
    (searchText: string) => {
      for (const container of document.querySelectorAll("diffs-container")) {
        const shadow = container.shadowRoot;
        if (shadow?.textContent?.includes(searchText)) return true;
      }
      return false;
    },
    text,
    { timeout },
  );
}

test.describe("Mobile changes panel", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test("renders timeline surface and opens Diff/Review/file/commit overlays", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Changes Surface",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("mobile-committed.txt", "base commit marker");
    git.stageAll();
    git.commit("mobile commit seed");
    git.createFile("mobile-unstaged.txt", "UNSTAGED_MARKER");
    git.createFile("mobile-staged.txt", "STAGED_MARKER");
    git.stageFile("mobile-staged.txt");

    await openMobileChangesPanel(testPage);

    // Regression: mobile Changes tab should be timeline summary, not inline merged diff.
    await expect(testPage.locator("diffs-container")).toHaveCount(0);
    await expect(testPage.getByTestId("mobile-changes-panel").getByRole("tab")).toHaveCount(0);
    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 20_000 });
    await expect(testPage.getByTestId("staged-files-section")).toBeVisible({ timeout: 20_000 });
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 20_000 });

    const mobileChangesPanel = testPage.getByTestId("mobile-changes-panel");

    await mobileChangesPanel.getByRole("button", { name: "Diff" }).tap();
    const closeButton = testPage.getByTestId("mobile-diff-sheet-close");
    await expect(closeButton).toBeVisible({ timeout: 10_000 });

    const committedTab = testPage.getByRole("tab", { name: /^Committed/i });
    await expect(testPage.getByRole("tab", { name: /^Uncommitted/i })).toBeVisible();
    await expect(committedTab).toBeVisible();
    await committedTab.tap();
    await expect(committedTab).toHaveAttribute("aria-selected", "true");
    await closeButton.tap();

    await mobileChangesPanel.getByRole("button", { name: "Review" }).tap();
    const reviewDialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(reviewDialog).toBeVisible({ timeout: 15_000 });
    await testPage.keyboard.press("Escape");
    await expect(reviewDialog).not.toBeVisible({ timeout: 10_000 });

    await testPage.getByTestId("file-row-mobile-unstaged.txt").tap();
    await expect(testPage.getByText("File Changes")).toBeVisible({ timeout: 10_000 });
    await expectDiffText(testPage, "UNSTAGED_MARKER");

    // Regression: mobile diffs must use word-wrap (overflow="wrap") so long
    // lines are readable without horizontal scroll on touch devices.
    const overflowAttr = await testPage.waitForFunction(
      () =>
        document
          .querySelector("diffs-container")
          ?.shadowRoot?.querySelector("[data-diffs]")
          ?.getAttribute("data-overflow"),
      { timeout: 10_000 },
    );
    expect(await overflowAttr.jsonValue()).toBe("wrap");

    await closeButton.tap();

    await expandSection(testPage, "commits-section");
    await testPage.locator("[data-testid^='commit-row-']").first().tap();
    await expect(testPage.getByText("Commit Changes")).toBeVisible({ timeout: 10_000 });
    await closeButton.tap();
  });

  test("tapping a staged file row opens file diff sheet", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Staged File Diff",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("mobile-staged-only.txt", "STAGED_ONLY_MARKER");
    git.stageFile("mobile-staged-only.txt");

    await openMobileChangesPanel(testPage);
    await expandSection(testPage, "staged-files-section");

    await testPage.getByTestId("file-row-mobile-staged-only.txt").tap();
    await expect(testPage.getByText("File Changes")).toBeVisible({ timeout: 10_000 });
    await expectDiffText(testPage, "STAGED_ONLY_MARKER");
    await testPage.getByTestId("mobile-diff-sheet-close").tap();
  });

  test("tapping a PR file row opens file diff sheet with diff content", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile PR File Diff",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 45_000 });

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 42,
        title: "Mobile PR file diff test",
        state: "open",
        head_branch: "feat/mobile-fix",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 3,
        deletions: 0,
      },
    ]);
    await apiClient.mockGitHubAddPRFiles("testorg", "testrepo", 42, [
      {
        filename: "mobile-pr-fix.txt",
        status: "added",
        additions: 3,
        deletions: 0,
        patch:
          "@@ -0,0 +1,3 @@\n+PR_FILE_MARKER_LINE_ONE\n+PR_FILE_MARKER_LINE_TWO\n+PR_FILE_MARKER_LINE_THREE",
      },
    ]);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 42,
      pr_url: "https://github.com/testorg/testrepo/pull/42",
      pr_title: "Mobile PR file diff test",
      head_branch: "feat/mobile-fix",
      base_branch: "main",
      author_login: "test-user",
    });

    await openMobileChangesPanel(testPage);
    await expandSection(testPage, "pr-changes-section");

    const prFilesList = testPage.getByTestId("pr-files-list");
    await expect(prFilesList).toBeVisible({ timeout: 20_000 });
    await prFilesList.getByText("mobile-pr-fix.txt").tap();

    await expect(testPage.getByText("File Changes")).toBeVisible({ timeout: 10_000 });
    await expectDiffText(testPage, "PR_FILE_MARKER_LINE_ONE");

    await testPage.getByTestId("mobile-diff-sheet-close").tap();
  });

  test("tapping PR file row shows PR diff when same file also has local changes", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile PR Overlap Diff",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 45_000 });

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 44,
        title: "Mobile overlap PR diff test",
        state: "open",
        head_branch: "feat/mobile-overlap",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 2,
        deletions: 0,
      },
    ]);
    await apiClient.mockGitHubAddPRFiles("testorg", "testrepo", 44, [
      {
        filename: "overlap-mobile.txt",
        status: "added",
        additions: 2,
        deletions: 0,
        patch: "@@ -0,0 +1,2 @@\n+MOBILE_OVERLAP_PR_MARKER_A\n+MOBILE_OVERLAP_PR_MARKER_B",
      },
    ]);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 44,
      pr_url: "https://github.com/testorg/testrepo/pull/44",
      pr_title: "Mobile overlap PR diff test",
      head_branch: "feat/mobile-overlap",
      base_branch: "main",
      author_login: "test-user",
    });

    // Create local change to the same file — this causes allFiles to deduplicate
    // the PR entry, which was the bug: tapping the PR row showed "No changes".
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("overlap-mobile.txt", "local change LOCAL_CHANGE_MARKER");

    await openMobileChangesPanel(testPage);
    await expandSection(testPage, "pr-changes-section");

    const prFilesList = testPage.getByTestId("pr-files-list");
    await expect(prFilesList).toBeVisible({ timeout: 20_000 });
    await prFilesList.getByText("overlap-mobile.txt").tap();

    await expect(testPage.getByText("File Changes")).toBeVisible({ timeout: 10_000 });
    // PR diff content must appear — not "No changes"
    await expectDiffText(testPage, "MOBILE_OVERLAP_PR_MARKER_A");

    await testPage.getByTestId("mobile-diff-sheet-close").tap();

    git.exec("git clean -fd");
    try {
      git.exec("git checkout -- .");
    } catch {
      // Ignore checkout errors if repo has no tracked files
    }
  });

  test("Diff sheet auto-selects Committed tab when no uncommitted changes exist", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Auto-Select Source",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("committed-only.txt", "committed content");
    git.stageAll();
    git.commit("committed-only seed");

    await openMobileChangesPanel(testPage);
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 20_000 });

    const mobileChangesPanel = testPage.getByTestId("mobile-changes-panel");
    await mobileChangesPanel.getByRole("button", { name: "Diff" }).tap();
    const closeButton = testPage.getByTestId("mobile-diff-sheet-close");
    await expect(closeButton).toBeVisible({ timeout: 10_000 });

    // Single source → no tab bar rendered; title shows source name instead.
    await expect(testPage.getByRole("tab", { name: /^Uncommitted/i })).not.toBeVisible();
    await expect(testPage.getByRole("tab", { name: /^Committed/i })).not.toBeVisible();
    await expect(testPage.getByText("Committed Changes")).toBeVisible({ timeout: 10_000 });

    await closeButton.tap();
  });
});
