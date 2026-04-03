import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

class GitHelper {
  constructor(
    private repoDir: string,
    private env: NodeJS.ProcessEnv,
  ) {}

  exec(cmd: string): string {
    const lockPath = path.join(this.repoDir, ".git", "index.lock");
    for (let attempt = 0; attempt < 3; attempt++) {
      if (fs.existsSync(lockPath)) fs.unlinkSync(lockPath);
      try {
        return execSync(cmd, { cwd: this.repoDir, env: this.env, encoding: "utf8" });
      } catch (err) {
        const msg = (err as Error).message ?? "";
        if (msg.includes("index.lock") && attempt < 2) {
          execSync("sleep 0.2");
          continue;
        }
        throw err;
      }
    }
    throw new Error(`git exec failed after 3 attempts: ${cmd}`);
  }

  createFile(name: string, content: string) {
    fs.writeFileSync(path.join(this.repoDir, name), content);
  }

  /** Set up a bare remote so the local branch can track ahead/behind. */
  setupRemote(tmpDir: string): void {
    const bareDir = path.join(tmpDir, "repos", "e2e-repo-bare.git");
    execSync(`git clone --bare "${this.repoDir}" "${bareDir}"`, { env: this.env });
    this.exec(`git remote add origin "${bareDir}"`);
    this.exec("git fetch origin");
    this.exec("git branch --set-upstream-to=origin/main main");
  }

  commitFile(name: string, content: string, message: string): void {
    this.createFile(name, content);
    this.exec("git add -A");
    this.exec(`git commit -m "${message}"`);
  }
}

async function createStandardProfile(apiClient: ApiClient, name: string) {
  const { agents } = await apiClient.listAgents();
  const agentId = agents[0]?.id;
  if (!agentId) {
    throw new Error(`E2E setup failed: no agent available for profile "${name}"`);
  }
  return apiClient.createAgentProfile(agentId, name, {
    model: "mock-fast",
    auto_approve: true,
    cli_passthrough: false,
  });
}

async function openTaskSession(page: Page, title: string): Promise<SessionPage> {
  const kanban = new KanbanPage(page);
  await kanban.goto();

  const card = kanban.taskCardByTitle(title);
  await expect(card).toBeVisible({ timeout: 15_000 });
  await card.click();
  await expect(page).toHaveURL(/\/t\//, { timeout: 15_000 });

  const session = new SessionPage(page);
  await session.waitForLoad();
  return session;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("VCS split button context", () => {
  test("shows Push when PR exists and commits are ahead", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    const profile = await createStandardProfile(apiClient, "VCS Push Profile");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "VCS Push Button Test",
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const session = await openTaskSession(testPage, "VCS Push Button Test");
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Set up git with a remote so we can get "ahead" status
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);
    git.setupRemote(backend.tmpDir);

    // Make a local commit → now 1 ahead of origin/main
    git.commitFile("push-test.txt", "push content", "feat: push test commit");

    // Before associating a PR, the button should show "Create PR"
    // (ahead > 0, no PR)
    await expect(session.vcsPrimaryButton("pr")).toBeVisible({ timeout: 30_000 });

    // Now associate an open PR with the task
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 50,
      pr_url: "https://github.com/testorg/testrepo/pull/50",
      pr_title: "Feature branch",
      head_branch: "main",
      base_branch: "develop",
      author_login: "test-user",
      additions: 10,
      deletions: 2,
    });

    // Wait for the PR topbar button to appear (confirms PR is in store)
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 30_000 });

    // The VCS button should now show "Push" instead of "Create PR"
    await expect(session.vcsPrimaryButton("push")).toBeVisible({ timeout: 15_000 });
    await expect(session.vcsPrimaryButton("push")).toContainText("Push");
  });

  test("shows Create PR when ahead but no PR exists", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    const profile = await createStandardProfile(apiClient, "VCS PR Profile");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "VCS PR Button Test", profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "VCS PR Button Test");
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Set up git with a remote
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Only set up remote if not already configured
    try {
      git.exec("git remote get-url origin");
    } catch {
      git.setupRemote(backend.tmpDir);
    }

    // Push current state to origin, then make a new commit to be ahead
    git.exec("git push origin main");
    git.commitFile("pr-test.txt", "pr content", "feat: pr test commit");

    // No PR associated → should show "Create PR"
    await expect(session.vcsPrimaryButton("pr")).toBeVisible({ timeout: 30_000 });
    await expect(session.vcsPrimaryButton("pr")).toContainText("Create PR");
  });

  test("shows Commit when there are uncommitted files even with open PR", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    const profile = await createStandardProfile(apiClient, "VCS Commit Profile");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "VCS Commit Priority Test",
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const session = await openTaskSession(testPage, "VCS Commit Priority Test");
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Associate an open PR
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 55,
      pr_url: "https://github.com/testorg/testrepo/pull/55",
      pr_title: "Commit Priority PR",
      head_branch: "main",
      base_branch: "develop",
      author_login: "test-user",
      additions: 5,
      deletions: 1,
    });

    await expect(session.prTopbarButton()).toBeVisible({ timeout: 30_000 });

    // Create an uncommitted file
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    });
    git.createFile("uncommitted.txt", "uncommitted content");

    // Even with open PR, uncommitted files → "Commit" takes priority
    await expect(session.vcsPrimaryButton("commit")).toBeVisible({ timeout: 30_000 });
    await expect(session.vcsPrimaryButton("commit")).toContainText("Commit");
  });

  test("defaults to Commit when no changes and no PR", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const profile = await createStandardProfile(apiClient, "VCS Default Profile");

    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "VCS Default Button Test",
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const session = await openTaskSession(testPage, "VCS Default Button Test");
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // No changes, no PR → default is "Commit"
    await expect(session.vcsPrimaryButton("commit")).toBeVisible({ timeout: 15_000 });
    await expect(session.vcsPrimaryButton("commit")).toContainText("Commit");
  });
});
