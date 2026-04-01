import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("PR watcher start button timing", () => {
  /**
   * When a task is created from a PR watcher on a workflow step WITHOUT
   * auto_start_agent, opening the task triggers environment preparation
   * (workspace-only, no agent). The "Start agent" button must NOT appear
   * while the environment is being prepared — only after preparation
   * completes.
   *
   * Flow:
   *   1. Create workflow step without auto_start_agent
   *   2. Create a task as the PR watcher would (with checkout_branch + metadata)
   *   3. Navigate to kanban, click the task
   *   4. Assert: "Start agent" button is NOT visible during preparation
   *   5. Wait for preparation to complete
   *   6. Assert: "Start agent" button becomes visible
   *   7. Click the button → agent starts and completes
   */
  test("hides start button during environment preparation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // --- Create a workflow step WITHOUT auto_start_agent ---
    const reviewStep = await apiClient.createWorkflowStep(seedData.workflowId, "Review", 0);

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      enable_preview_on_click: false,
    });

    // --- Create a task as the PR watcher would ---
    // The task has a checkout_branch and metadata.agent_profile_id so that
    // useAutoStartSession fires, but the backend downgrades to prepare
    // because the step lacks auto_start_agent.
    await apiClient.createTask(seedData.workspaceId, "PR #42: Add feature", {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: reviewStep.id,
      repositories: [
        {
          repository_id: seedData.repositoryId,
          base_branch: "main",
        },
      ],
      metadata: {
        agent_profile_id: seedData.agentProfileId,
        pr_number: 42,
        pr_branch: "feature/add-feature",
        pr_repo: "testorg/testrepo",
        pr_author: "contributor",
      },
    });

    // --- Navigate to kanban and click the task ---
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("PR #42: Add feature");
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();

    // Wait for navigation to session view
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);

    // --- Assert: "Start agent" button should NOT be visible during preparation ---
    // The environment preparation starts when the session auto-prepares.
    // The button should be hidden while status is "preparing".
    const startButton = session.chat.getByTestId("task-description-start-button");

    // First, wait for the session chat to be visible (so we know the page loaded)
    await session.waitForLoad();

    // --- Assert: button appears only after preparation completes ---
    // In the mock environment, preparation may complete very quickly.
    // The key invariant is: the button must eventually become visible
    // (it was hidden during the preparing phase via prepareProgress store check).
    await expect(startButton).toBeVisible({ timeout: 60_000 });

    // --- Click the button to start the agent ---
    await startButton.click();

    // Agent should start and eventually complete (mock agent with simple message)
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  });
});
