import { randomUUID } from "node:crypto";
import { expect, type Page } from "@playwright/test";
import {
  createEmptyRemoteRepository,
  removeTestRepository,
} from "../../helpers/empty-remote-repository";
import { GitHelper } from "../../helpers/git-helper";
import type { ApiClient } from "../../helpers/api-client";
import type { SessionPage } from "../../pages/session-page";

const PR_NUMBER = 3879;
const HEAD_BRANCH = "feature/fork-pr-launch";

export async function expectTerminalCommit(page: Page, commitOID: string): Promise<void> {
  const readUnwrappedTerminal = async () =>
    page
      .locator('[data-testid="terminal-panel"]:visible')
      .first()
      .locator(".xterm")
      .evaluate((xtermElement) => {
        const container = xtermElement.parentElement as HTMLElement & {
          __xtermReadBuffer?: () => string;
        };
        return (container.__xtermReadBuffer?.() ?? "").replace(/\s+/gu, "");
      });

  await expect
    .poll(readUnwrappedTerminal, {
      timeout: 10_000,
      message: `Expected terminal to contain commit ${commitOID} without line wrapping`,
    })
    .toContain(commitOID);
}

export type PRLinkForkLaunchFixture = {
  repositoryId: string;
  repositoryName: string;
  upstreamOwner: string;
  upstreamRepository: string;
  forkOwner: string;
  forkRepository: string;
  prURL: string;
  headBranch: string;
  headOID: string;
  targetOID: string;
  forkCleanup: () => void;
  upstreamCleanup: () => void;
};

export async function expectForkPRLaunchState(
  page: Page,
  session: SessionPage,
  apiClient: ApiClient,
  fixture: PRLinkForkLaunchFixture,
  taskId: string,
): Promise<void> {
  await expect(session.terminal).toBeVisible();
  await session.typeInTerminal("git branch --show-current");
  await session.expectTerminalHasText(fixture.headBranch);
  await session.typeInTerminal("git rev-parse HEAD");
  await expectTerminalCommit(page, fixture.headOID);
  await session.typeInTerminal("git rev-parse refs/remotes/origin/main");
  await expectTerminalCommit(page, fixture.targetOID);

  await page.reload();
  await session.waitForLoad();
  const task = await apiClient.getTask(taskId);
  expect(task.repositories?.[0]?.checkout_branch).toBe(fixture.headBranch);
  expect(task.repositories?.[0]?.base_branch).toBe("main");
  await expect
    .poll(async () => (await apiClient.getTask(taskId)).status_summary?.pull_request?.number, {
      timeout: 15_000,
      message: "waiting for the persisted PR summary to reach the task list",
    })
    .toBe(PR_NUMBER);
  await expect
    .poll(async () =>
      (await apiClient.listTaskPRs(taskId)).map((pr) => ({
        owner: pr.owner,
        repo: pr.repo,
        pr_number: pr.pr_number,
        head_branch: pr.head_branch,
      })),
    )
    .toEqual([
      {
        owner: fixture.upstreamOwner,
        repo: fixture.upstreamRepository,
        pr_number: PR_NUMBER,
        head_branch: fixture.headBranch,
      },
    ]);
}

export async function createPRLinkForkLaunchFixture(
  apiClient: ApiClient,
  workspaceId: string,
  tmpDir: string,
): Promise<PRLinkForkLaunchFixture> {
  const suffix = randomUUID().replaceAll("-", "").slice(0, 10);
  const upstreamOwner = `e2e-upstream-${suffix}`;
  const upstreamRepository = `pr-link-upstream-${suffix}`;
  const forkOwner = `e2e-fork-${suffix}`;
  const forkRepository = `pr-link-fork-${suffix}`;
  const upstream = createEmptyRemoteRepository(tmpDir, `pr-link-upstream-${suffix}`);
  const fork = createEmptyRemoteRepository(tmpDir, `pr-link-fork-${suffix}`);
  let repositoryId = "";

  try {
    const upstreamGit = new GitHelper(upstream.localPath, upstream.gitEnv);
    upstreamGit.createFile("base.txt", "upstream target main\n");
    upstreamGit.stageAll();
    const targetOID = upstreamGit.commit("upstream target main");
    upstreamGit.exec("git push origin main");

    const forkGit = new GitHelper(fork.localPath, fork.gitEnv);
    forkGit.createFile("base.txt", "fork main\n");
    forkGit.stageAll();
    forkGit.commit("fork main");
    forkGit.exec("git push origin main");
    forkGit.exec(`git checkout -b ${HEAD_BRANCH}`);
    forkGit.createFile("pr.txt", "fork pull request head\n");
    forkGit.stageAll();
    const headOID = forkGit.commit("fork pull request head");
    forkGit.exec(`git push origin ${HEAD_BRANCH}`);
    forkGit.exec(`git push "${upstream.remoteURL}" HEAD:refs/pull/${PR_NUMBER}/head`);

    repositoryId = (
      await apiClient.createRepository(workspaceId, upstream.localPath, "main", {
        name: `${upstreamOwner}/${upstreamRepository}`,
        provider: "github",
        provider_owner: upstreamOwner,
        provider_name: upstreamRepository,
        pull_before_worktree: false,
      })
    ).id;
    await apiClient.mockGitHubAddBranches(upstreamOwner, upstreamRepository, [
      { name: "main" },
      { name: HEAD_BRANCH },
    ]);
    await apiClient.mockGitHubAddPRs([
      {
        number: PR_NUMBER,
        title: "Fork PR task launch",
        state: "open",
        head_branch: HEAD_BRANCH,
        head_sha: headOID,
        head_repo_id: 387901,
        head_repo_owner: forkOwner,
        head_repo_name: forkRepository,
        head_repo_clone_url: `https://github.com/${forkOwner}/${forkRepository}.git`,
        base_branch: "main",
        base_repo_id: 387902,
        base_repo_owner: upstreamOwner,
        base_repo_name: upstreamRepository,
        base_default_branch: "main",
        author_login: "e2e-fork-author",
        repo_owner: upstreamOwner,
        repo_name: upstreamRepository,
      },
    ]);

    return {
      repositoryId,
      repositoryName: `${upstreamOwner}/${upstreamRepository}`,
      upstreamOwner,
      upstreamRepository,
      forkOwner,
      forkRepository,
      prURL: `https://github.com/${upstreamOwner}/${upstreamRepository}/pull/${PR_NUMBER}`,
      headBranch: HEAD_BRANCH,
      headOID,
      targetOID,
      forkCleanup: fork.cleanup,
      upstreamCleanup: upstream.cleanup,
    };
  } catch (error) {
    if (repositoryId) await removeTestRepository(apiClient, repositoryId).catch(() => undefined);
    fork.cleanup();
    upstream.cleanup();
    throw error;
  }
}

export async function cleanupPRLinkForkLaunchFixture(
  apiClient: ApiClient,
  fixture: PRLinkForkLaunchFixture,
  taskId?: string,
): Promise<void> {
  let cleanupError: unknown;
  if (taskId) {
    try {
      await apiClient.deleteTask(taskId);
    } catch (error) {
      cleanupError = error;
    }
  }
  try {
    await removeTestRepository(apiClient, fixture.repositoryId);
  } catch (error) {
    cleanupError ??= error;
  }
  for (const cleanup of [fixture.forkCleanup, fixture.upstreamCleanup]) {
    try {
      cleanup();
    } catch (error) {
      cleanupError ??= error;
    }
  }
  if (cleanupError) throw cleanupError;
}
