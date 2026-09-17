import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
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
      full_name: "mock-user/mobile-subtask-mixed",
      owner: "mock-user",
      name: "mobile-subtask-mixed",
      private: false,
    },
  ]);
  await apiClient.mockGitHubAddBranches("mock-user", "mobile-subtask-mixed", [{ name: "main" }]);
}

async function createLocalTarget(
  apiClient: ApiClient,
  backendTmpDir: string,
  workspaceId: string,
): Promise<{ id: string }> {
  const repositoryPath = path.join(backendTmpDir, "repos", "mobile-subtask-mixed-local");
  fs.mkdirSync(repositoryPath, { recursive: true });
  const gitEnv = makeGitEnv(backendTmpDir);
  execSync("git init -b develop", { cwd: repositoryPath, env: gitEnv });
  execSync('git commit --allow-empty -m "init"', { cwd: repositoryPath, env: gitEnv });
  return apiClient.createRepository(workspaceId, repositoryPath, "develop", {
    name: "Mobile Subtask Mixed Local",
  });
}

test.describe("New Subtask mixed repository selection on mobile", () => {
  test("edits inherited sources in one repository sheet", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);
    await seedRemoteRepository(apiClient);
    const secondLocal = await createLocalTarget(apiClient, backend.tmpDir, seedData.workspaceId);
    const parentTitle = "Mobile mixed subtask parent";
    const parent = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      parentTitle,
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );

    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto(`/t/${parent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await testPage.getByTestId("mobile-session-menu").tap();
    const taskSheet = testPage.getByRole("dialog", { name: "Tasks" });
    const taskRow = taskSheet.getByTestId("sidebar-task-item").filter({ hasText: parentTitle });
    await taskRow.getByRole("button", { name: "Task actions" }).tap();
    await testPage.getByRole("menuitem", { name: "Create Subtask", exact: true }).tap();

    const dialog = testPage.getByTestId("new-subtask-dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByTestId("subtask-workspace-mode-new").tap();

    await openTaskRepositoryPicker(testPage, { mobile: true, provider: "github" });
    await testPage
      .getByTestId("task-repository-remote-option")
      .filter({ hasText: "mock-user/mobile-subtask-mixed" })
      .dispatchEvent("click");
    await openTaskRepositoryPicker(testPage, { mobile: true, provider: "local" });
    await testPage
      .getByTestId("task-repository-local-option")
      .filter({ hasText: "Mobile Subtask Mixed Local" })
      .dispatchEvent("click");

    await expect(testPage.getByTestId("mobile-repository-sheet-content")).toHaveCount(1);
    await expect(testPage.getByTestId("repo-chip")).toHaveCount(2);
    await expect(testPage.getByTestId("remote-repo-chip")).toHaveCount(1);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile mixed subtask repository sheet");
    await testPage.getByTestId("mobile-repository-done").dispatchEvent("click");

    const childTitle = `Mobile mixed subtask ${Date.now()}`;
    await dialog.getByTestId("subtask-title-input").fill(childTitle);
    await dialog.getByTestId("subtask-prompt-input").fill("/e2e:simple-message");
    const submit = dialog.getByRole("button", { name: "Create Subtask", exact: true });
    await expect(submit).toBeEnabled({ timeout: 15_000 });
    const responsePromise = testPage.waitForResponse(
      (response) =>
        response.url().endsWith("/api/v1/tasks") && response.request().method() === "POST",
    );
    await submit.tap();
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
