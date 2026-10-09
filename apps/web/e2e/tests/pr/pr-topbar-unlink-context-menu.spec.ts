import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { makeGitEnv } from "../../helpers/git-helper";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";

const OWNER = "acme";
const PR_NUMBER = 42;

async function seedTask(
  apiClient: ApiClient,
  workspaceId: string,
  agentProfileId: string,
  repositoryIds: string[],
) {
  const title = "Topbar PR unlink context menu";
  const workflow = await apiClient.createWorkflow(workspaceId, `${title} Workflow`);
  const done = await apiClient.createWorkflowStep(workflow.id, "Done", 0);
  await apiClient.saveUserSettings({ workspace_id: workspaceId, workflow_filter_id: workflow.id });
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");

  const task = await apiClient.createTask(workspaceId, title, {
    workflow_id: workflow.id,
    workflow_step_id: done.id,
    agent_profile_id: agentProfileId,
    repository_ids: repositoryIds,
  });
  const now = new Date().toISOString();
  await apiClient.seedTaskSession(task.id, {
    state: "COMPLETED",
    agentProfileId,
    repositoryId: repositoryIds[0],
    startedAt: now,
    completedAt: now,
  });

  return { taskId: task.id, title, workflowStepId: done.id };
}

async function associatePR(
  apiClient: ApiClient,
  workspaceId: string,
  taskId: string,
  repositoryId: string,
  repo: string,
) {
  await apiClient.mockGitHubAssociateTaskPR({
    workspace_id: workspaceId,
    task_id: taskId,
    repository_id: repositoryId,
    owner: OWNER,
    repo,
    pr_number: PR_NUMBER,
    pr_url: `https://github.com/${OWNER}/${repo}/pull/${PR_NUMBER}`,
    pr_title: `${repo} PR ${PR_NUMBER}`,
    head_branch: `feat/${repo}`,
    base_branch: "main",
    author_login: "test-user",
    state: "open",
  });
}

test.describe("PR top-bar unlink context menu", () => {
  test("unlinks one exact association and preserves its sibling after reload", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const secondaryRepoDir = path.join(backend.tmpDir, "repos", "pr-unlink-api");
    fs.mkdirSync(secondaryRepoDir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: secondaryRepoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: secondaryRepoDir, env: gitEnv });
    const secondaryRepo = await apiClient.createRepository(
      seedData.workspaceId,
      secondaryRepoDir,
      "main",
      { name: "PR unlink API repository", pull_before_worktree: false },
    );
    const seed = await seedTask(apiClient, seedData.workspaceId, seedData.agentProfileId, [
      seedData.repositoryId,
      secondaryRepo.id,
    ]);
    await associatePR(apiClient, seedData.workspaceId, seed.taskId, seedData.repositoryId, "web");
    await associatePR(apiClient, seedData.workspaceId, seed.taskId, secondaryRepo.id, "api");
    await expect
      .poll(async () =>
        (await apiClient.listTaskPRs(seed.taskId)).map((pr) => `${pr.repo}#${pr.pr_number}`).sort(),
      )
      .toEqual(["api#42", "web#42"]);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardInColumn(seed.title, seed.workflowStepId)).toBeVisible();
    await kanban.taskCardInColumn(seed.title, seed.workflowStepId).click();
    await expect(testPage).toHaveURL(/\/[st]\//);
    let session = new SessionPage(testPage);
    await session.waitForLoad();
    const trigger = session.prTopbarButton();
    await expect(trigger).toHaveAttribute("data-pr-count", "2");

    await trigger.hover();
    await expect(testPage.getByTestId("pr-topbar-popover")).toBeVisible();
    await trigger.click({ button: "right" });
    const edit = testPage.getByRole("menuitem", { name: "Edit" });
    await expect(edit).toBeVisible();
    await expect(testPage.getByTestId("pr-topbar-popover")).toHaveCount(0);
    await edit.focus();
    await testPage.keyboard.press("ArrowRight");
    const unlinkChoice = testPage.getByRole("menuitem", { name: "Remove acme/api #42 from task" });
    await expect(unlinkChoice).toBeVisible();
    await waitForFiniteAnimations(testPage.getByRole("menu").last());
    await prCapture.screenshot("desktop-topbar-pr-unlink-menu", {
      caption: "Desktop top-bar PR unlink menu",
    });
    await unlinkChoice.click();

    await expect
      .poll(async () =>
        (await apiClient.listTaskPRs(seed.taskId)).map((pr) => `${pr.repo}#${pr.pr_number}`).sort(),
      )
      .toEqual(["web#42"]);
    await expect(session.prTopbarButton()).toHaveAttribute("data-pr-number", "42");

    await testPage.reload();
    session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.prTopbarButton()).toHaveAttribute("data-pr-number", "42");
    await expect
      .poll(async () =>
        (await apiClient.listTaskPRs(seed.taskId)).map((pr) => `${pr.repo}#${pr.pr_number}`).sort(),
      )
      .toEqual(["web#42"]);
  });
});
