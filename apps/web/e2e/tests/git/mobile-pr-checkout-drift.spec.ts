import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { waitForFiniteAnimations } from "../../helpers/animations";
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

function createLocalRebasedProviderHistory(
  git: GitHelper,
  branch: string,
): {
  localHead: string;
  publishedHead: string;
  commits: Array<{
    sha: string;
    message: string;
    author_login: string;
    author_date: string;
    stats_available: boolean;
  }>;
} {
  const publishedBranch = "kandev-e2e-mobile-published-history";
  git.exec(`git checkout -B ${publishedBranch} origin/main`);
  const commits: Array<{
    sha: string;
    message: string;
    author_login: string;
    author_date: string;
    stats_available: boolean;
  }> = [];
  for (let index = 1; index <= 5; index += 1) {
    const message = `Mobile published contribution commit ${index}`;
    git.createFile(`mobile-published-contribution-${index}.txt`, message);
    git.stageFile(`mobile-published-contribution-${index}.txt`);
    const sha = git.commit(message);
    commits.push({
      sha,
      message,
      author_login: "mobile-local-rebase-contributor",
      author_date: `2026-08-${String(index).padStart(2, "0")}T12:00:00Z`,
      stats_available: false,
    });
  }
  const publishedHead = git.getCurrentSha();
  git.exec(`git push --force origin HEAD:refs/heads/${branch}`);

  git.exec("git checkout -B main origin/main");
  for (let index = 1; index <= 3; index += 1) {
    git.createFile(`mobile-newer-base-${index}.txt`, `mobile newer base commit ${index}`);
    git.stageFile(`mobile-newer-base-${index}.txt`);
    git.commit(`Mobile newer base commit ${index}`);
  }
  git.exec("git push --force origin main");

  git.exec(`git checkout -B ${branch} ${publishedHead}`);
  git.exec("git rebase origin/main");
  const localHead = git.getCurrentSha();
  git.exec(`git branch --set-upstream-to=origin/${branch} ${branch}`);
  return { localHead, publishedHead, commits };
}

function configureContributionRemote(
  git: GitHelper,
  remoteName: string,
  sourceRemoteURL: string,
  localRemoteURL: string,
): void {
  const remotes = git.exec("git remote").split(/\r?\n/);
  if (remotes.includes(remoteName)) {
    git.exec(`git remote set-url ${remoteName} "${sourceRemoteURL}"`);
  } else {
    git.exec(`git remote add ${remoteName} "${sourceRemoteURL}"`);
  }
  git.exec(`git config url."${localRemoteURL}".insteadOf "${sourceRemoteURL}"`);
}

function advanceProviderHead(git: GitHelper, previousHead: string, branch: string): string {
  const tree = git.exec(`git rev-parse ${previousHead}^{tree}`).trim();
  const nextHead = git
    .exec(
      `git -c commit.gpgsign=false commit-tree ${tree} -p ${previousHead} -m "Mobile provider head advanced"`,
    )
    .trim();
  git.exec(`git push --force origin ${nextHead}:refs/heads/${branch}`);
  return nextHead;
}

function recoveryBranches(git: GitHelper): string[] {
  return git
    .exec('git for-each-ref --format="%(refname:short)" refs/heads/kandev')
    .split(/\r?\n/)
    .map((branch) => branch.trim())
    .filter(Boolean);
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

  // @covers AC-UI-MOBILE-TASK-CHROME-001.4
  test("Changes recovery menu preserves local history after a rewrite", async ({
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
    const contributionRepository = await apiClient.createRepository(
      seedData.workspaceId,
      repoDir,
      "main",
      {
        name: "E2E GitHub Contribution Repository",
        provider: "github",
        provider_host: "https://github.com",
        provider_owner: "testorg",
        provider_name: "testrepo",
        remote_url: "https://github.com/testorg/testrepo.git",
      },
    );
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Rewritten Contribution History",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [contributionRepository.id],
        start_agent: false,
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
        head_repo_owner: "testorg",
        head_repo_name: "testrepo",
        base_repo_owner: "testorg",
        base_repo_name: "testrepo",
        base_default_branch: "main",
      },
    ]);
    const contribution = await apiClient.attachE2EGitHubContribution(
      task.id,
      "https://github.com/testorg/testrepo/pull/902",
    );
    configureContributionRemote(
      git,
      contribution.remote_name,
      contribution.binding.source_repository.remote_url,
      seedData.repositoryRemoteURL,
    );
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
    await apiClient.ensureTaskSession(task.id);
    // The first provider-history request fails. The resource must retry it
    // without dropping the local checkout history from the final panel.
    await apiClient.mockGitHubSetPRCommitsFailures("testorg", "testrepo", 902, 1);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    git.exec(`git checkout -B ${providerBranch} ${localHead}`);
    git.exec(`git branch --set-upstream-to=origin/${providerBranch} ${providerBranch}`);
    await testPage.getByRole("button", { name: "Changes" }).tap();

    const changes = testPage.getByTestId("mobile-changes-panel");
    await expect(changes.getByTestId("remote-contribution-drift-status")).toHaveCount(0);
    const providerSection = changes.getByTestId("current-pr-commits-section");
    const localSection = changes.getByTestId("local-checkout-commits-section");
    await expect(providerSection).toBeVisible({ timeout: 10_000 });
    await expect(localSection).toBeVisible({ timeout: 10_000 });
    await expect(
      providerSection.getByTestId("current-pr-commits-section-collapse-toggle"),
    ).toContainText("PR #902 version");
    await expect(
      providerSection.getByTestId("current-pr-commits-section-collapse-toggle"),
    ).toHaveAttribute("aria-expanded", "false");
    await expect(localSection.locator('[data-testid^="commit-row-"]')).toHaveCount(6);
    await expect(localSection.locator('[data-commit-provenance="local_checkout"]')).toHaveCount(6);
    const contributionWarning = changes.getByTestId("header-remote-contribution-warning");
    await expect(contributionWarning).toBeVisible();
    await contributionWarning.tap();
    const comparisonMenu = testPage.getByTestId("header-remote-contribution-menu");
    await expect(comparisonMenu).toBeVisible();
    const canonicalDrawer = testPage.locator('[data-slot="drawer-content"]');
    const canonicalDrawerBox = await canonicalDrawer.boundingBox();
    expect(canonicalDrawerBox).not.toBeNull();
    const canonicalViewport = await testPage.evaluate(() => ({ height: innerHeight }));
    expect(canonicalDrawerBox!.y).toBeGreaterThan(0);
    expect(canonicalDrawerBox!.height).toBeLessThan(canonicalViewport.height);
    await expect(comparisonMenu.getByRole("button", { name: "Close" })).toBeVisible();
    await expect(comparisonMenu.getByTestId("header-compare-versions")).toBeVisible();
    await comparisonMenu.getByTestId("header-compare-versions").tap();
    await expect(comparisonMenu).toHaveCount(0);
    const providerToggle = providerSection.getByTestId(
      "current-pr-commits-section-collapse-toggle",
    );
    await expect(providerToggle).toHaveAttribute("aria-expanded", "true");
    await expect(providerToggle).toBeFocused();
    expect(git.getCurrentSha()).toBe(localHead);
    await expect(providerSection.locator('[data-commit-provenance="current_pr"]')).toHaveCount(15);
    await expect(
      providerSection.locator('[data-commit-provenance="current_pr"]').first(),
    ).toHaveAttribute("title", "Current PR commit");
    await expect(
      localSection.locator('[data-commit-provenance="local_checkout"]').first(),
    ).toHaveAttribute("title", "Local checkout commit");
    await expect(providerSection.locator('[data-testid^="commit-row-"]')).toHaveCount(15);
    await expect(providerSection.locator('[data-testid^="commit-row-"]').first()).toContainText(
      "Mobile rewritten provider commit 15",
    );
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

    const warningBox = await contributionWarning.boundingBox();
    expect(warningBox).not.toBeNull();
    expect(Math.round(warningBox!.width)).toBeGreaterThanOrEqual(44);
    expect(Math.round(warningBox!.height)).toBeGreaterThanOrEqual(44);
    // Phone composition extends through 767px. Exercise the wider touch range
    // where an `sm:` reset could otherwise shrink touch controls before `md`.
    await testPage.setViewportSize({ width: 700, height: 500 });
    await contributionWarning.click();
    const openMenu = testPage.getByTestId("header-remote-contribution-menu");
    await expect(openMenu).toHaveCount(1);
    const wideDrawer = testPage.locator('[data-slot="drawer-content"]');
    const wideDrawerBox = await wideDrawer.boundingBox();
    expect(wideDrawerBox).not.toBeNull();
    expect(wideDrawerBox!.y).toBeGreaterThan(0);
    expect(wideDrawerBox!.height).toBeLessThan(500);
    await expect(openMenu.getByRole("button", { name: "Close" })).toBeVisible();
    await expect(openMenu.getByTestId("header-replace-pr-branch")).toBeVisible();
    await expect(openMenu.getByTestId("header-use-pr-version")).toBeVisible();
    await expect(openMenu.getByTestId("header-view-pr-version")).toContainText("Open PR on GitHub");
    await expect(openMenu).toContainText("Replace the published PR history");
    await expect(openMenu).toContainText("Replace the task checkout history");
    await expect(openMenu.locator('[data-slot="dropdown-menu-sub-trigger"]')).toHaveCount(0);
    await openMenu.getByTestId("header-replace-pr-branch").tap();
    const dialog = testPage.getByTestId("remote-contribution-resolution-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText(providerHistory.head);
    const cancel = dialog.getByRole("button", { name: "Cancel" });
    await expect
      .poll(async () => Math.round((await cancel.boundingBox())?.height ?? 0))
      .toBeGreaterThanOrEqual(44);
    const confirm = testPage.getByTestId("remote-contribution-confirm");
    await expect
      .poll(async () => Math.round((await confirm.boundingBox())?.height ?? 0))
      .toBeGreaterThanOrEqual(44);
    const changedProviderHead = advanceProviderHead(git, providerHistory.head, providerBranch);
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
        head_sha: changedProviderHead,
        head_repo_owner: "testorg",
        head_repo_name: "testrepo",
        base_repo_owner: "testorg",
        base_repo_name: "testrepo",
        base_default_branch: "main",
      },
    ]);
    await apiClient.mockGitHubAddPRCommits("testorg", "testrepo", 902, [
      {
        sha: changedProviderHead,
        message: "Mobile provider head advanced",
        author_login: "mobile-remote-contributor",
        author_date: "2026-08-20T12:00:00Z",
        stats_available: false,
      },
    ]);
    await confirm.tap();
    await expect(dialog).toContainText("The PR changed again");
    await expect(dialog).toContainText(changedProviderHead);
    expect(git.getCurrentSha()).toBe(localHead);
    expect(git.exec("git status --porcelain").trim()).toBe("");
    expect(git.exec(`git ls-remote origin refs/heads/${providerBranch}`)).toContain(
      changedProviderHead,
    );
    await cancel.tap();
    await expect(dialog).toHaveCount(0);

    expect(git.getCurrentSha()).toBe(localHead);
    expect(git.exec("git status --porcelain").trim()).toBe("");
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await prCapture.screenshot("remote-contribution-drift-mobile", {
      caption:
        "700px touch Changes preserves the local checkout and offers provider version choices",
    });
  });

  test("shows a real local-rebase explanation on the canonical phone viewport", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    const providerBranch = "feature/mobile-local-rebase";
    const history = createLocalRebasedProviderHistory(git, providerBranch);
    // Start session preparation from the published branch. The real rebase is
    // recreated after the session owns the contribution binding.
    git.exec(`git checkout -B ${providerBranch} ${history.publishedHead}`);
    git.exec(`git branch --set-upstream-to=origin/${providerBranch} ${providerBranch}`);
    const contributionRepository = await apiClient.createRepository(
      seedData.workspaceId,
      repoDir,
      "main",
      {
        name: "E2E GitHub Contribution Repository",
        provider: "github",
        provider_host: "https://github.com",
        provider_owner: "testorg",
        provider_name: "testrepo",
        remote_url: "https://github.com/testorg/testrepo.git",
      },
    );
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("mobile-local-rebase-contributor");
    await apiClient.mockGitHubAddPRs([
      {
        number: 905,
        title: "Mobile local rebase explanation",
        state: "open",
        head_branch: providerBranch,
        base_branch: "main",
        author_login: "mobile-local-rebase-contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
        head_sha: history.publishedHead,
        head_repo_owner: "testorg",
        head_repo_name: "testrepo",
        base_repo_owner: "testorg",
        base_repo_name: "testrepo",
        base_default_branch: "main",
      },
    ]);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Local Rebase Explanation",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [contributionRepository.id],
        start_agent: false,
      },
    );

    const contribution = await apiClient.attachE2EGitHubContribution(
      task.id,
      "https://github.com/testorg/testrepo/pull/905",
    );
    configureContributionRemote(
      git,
      contribution.remote_name,
      contribution.binding.source_repository.remote_url,
      seedData.repositoryRemoteURL,
    );
    await apiClient.mockGitHubAddPRCommits("testorg", "testrepo", 905, history.commits);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 905,
      pr_url: "https://github.com/testorg/testrepo/pull/905",
      pr_title: "Mobile local rebase explanation",
      head_branch: providerBranch,
      base_branch: "main",
      author_login: "mobile-local-rebase-contributor",
    });
    await apiClient.ensureTaskSession(task.id);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });

    git.exec(`git checkout -B ${providerBranch} ${history.publishedHead}`);
    git.exec("git rebase origin/main");
    history.localHead = git.getCurrentSha();
    git.exec(`git branch --set-upstream-to=origin/${providerBranch} ${providerBranch}`);

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    await testPage.getByRole("button", { name: "Changes" }).tap();

    const changes = testPage.getByTestId("mobile-changes-panel");
    const providerSection = changes.getByTestId("current-pr-commits-section");
    const localSection = changes.getByTestId("local-checkout-commits-section");
    await expect(providerSection).toBeVisible({ timeout: 15_000 });
    await expect(localSection).toBeVisible({ timeout: 15_000 });

    const warning = changes.getByTestId("header-remote-contribution-warning");
    await warning.tap();
    const menu = testPage.getByTestId("header-remote-contribution-menu");
    await expect(menu).toBeVisible();
    await expect(menu).toContainText("Task and PR histories differ");
    await expect(menu).toContainText("The task branch was rebased locally");
    await expect(menu).toContainText("Task commits: 5");
    await expect(menu).toContainText("Published commits: 5");
    await expect(menu).toContainText("Newer base commits: 3");
    await expect(menu.getByRole("button", { name: "Close" })).toBeVisible();
    await waitForFiniteAnimations(menu);
    await prCapture.screenshot("remote-contribution-menu-mobile", {
      caption: "Phone history drawer explains the local rebase and version choices",
    });

    await menu.getByTestId("header-compare-versions").tap();
    await expect(menu).toHaveCount(0);
    await expect(
      providerSection.getByTestId("current-pr-commits-section-collapse-toggle"),
    ).toHaveAttribute("aria-expanded", "true");
    await expect(
      providerSection.getByTestId("current-pr-commits-section-collapse-toggle"),
    ).toBeFocused();
    await expect(providerSection.locator('[data-testid^="commit-row-"]')).toHaveCount(5);
    expect(git.getCurrentSha()).toBe(history.localHead);
    expect(git.exec("git status --porcelain").trim()).toBe("");
    await expect(
      testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).resolves.toBe(true);

    await warning.tap();
    const resolutionMenu = testPage.getByTestId("header-remote-contribution-menu");
    await expect(resolutionMenu).toBeVisible();
    const recoveryBefore = recoveryBranches(git);
    git.createFile("mobile-local-rebase-dirty.txt", "uncommitted mobile local change");
    await resolutionMenu.getByTestId("header-use-pr-version").tap();
    const dialog = testPage.getByTestId("remote-contribution-resolution-dialog");
    await expect(dialog).toContainText("replaces the current task checkout history");
    await expect(dialog).toContainText(history.publishedHead);
    await dialog.getByTestId("remote-contribution-confirm").tap();
    await expect(dialog).toContainText("Commit or discard your local changes");
    expect(git.getCurrentSha()).toBe(history.localHead);
    expect(git.exec("git status --porcelain").trim()).toContain("mobile-local-rebase-dirty.txt");
    expect(recoveryBranches(git)).toEqual(recoveryBefore);

    git.deleteFile("mobile-local-rebase-dirty.txt");
    await dialog.getByTestId("remote-contribution-confirm").tap();
    await expect(dialog).toHaveCount(0);
    expect(git.getCurrentSha()).toBe(history.publishedHead);
    expect(git.exec("git status --porcelain").trim()).toBe("");
    const recoveryAfter = recoveryBranches(git);
    const createdRecoveryBranches = recoveryAfter.filter(
      (branch) => !recoveryBefore.includes(branch),
    );
    expect(createdRecoveryBranches).toHaveLength(1);
    expect(git.exec(`git rev-parse refs/heads/${createdRecoveryBranches[0]}`).trim()).toBe(
      history.localHead,
    );
  });
});
