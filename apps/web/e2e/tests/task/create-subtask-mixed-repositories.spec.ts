import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { makeGitEnv } from "../../helpers/git-helper";
import { openTaskRepositoryPicker } from "../../helpers/task-repository-picker";
import { useRegularMode } from "../../helpers/regular-mode";
import { SessionPage } from "../../pages/session-page";

useRegularMode();

type TaskRepositoryRow = { repository_id: string; position: number };

async function seedRemoteRepository(apiClient: ApiClient): Promise<void> {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubAddRepos("mock-user", [
    {
      full_name: "mock-user/subtask-mixed",
      owner: "mock-user",
      name: "subtask-mixed",
      private: false,
    },
  ]);
  await apiClient.mockGitHubAddBranches("mock-user", "subtask-mixed", [{ name: "main" }]);
}

async function createLocalTarget(
  apiClient: ApiClient,
  backendTmpDir: string,
  workspaceId: string,
): Promise<{ id: string }> {
  const repositoryPath = path.join(backendTmpDir, "repos", "subtask-mixed-local");
  fs.mkdirSync(repositoryPath, { recursive: true });
  const gitEnv = makeGitEnv(backendTmpDir);
  execSync("git init -b develop", { cwd: repositoryPath, env: gitEnv });
  execSync('git commit --allow-empty -m "init"', { cwd: repositoryPath, env: gitEnv });
  return apiClient.createRepository(workspaceId, repositoryPath, "develop", {
    name: "Subtask Mixed Local",
  });
}

test.describe("New Subtask mixed repository selection", () => {
  test("submits edited inherited sources in order", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);
    await seedRemoteRepository(apiClient);
    const secondLocal = await createLocalTarget(apiClient, backend.tmpDir, seedData.workspaceId);
    const parent = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mixed subtask parent",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );

    await testPage.goto(`/t/${parent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await session.openCreateSubtaskForSidebarTask(parent.title);
    const dialog = testPage.getByTestId("new-subtask-dialog");
    await expect(dialog).toBeVisible();

    // A materialized parent defaults to inherited workspace mode. Switch to a
    // new workspace before editing the child repository selection.
    await dialog.getByTestId("subtask-workspace-mode-new").click();
    await openTaskRepositoryPicker(testPage, { provider: "github" });
    await testPage
      .getByTestId("task-repository-remote-option")
      .filter({ hasText: "mock-user/subtask-mixed" })
      .click();
    await openTaskRepositoryPicker(testPage, { provider: "local" });
    await testPage
      .getByTestId("task-repository-local-option")
      .filter({ hasText: "Subtask Mixed Local" })
      .click();

    await expect(dialog.getByTestId("repo-chip")).toHaveCount(2);
    await expect(dialog.getByTestId("remote-repo-chip")).toHaveCount(1);
    const childTitle = `Mixed subtask ${Date.now()}`;
    await dialog.getByTestId("subtask-title-input").fill(childTitle);
    await dialog.getByTestId("subtask-prompt-input").fill("Create this mixed subtask");
    const submit = dialog.getByRole("button", { name: "Create Subtask", exact: true });
    await expect(submit).toBeEnabled({ timeout: 15_000 });
    const responsePromise = testPage.waitForResponse(
      (response) =>
        response.url().endsWith("/api/v1/tasks") && response.request().method() === "POST",
    );
    await submit.click();
    const response = await responsePromise;
    expect(response.ok()).toBe(true);
    const created = (await response.json()) as { id: string };
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    const task = await apiClient.getTask(created.id);
    const rows = ([...(task.repositories ?? [])] as TaskRepositoryRow[]).sort(
      (first, second) => first.position - second.position,
    );
    expect(rows).toHaveLength(3);
    expect(rows[0].repository_id).toBe(seedData.repositoryId);
    expect(rows[2].repository_id).toBe(secondLocal.id);
  });
});
