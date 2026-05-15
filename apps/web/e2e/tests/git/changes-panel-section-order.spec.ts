import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";

// ---------------------------------------------------------------------------
// Git helper (same as git-changes-panel.spec.ts)
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

  modifyFile(name: string, content: string) {
    this.createFile(name, content);
  }

  stageFile(name: string) {
    this.exec(`git add "${name}"`);
  }

  stageAll() {
    this.exec("git add -A");
  }

  commit(message: string): string {
    this.exec(`git commit -m "${message}"`);
    return this.exec("git rev-parse HEAD").trim();
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

async function createStandardProfile(apiClient: ApiClient, name: string) {
  const { agents } = await apiClient.listAgents();
  const agentId = agents[0]?.id;
  if (!agentId) throw new Error(`No agent available for profile "${name}"`);
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

/**
 * Seed a workflow: Inbox -> Working (auto_start, on_turn_complete -> Done) -> Done
 * and set it as the active workflow filter.
 */
async function seedWorkflow(apiClient: ApiClient, workspaceId: string) {
  const workflow = await apiClient.createWorkflow(workspaceId, "Section Order Workflow");
  const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
  const workingStep = await apiClient.createWorkflowStep(workflow.id, "Working", 1);
  const doneStep = await apiClient.createWorkflowStep(workflow.id, "Done", 2);

  await apiClient.updateWorkflowStep(workingStep.id, {
    prompt: 'e2e:message("done")\n{{task_prompt}}',
    events: {
      on_enter: [{ type: "auto_start_agent" }],
      on_turn_complete: [{ type: "move_to_step", config: { step_id: doneStep.id } }],
    },
  });

  await apiClient.saveUserSettings({
    workspace_id: workspaceId,
    workflow_filter_id: workflow.id,
    enable_preview_on_click: false,
  });

  return { workflow, inboxStep, workingStep, doneStep };
}

/**
 * Seed mock GitHub PR data: one PR with two files and one commit.
 */
async function seedMockPR(apiClient: ApiClient) {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");

  await apiClient.mockGitHubAddPRs([
    {
      number: 300,
      title: "Add feature",
      state: "open",
      head_branch: "feat/order-test",
      base_branch: "main",
      author_login: "test-user",
      repo_owner: "testorg",
      repo_name: "testrepo",
      additions: 40,
      deletions: 5,
    },
  ]);

  await apiClient.mockGitHubAddPRFiles("testorg", "testrepo", 300, [
    { filename: "feature.ts", status: "added", additions: 30, deletions: 0 },
    { filename: "index.ts", status: "modified", additions: 10, deletions: 5 },
  ]);

  await apiClient.mockGitHubAddPRCommits("testorg", "testrepo", 300, [
    {
      sha: "ddd1111222233334444555566667777aaaabbbb",
      message: "add feature module",
      author_login: "test-user",
      author_date: "2026-03-01T12:00:00Z",
    },
  ]);
}

/**
 * Assert that element A appears before element B in the DOM (top-to-bottom).
 * Uses bounding boxes: A's top should be less than B's top.
 */
async function expectAbove(a: ReturnType<Page["locator"]>, b: ReturnType<Page["locator"]>) {
  const boxA = await a.boundingBox();
  const boxB = await b.boundingBox();
  expect(boxA).toBeTruthy();
  expect(boxB).toBeTruthy();
  expect(boxA!.y).toBeLessThan(boxB!.y);
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("Changes panel section ordering", () => {
  /**
   * When a task has both local changes (unstaged) and PR files,
   * the unstaged section should appear above the PR files section.
   */
  test("local changes appear above PR files when both exist", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    const { workflow, inboxStep, workingStep, doneStep } = await seedWorkflow(
      apiClient,
      seedData.workspaceId,
    );
    await seedMockPR(apiClient);

    const profile = await createStandardProfile(apiClient, "Order Test Profile");
    const task = await apiClient.createTask(seedData.workspaceId, "Order Test Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: profile.id,
      repository_ids: [seedData.repositoryId],
    });

    // Navigate to kanban before moving so WS is subscribed
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await apiClient.moveTask(task.id, workflow.id, workingStep.id);
    await expect(kanban.taskCardInColumn("Order Test Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // Associate PR with task
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 300,
      pr_url: "https://github.com/testorg/testrepo/pull/300",
      pr_title: "Add feature",
      head_branch: "feat/order-test",
      base_branch: "main",
      author_login: "test-user",
      additions: 40,
      deletions: 5,
    });

    // Create a local unstaged change
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

    git.createFile("local-change.txt", "initial");
    git.stageAll();
    git.commit("base commit");
    git.modifyFile("local-change.txt", "modified");

    // Open task and switch to Changes tab
    await kanban.taskCardInColumn("Order Test Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.clickTab("Changes");

    // Wait for both sections to appear
    const unstaged = testPage.getByTestId("unstaged-files-section");
    const prFiles = testPage.getByTestId("pr-files-section");
    await expect(unstaged).toBeVisible({ timeout: 15_000 });
    await expect(prFiles).toBeVisible({ timeout: 15_000 });

    // Unstaged should appear above PR files
    await expectAbove(unstaged, prFiles);

    // Staged section should NOT be visible (no staged files)
    await expect(testPage.getByTestId("staged-files-section")).not.toBeVisible();

    // Clean up
    git.exec("git checkout -- .");
  });

  /**
   * When a task has only PR files and no local changes,
   * the PR files section should appear at the top of the timeline.
   */
  test("PR files appear at top when no local changes exist", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { workflow, inboxStep, workingStep, doneStep } = await seedWorkflow(
      apiClient,
      seedData.workspaceId,
    );
    await seedMockPR(apiClient);

    const profile = await createStandardProfile(apiClient, "PR Only Profile");
    const task = await apiClient.createTask(seedData.workspaceId, "PR Only Task", {
      workflow_id: workflow.id,
      workflow_step_id: inboxStep.id,
      agent_profile_id: profile.id,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await apiClient.moveTask(task.id, workflow.id, workingStep.id);
    await expect(kanban.taskCardInColumn("PR Only Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 300,
      pr_url: "https://github.com/testorg/testrepo/pull/300",
      pr_title: "Add feature",
      head_branch: "feat/order-test",
      base_branch: "main",
      author_login: "test-user",
      additions: 40,
      deletions: 5,
    });

    // Open task (no local changes)
    await kanban.taskCardInColumn("PR Only Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.clickTab("Changes");

    // PR files should be visible at the top
    const prFiles = testPage.getByTestId("pr-files-section");
    const commits = testPage.getByTestId("commits-section");
    await expect(prFiles).toBeVisible({ timeout: 15_000 });
    await expect(commits).toBeVisible({ timeout: 15_000 });

    // PR files should appear above commits
    await expectAbove(prFiles, commits);

    // No unstaged or staged sections
    await expect(testPage.getByTestId("unstaged-files-section")).not.toBeVisible();
    await expect(testPage.getByTestId("staged-files-section")).not.toBeVisible();
  });

  /**
   * When there are staged files, the staged section should be visible.
   * When there are only unstaged files, the staged section should be hidden.
   */
  test("staged section only visible when files are staged", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    const profile = await createStandardProfile(apiClient, "Staged Visibility Profile");
    await apiClient.createTaskWithAgent(seedData.workspaceId, "Staged Test Task", profile.id, {
      description: "Test staged visibility",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Staged Test Task");

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

    // Create an unstaged file (not staged)
    git.createFile("staged-test.txt", "initial");
    git.stageAll();
    git.commit("base for staged test");
    git.modifyFile("staged-test.txt", "modified");

    await session.clickTab("Changes");

    // Unstaged should be visible, staged should NOT
    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("staged-files-section")).not.toBeVisible();

    // Now stage the file
    git.stageFile("staged-test.txt");

    // Staged section should appear
    await expect(testPage.getByTestId("staged-files-section")).toBeVisible({ timeout: 15_000 });

    // Clean up
    git.exec("git reset HEAD -- staged-test.txt");
    git.exec("git checkout -- .");
  });
});

// ---------------------------------------------------------------------------
// Desktop PR Overlap Test Helpers
// ---------------------------------------------------------------------------

type SeedData = {
  workspaceId: string;
  repositoryId: string;
};

type Backend = {
  tmpDir: string;
};

async function seedDesktopOverlapPR(apiClient: ApiClient, workspaceId: string, seedData: SeedData) {
  const { workflow, inboxStep, workingStep, doneStep } = await seedWorkflow(apiClient, workspaceId);

  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  await apiClient.mockGitHubAddPRs([
    {
      number: 301,
      title: "Desktop overlap PR diff test",
      state: "open",
      head_branch: "feat/desktop-overlap",
      base_branch: "main",
      author_login: "test-user",
      repo_owner: "testorg",
      repo_name: "testrepo",
      additions: 2,
      deletions: 0,
    },
  ]);
  await apiClient.mockGitHubAddPRFiles("testorg", "testrepo", 301, [
    {
      filename: "overlap-desktop.txt",
      status: "added",
      additions: 2,
      deletions: 0,
      patch: "@@ -0,0 +1,2 @@\n+DESKTOP_PR_OVERLAP_MARKER_A\n+DESKTOP_PR_OVERLAP_MARKER_B",
    },
  ]);

  const profile = await createStandardProfile(apiClient, "Desktop Overlap Profile");
  const task = await apiClient.createTask(workspaceId, "Desktop PR Overlap Task", {
    workflow_id: workflow.id,
    workflow_step_id: inboxStep.id,
    agent_profile_id: profile.id,
    repository_ids: [seedData.repositoryId],
  });

  return { workflow, inboxStep, workingStep, doneStep, task, profile };
}

function createLocalOverlapFile(backend: Backend): { git: GitHelper; repoDir: string } {
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
  git.createFile("overlap-desktop.txt", "local change LOCAL_CHANGE_MARKER");
  return { git, repoDir };
}

async function assertPRDiffContains(testPage: Page, marker: string) {
  await testPage.waitForFunction(
    (text: string) => {
      for (const c of document.querySelectorAll("diffs-container")) {
        if (c.shadowRoot?.textContent?.includes(text)) return true;
      }
      return false;
    },
    marker,
    { timeout: 30_000 },
  );
}

// ---------------------------------------------------------------------------
// Desktop PR Overlap Regression Test
// ---------------------------------------------------------------------------

test.describe("PR diff regression", () => {
  /**
   * Regression: clicking a PR file row should open the PR diff even when the
   * same path also has local (uncommitted) changes. Previously, allFiles
   * deduplication caused the PR entry to be shadowed, and the diff panel
   * showed "No changes" instead of the PR content.
   */
  test("clicking PR file row shows PR diff when same file has local changes", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // Seed PR with overlap file
    const { workflow, workingStep, doneStep, task } = await seedDesktopOverlapPR(
      apiClient,
      seedData.workspaceId,
      seedData,
    );

    // Create task and move to working step
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await apiClient.moveTask(task.id, workflow.id, workingStep.id);
    await expect(kanban.taskCardInColumn("Desktop PR Overlap Task", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // Associate PR with task
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 301,
      pr_url: "https://github.com/testorg/testrepo/pull/301",
      pr_title: "Desktop overlap PR diff test",
      head_branch: "feat/desktop-overlap",
      base_branch: "main",
      author_login: "test-user",
      additions: 2,
      deletions: 0,
    });

    // Create local overlap file
    const { git } = createLocalOverlapFile(backend);

    // Open task and verify PR diff appears
    await kanban.taskCardInColumn("Desktop PR Overlap Task", doneStep.id).click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.clickTab("Changes");

    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("pr-files-section")).toBeVisible({ timeout: 15_000 });

    await session.expandPRChangesSection();
    await session.prFilesSection().getByText("overlap-desktop.txt").click();

    // Assert PR diff content appears (not "No changes")
    await assertPRDiffContains(testPage, "DESKTOP_PR_OVERLAP_MARKER_A");

    git.exec("git checkout -- .");
    git.exec("git clean -fd");
  });
});
