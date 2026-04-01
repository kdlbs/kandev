import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("PR watcher start button timing", () => {
  /**
   * When a task is created from a PR watcher on a workflow step WITHOUT
   * auto_start_agent, opening the task triggers environment preparation
   * (workspace-only, no agent). The "Start agent" button should appear
   * after preparation completes and clicking it should start the agent.
   *
   * Flow:
   *   1. Create workflow step without auto_start_agent
   *   2. Create a task as the PR watcher would (with metadata.agent_profile_id)
   *   3. Navigate to task — auto-start fires, backend downgrades to prepare
   *   4. Wait for CREATED session to exist (env preparing)
   *   5. Reload page so SSR picks up the session and renders it
   *   6. Assert: "Start agent" button becomes visible
   *   7. Click the button — agent starts and completes
   */
  test("start button appears after environment preparation and starts agent", async ({
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
    const task = await apiClient.createTask(seedData.workspaceId, "PR #42: Add feature", {
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

    // --- Wait for auto-start to create a CREATED session ---
    // useAutoStartSession fires with autoStart=true, backend downgrades to
    // prepare (step lacks auto_start_agent). Session stays in CREATED state.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.length > 0;
        },
        { timeout: 30_000, message: "Waiting for session to be created" },
      )
      .toBe(true);

    // --- Reload so SSR picks up the session and renders the chat ---
    // After auto-start creates the CREATED session, the frontend doesn't
    // automatically set it as the active session. Reloading ensures SSR
    // resolves the session and the chat panel renders with the task description.
    await testPage.reload();

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // --- Assert: "Start agent" button becomes visible after prep completes ---
    const startButton = session.chat.getByTestId("task-description-start-button");
    await expect(startButton).toBeVisible({ timeout: 60_000 });

    // --- Click the button to start the agent ---
    await startButton.click();

    // Agent should start and eventually complete (mock agent with simple message)
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  });
});
