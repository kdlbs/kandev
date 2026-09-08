import { test, expect } from "../../fixtures/test-base";
import {
  createEmptyRemoteRepository,
  openTaskByID,
  remoteRef,
  removeTestRepository,
  taskWorktreeGit,
  waitForTaskWorktree,
} from "../../helpers/empty-remote-repository";

test.describe("Mobile empty remote repository", () => {
  test.setTimeout(180_000);

  test("publishes base and task branches through the touch Changes path", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const fixture = createEmptyRemoteRepository(backend.tmpDir, "mobile");
    let repositoryID = "";
    try {
      const repository = await apiClient.createRepository(
        seedData.workspaceId,
        fixture.localPath,
        "main",
        { name: "Empty Remote Mobile" },
      );
      repositoryID = repository.id;
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Empty remote mobile publication",
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
      git.createFile("empty-remote-mobile.txt", "published through mobile Changes\n");
      git.stageAll();
      const taskCommit = git.commit("Add empty remote mobile fixture");
      const taskBranch = git.exec("git branch --show-current").trim();

      await testPage.getByRole("button", { name: "Changes" }).tap();
      const changesPanel = testPage.getByTestId("mobile-changes-panel");
      await expect(changesPanel).toBeVisible({ timeout: 15_000 });
      const commitsToggle = changesPanel.getByTestId("commits-section-collapse-toggle");
      await expect(commitsToggle).toBeVisible({ timeout: 15_000 });
      await expect
        .poll(
          async () => {
            if ((await commitsToggle.getAttribute("aria-expanded")) === "true") return true;
            await commitsToggle.tap();
            return (await commitsToggle.getAttribute("aria-expanded")) === "true";
          },
          { timeout: 15_000 },
        )
        .toBe(true);

      const pushButton = changesPanel.getByTestId("commits-repo-push");
      await expect(pushButton).toBeVisible({ timeout: 15_000 });
      await pushButton.tap();

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
