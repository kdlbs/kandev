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

function rewrittenProviderCommits() {
  return Array.from({ length: 15 }, (_, index) => ({
    sha: `${String(index + 1).padStart(2, "0")}${"b".repeat(38)}`,
    message: `Mobile rewritten provider commit ${index + 1}`,
    author_login: "mobile-remote-contributor",
    author_date: `2026-08-${String(index + 1).padStart(2, "0")}T12:00:00Z`,
    stats_available: false,
  }));
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

  test("preserves local history and disables remote mutations after a rewrite", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    const localHead = seedStaleCheckout(git, seedData.repositoryRemoteURL);
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
    const providerCommits = rewrittenProviderCommits();

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("mobile-remote-contributor");
    await apiClient.mockGitHubAddPRs([
      {
        number: 902,
        title: "Mobile rewritten contribution",
        state: "open",
        head_branch: "feature/mobile-rewritten",
        base_branch: "main",
        author_login: "mobile-remote-contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
        head_sha: providerCommits[providerCommits.length - 1].sha,
      },
    ]);
    await apiClient.mockGitHubAddPRCommits("testorg", "testrepo", 902, providerCommits);
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
    await expect(changes.getByTestId("remote-contribution-drift-warning")).toBeVisible({
      timeout: 30_000,
    });
    const providerSection = changes.getByTestId("current-pr-commits-section");
    const localSection = changes.getByTestId("local-checkout-commits-section");
    await expect(providerSection).toBeVisible({ timeout: 10_000 });
    await expect(localSection).toBeVisible({ timeout: 10_000 });
    await expect(providerSection.locator('[data-testid^="commit-row-"]')).toHaveCount(15);
    await expect(localSection.locator('[data-testid^="commit-row-"]')).toHaveCount(6);
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

    const changesPull = changes.getByRole("button", { name: /^Pull$/ });
    await expect(changesPull).toBeDisabled();

    const gitActions = testPage.getByTestId("mobile-git-actions");
    await gitActions.tap();
    const openMenu = testPage.locator('[data-slot="dropdown-menu-content"][data-state="open"]');
    await expect(openMenu).toHaveCount(1);
    const menuItems = openMenu.locator('[data-slot="dropdown-menu-item"]');
    await expect(menuItems.filter({ hasText: /^Commit/ })).not.toHaveAttribute(
      "aria-disabled",
      "true",
    );
    await expect(menuItems.filter({ hasText: /^Pull$/ })).toHaveAttribute("aria-disabled", "true");
    await expect(
      openMenu.locator('[data-slot="dropdown-menu-sub-trigger"]').filter({ hasText: /^Push/ }),
    ).toHaveAttribute("aria-disabled", "true");

    expect(git.getCurrentSha()).toBe(localHead);
    expect(git.exec("git status --porcelain").trim()).toBe("");
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await prCapture.screenshot("remote-contribution-drift-mobile", {
      caption: "Pixel 5 keeps the preserved checkout separate and blocks remote mutations",
    });
  });
});
