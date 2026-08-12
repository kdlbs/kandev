import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";
import path from "node:path";

function seedStaleCheckout(git: GitHelper, remoteUrl: string): string {
  git.exec("git checkout -f main");
  if (git.exec("git remote").split(/\r?\n/).includes("origin")) {
    git.exec(`git remote set-url origin "${remoteUrl}"`);
  } else {
    git.exec(`git remote add origin "${remoteUrl}"`);
  }
  git.exec("git fetch origin main");
  git.exec("git reset --hard origin/main");
  git.exec("git clean -fd");
  git.exec("git branch --set-upstream-to=origin/main main");
  for (let index = 1; index <= 6; index += 1) {
    git.createFile(`mobile-drift-local-${index}.txt`, `local checkout commit ${index}`);
    git.stageFile(`mobile-drift-local-${index}.txt`);
    git.commit(`Mobile contribution commit ${index}`);
  }
  return git.getCurrentSha();
}

function createRewrittenProviderHistory(
  git: GitHelper,
  branch: string,
): {
  head: string;
  commits: Array<{
    sha: string;
    message: string;
    author_login: string;
    author_date: string;
    stats_available: boolean;
  }>;
} {
  git.exec("git checkout -B kandev-e2e-provider-rewrite origin/main");
  const commits: Array<{
    sha: string;
    message: string;
    author_login: string;
    author_date: string;
    stats_available: boolean;
  }> = [];
  for (let index = 1; index <= 15; index += 1) {
    const message = `Mobile rewritten provider commit ${index}`;
    git.createFile(`mobile-provider-rewrite-${index}.txt`, message);
    git.stageFile(`mobile-provider-rewrite-${index}.txt`);
    const sha = git.commit(message);
    commits.push({
      sha,
      message,
      author_login: "mobile-remote-contributor",
      author_date: `2026-08-${String(index).padStart(2, "0")}T12:00:00Z`,
      stats_available: false,
    });
  }
  git.exec(`git push --force origin HEAD:refs/heads/${branch}`);
  git.exec("git checkout -f main");
  return { head: commits[commits.length - 1].sha, commits };
}

async function swipeUpOnElement(page: Page, element: Locator): Promise<void> {
  const box = await element.boundingBox();
  if (!box) throw new Error("Changes scroll container has no bounding box");

  const cdp = await page.context().newCDPSession(page);
  const centerX = box.x + box.width / 2;
  const startY = box.y + box.height - 20;
  const endY = box.y + 20;
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x: centerX, y: startY }],
  });
  for (let step = 1; step <= 8; step += 1) {
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ x: centerX, y: startY + ((endY - startY) * step) / 8 }],
    });
  }
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
}

test.describe("Mobile rewritten contribution history", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test.beforeEach(({ backend, seedData }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.exec("git checkout -f main");
    if (git.exec("git remote").split(/\r?\n/).includes("origin")) {
      git.exec(`git remote set-url origin "${seedData.repositoryRemoteURL}"`);
    } else {
      git.exec(`git remote add origin "${seedData.repositoryRemoteURL}"`);
    }
    git.exec("git fetch origin main");
    git.exec("git reset --hard origin/main");
    git.exec("git clean -fd");
  });

  test("local-first contribution menu preserves local history after a rewrite", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    const localHead = seedStaleCheckout(git, seedData.repositoryRemoteURL);
    const providerBranch = "feature/mobile-rewritten";
    const providerHistory = createRewrittenProviderHistory(git, providerBranch);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Rewritten Contribution History",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("mobile-remote-contributor");
    await apiClient.mockGitHubAddPRs([
      {
        number: 902,
        title: "Mobile rewritten contribution",
        state: "open",
        head_branch: providerBranch,
        base_branch: "main",
        author_login: "mobile-remote-contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
        head_sha: providerHistory.head,
      },
    ]);
    await apiClient.mockGitHubAddPRCommits("testorg", "testrepo", 902, providerHistory.commits);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 902,
      pr_url: "https://github.com/testorg/testrepo/pull/902",
      pr_title: "Mobile rewritten contribution",
      head_branch: "feature/mobile-rewritten",
      base_branch: "main",
      author_login: "mobile-remote-contributor",
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    await testPage.getByRole("button", { name: "Changes" }).tap();

    const changes = testPage.getByTestId("mobile-changes-panel");
    await expect(changes.getByTestId("remote-contribution-drift-status")).toBeVisible({
      timeout: 30_000,
    });
    const providerSection = changes.getByTestId("current-pr-commits-section");
    const localSection = changes.getByTestId("local-checkout-commits-section");
    await expect(providerSection).toBeVisible({ timeout: 10_000 });
    await expect(localSection).toBeVisible({ timeout: 10_000 });
    await expect(
      providerSection.getByTestId("current-pr-commits-section-collapse-toggle"),
    ).toHaveAttribute("aria-expanded", "false");
    await expect(localSection.locator('[data-testid^="commit-row-"]')).toHaveCount(6);
    await providerSection.getByTestId("current-pr-commits-section-collapse-toggle").tap();
    await expect(providerSection.locator('[data-testid^="commit-row-"]')).toHaveCount(15);
    await expect(providerSection.locator(".tabler-icon-arrow-up")).toHaveCount(0);
    await expect(localSection.locator(".tabler-icon-arrow-up")).toHaveCount(0);

    const scrollOwners = changes.locator('[class*="overflow-y-auto"]');
    await expect(scrollOwners).toHaveCount(1);
    const scroller = scrollOwners.first();
    await expect
      .poll(() => scroller.evaluate((element) => element.scrollHeight > element.clientHeight))
      .toBe(true);
    await scroller.evaluate((element) => {
      element.scrollTop = 0;
    });
    await swipeUpOnElement(testPage, scroller);
    await expect.poll(() => scroller.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);

    const gitActions = testPage.getByTestId("mobile-git-actions");
    await gitActions.tap();
    const openMenu = testPage.locator('[data-slot="dropdown-menu-content"][data-state="open"]');
    await expect(openMenu).toHaveCount(1);
    const menuItems = openMenu.locator('[data-slot="dropdown-menu-item"]');
    await expect(menuItems.filter({ hasText: /^Commit/ })).not.toHaveAttribute(
      "aria-disabled",
      "true",
    );
    await expect(openMenu.getByTestId("mobile-replace-pr-branch")).toBeVisible();
    await expect(openMenu.getByTestId("mobile-use-pr-version")).toBeVisible();
    await expect(openMenu.getByTestId("mobile-view-pr-version")).toBeVisible();
    await expect(openMenu).toContainText("Replace the published PR branch");
    await expect(openMenu).toContainText("Use the current PR version");
    await expect(openMenu).toContainText("Open the current PR version");
    await expect(menuItems.filter({ hasText: /^Pull$/ })).toHaveCount(0);
    await expect(openMenu.locator('[data-slot="dropdown-menu-sub-trigger"]')).toHaveCount(0);
    await openMenu.getByTestId("mobile-replace-pr-branch").tap();
    const drawer = testPage.getByTestId("mobile-remote-contribution-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer).toContainText(providerHistory.head);
    await expect(testPage.getByTestId("mobile-remote-contribution-confirm")).toHaveClass(/h-11/);
    await drawer.getByRole("button", { name: "Cancel" }).tap();

    expect(git.getCurrentSha()).toBe(localHead);
    expect(git.exec("git status --porcelain").trim()).toBe("");
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await prCapture.screenshot("remote-contribution-drift-mobile", {
      caption: "Pixel 5 preserves the local checkout and offers explicit provider version choices",
    });
  });
});
