import { test, expect } from "../../fixtures/test-base";
import {
  createEmptyRemoteRepository,
  openTaskByID,
  remoteRef,
  removeTestRepository,
  taskWorktreeGit,
  waitForTaskWorktree,
} from "../../helpers/empty-remote-repository";

test.describe("Empty remote repository", () => {
  test.setTimeout(180_000);

  test("launches without remote mutation and publishes base before the task branch", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const fixture = createEmptyRemoteRepository(backend.tmpDir, "desktop");
    let repositoryID = "";
    try {
      const repository = await apiClient.createRepository(
        seedData.workspaceId,
        fixture.localPath,
        "main",
        { name: "Empty Remote Desktop" },
      );
      repositoryID = repository.id;
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Empty remote desktop publication",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          executor_profile_id: seedData.worktreeExecutorProfileId,
          repositories: [{ repository_id: repositoryID, base_branch: "main" }],
        },
      );

      const session = await openTaskByID(testPage, task.id);
      await session.waitForChatIdle({ timeout: 60_000 });
      const worktreePath = await waitForTaskWorktree(apiClient, task.id, repositoryID);
      const git = taskWorktreeGit(worktreePath, fixture.gitEnv);

      expect(git.exec("git ls-remote --refs origin").trim()).toBe("");
      const baseCommit = git.exec("git rev-parse refs/heads/main").trim();
      expect(git.exec("git rev-list --count refs/heads/main").trim()).toBe("1");
      expect(git.exec("git ls-tree --name-only refs/heads/main").trim()).toBe("");

      git.createFile("empty-remote-desktop.txt", "published through Changes\n");
      git.stageAll();
      const taskCommit = git.commit("Add empty remote desktop fixture");
      const taskBranch = git.exec("git branch --show-current").trim();

      await session.clickTab("Changes");
      await expect(session.changes).toBeVisible({ timeout: 15_000 });
      await session.expandCommitsSection();
      const pushButton = session.changes.getByTestId("commits-repo-push");
      await expect(pushButton).toBeVisible({ timeout: 15_000 });
      await pushButton.click();

      await expect(
        testPage.getByTestId("toast-message").filter({ hasText: "Push successful" }),
      ).toBeVisible({ timeout: 30_000 });
      await expect.poll(() => remoteRef(git, "main"), { timeout: 30_000 }).toBe(baseCommit);
      await expect.poll(() => remoteRef(git, taskBranch), { timeout: 30_000 }).toBe(taskCommit);
    } finally {
      await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]).catch(() => undefined);
      if (repositoryID) await removeTestRepository(apiClient, repositoryID);
      fixture.cleanup();
    }
  });
});
