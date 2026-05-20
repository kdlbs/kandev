import { test, expect } from "../../fixtures/test-base";

test.describe("GitHub PR list task indicator", () => {
  test("renders indicator states, tooltip content, and navigates on click", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    await apiClient.mockGitHubAddPRs([
      {
        number: 100,
        title: "PR with linked task",
        state: "open",
        head_branch: "feat/linked",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
      {
        number: 101,
        title: "PR without task",
        state: "open",
        head_branch: "feat/orphan",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
      {
        number: 102,
        title: "PR with multiple tasks",
        state: "open",
        head_branch: "feat/multi",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);

    const task = await apiClient.createTask(seedData.workspaceId, "Linked task title", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 100,
      pr_url: "https://github.com/testorg/testrepo/pull/100",
      pr_title: "PR with linked task",
      head_branch: "feat/linked",
      base_branch: "main",
      author_login: "test-user",
    });

    // Multi-task case: two distinct tasks linked to the same PR (PR #102).
    // Regression guard for the bug where review tasks created via "+ Task"
    // on the GitHub page never got their github_task_prs row written
    // (synthetic worktree branches never matched the PR head on GitHub).
    const multiA = await apiClient.createTask(seedData.workspaceId, "Multi task A", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    const multiB = await apiClient.createTask(seedData.workspaceId, "Multi task B", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    for (const t of [multiA, multiB]) {
      await apiClient.mockGitHubAssociateTaskPR({
        task_id: t.id,
        owner: "testorg",
        repo: "testrepo",
        pr_number: 102,
        pr_url: "https://github.com/testorg/testrepo/pull/102",
        pr_title: "PR with multiple tasks",
        head_branch: "feat/multi",
        base_branch: "main",
        author_login: "test-user",
      });
    }

    await testPage.goto("/github");

    const singleIndicator = testPage.getByTestId("pr-row-task-indicator-single");
    await expect(singleIndicator).toBeVisible({ timeout: 15_000 });
    await expect(singleIndicator).toContainText("Linked task title");

    const emptyIndicator = testPage.getByTestId("pr-row-task-indicator-empty");
    await expect(emptyIndicator).toBeVisible();
    await expect(emptyIndicator).toHaveText("No task created yet");

    const multiIndicator = testPage.getByTestId("pr-row-task-indicator-multi");
    await expect(multiIndicator).toBeVisible();
    await expect(multiIndicator).toContainText("2");

    await singleIndicator.hover();

    const startStep = seedData.steps.find((s) => s.id === seedData.startStepId);
    const tooltip = testPage.getByRole("tooltip");
    await expect(tooltip).toContainText("Task: Linked task title", { timeout: 5_000 });
    if (startStep?.title) {
      await expect(tooltip).toContainText(`Step: ${startStep.title}`);
    }

    // Click the indicator and confirm a real route change to /t/<task.id>.
    // Regression guard: `replaceTaskUrl` previously only mutated history.state
    // without triggering Next.js navigation, leaving the PR list rendered.
    await singleIndicator.click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}(?:\\?|$)`), {
      timeout: 15_000,
    });
    await expect(testPage.getByTestId("pr-row-task-indicator-single")).toHaveCount(0, {
      timeout: 10_000,
    });
  });
});
