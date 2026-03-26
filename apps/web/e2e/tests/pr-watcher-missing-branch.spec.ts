import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

test.describe("PR watcher missing branch", () => {
  /**
   * When a task is created from a PR watcher and the PR's head branch has
   * already been deleted (merged PR), clicking the task should show a
   * user-friendly error in the chat with options to archive or delete.
   *
   * Setup:
   *   - Create task with checkout_branch pointing to a non-existent branch
   *     (simulates what the PR watcher creates when a PR is found)
   *   - Associate a mock PR with the task
   *   - Navigate to the task — frontend auto-prepares the session
   *   - Environment preparation fails because the branch doesn't exist
   *   - Assert the guidance message appears in chat
   */
  test("shows guidance message when PR branch is deleted", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // --- Setup mock GitHub ---
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    const prBranch = "feature/already-merged-and-deleted";

    // Mock the PR that was already merged (branch deleted on remote)
    await apiClient.mockGitHubAddPRs([
      {
        number: 999,
        title: "Already merged feature",
        state: "closed",
        head_branch: prBranch,
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);

    // Create a task as the PR watcher would: with a checkout_branch that
    // no longer exists on remote (PR was merged, branch deleted).
    const task = await apiClient.createTask(
      seedData.workspaceId,
      "PR #999: Already merged feature",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [
          {
            repository_id: seedData.repositoryId,
            base_branch: "main",
            checkout_branch: prBranch,
          },
        ],
        metadata: {
          agent_profile_id: seedData.agentProfileId,
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
      pr_title: "Already merged feature",
      head_branch: prBranch,
      base_branch: "main",
      author_login: "test-user",
      state: "closed",
    });

    // --- Navigate to the kanban and click the task ---
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("PR #999: Already merged feature");
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();

    // Wait for navigation to session view
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);

    // --- Assert the guidance message appears in chat ---
    // The MissingBranchMessage component renders with archive/delete action buttons.
    await expect(session.chat.getByText("no longer exists", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    // Verify the action buttons are present
    await expect(session.chat.getByTestId("missing-branch-archive-button")).toBeVisible({
      timeout: 5_000,
    });
    await expect(session.chat.getByTestId("missing-branch-delete-button")).toBeVisible({
      timeout: 5_000,
    });

    // Verify the session state via API — should be FAILED
    const { sessions } = await apiClient.listTaskSessions(task.id);
    const failedSession = sessions.find((s) => s.state === "FAILED");
    expect(failedSession).toBeTruthy();

    // Verify the guidance message metadata via API
    const { messages } = await apiClient.listSessionMessages(failedSession!.id);
    const guidanceMsg = messages.find(
      (m: Record<string, unknown>) =>
        (m.metadata as Record<string, unknown>)?.failure_kind === "missing_pr_branch",
    );
    expect(guidanceMsg).toBeTruthy();
    expect((guidanceMsg as Record<string, unknown>).content).toContain(prBranch);
  });
});
