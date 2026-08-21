import { restoreSeedRepositoryOrigin, test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("PR watcher missing branch", () => {
  /** A deleted PR branch must use the typed launch-error surface. */
  test("shows one focused recovery panel when PR branch is deleted", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(90_000);

    restoreSeedRepositoryOrigin(seedData);

    // --- Setup mock GitHub ---
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    const prBranch = "feature/already-merged-and-deleted";

    // Mock an open PR whose head branch was deleted on the remote.
    await apiClient.mockGitHubAddPRs([
      {
        number: 999,
        title: "Open feature with deleted branch",
        state: "open",
        head_branch: prBranch,
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);

    // Create a task as the PR watcher would: with a checkout_branch that
    // no longer exists on remote (PR was merged, branch deleted).
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "PR #999: Open feature with deleted branch",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        repositories: [
          {
            repository_id: seedData.repositoryId,
            base_branch: "main",
            checkout_branch: prBranch,
            pr_number: 999,
          },
        ],
        metadata: {
          pr_number: 999,
          pr_branch: prBranch,
          pr_repo: "testorg/testrepo",
          pr_author: "test-user",
        },
      },
    );

    // Associate the PR with the task (as the watcher would)
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 999,
      pr_url: "https://github.com/testorg/testrepo/pull/999",
      pr_title: "Open feature with deleted branch",
      head_branch: prBranch,
      base_branch: "main",
      author_login: "test-user",
      state: "open",
    });

    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          return tasks.find((candidate) => candidate.id === task.id)?.status_summary?.active_error
            ?.category;
        },
        { timeout: 60_000, message: "waiting for the durable PR launch-error projection" },
      )
      .toBe("base_branch_missing");

    // --- Navigate to the task session view ---
    await testPage.goto(`/t/${task.id}`);
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // --- Assert the bounded launch-error card appears once in chat ---
    const chat = session.activeChat();
    const recovery = chat.getByTestId("task-launch-error-entry");
    await expect(recovery).toHaveCount(1, { timeout: 30_000 });
    await expect(recovery).toContainText("The selected base branch is not available.");
    await expect(recovery).not.toContainText(prBranch);
    await expect(recovery.getByTestId("task-launch-archive-button")).toHaveCount(0);
    await expect(recovery.getByTestId("task-launch-delete-button")).toHaveCount(0);

    await testPage.screenshot({
      path: testInfo.outputPath("missing-pr-branch-desktop.png"),
      fullPage: true,
    });

    // Verify the session state via API — should be FAILED
    const { sessions } = await apiClient.listTaskSessions(task.id);
    const failedSession = sessions.find((s) => s.state === "FAILED");
    expect(failedSession).toBeTruthy();

    expect(failedSession!.metadata?.last_agent_error).toBeTruthy();
  });
});
