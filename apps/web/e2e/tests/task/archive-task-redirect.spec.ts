import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("Archive task redirect", () => {
  /**
   * Verifies that archiving a task from the sidebar correctly redirects:
   *   - When other tasks remain: switches to the next available task's session
   *     and the chat panel shows that task's agent conversation.
   *   - When no tasks remain: redirects to the home page.
   */
  test("archiving active task redirects to remaining task, archiving last task redirects home", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // Use distinct descriptions so we can verify the chat panel switches content.
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Archive Task A",
      seedData.agentProfileId,
      {
        description: 'e2e:message("alpha archive response")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Archive Task B",
      seedData.agentProfileId,
      {
        description: 'e2e:message("beta archive response")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // --- Navigate to kanban and wait for both task cards ---
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const cardA = kanban.taskCardByTitle("Archive Task A");
    const cardB = kanban.taskCardByTitle("Archive Task B");
    await expect(cardA).toBeVisible({ timeout: 30_000 });
    await expect(cardB).toBeVisible({ timeout: 30_000 });

    // Click task A to open its session detail page
    await cardA.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Verify we're viewing Task A's session — the chat shows its agent response.
    // Use .last() because the response text also appears inside the user message
    // prompt text (e2e:message("alpha archive response")).
    await expect(session.chat.getByText("alpha archive response").last()).toBeVisible({
      timeout: 30_000,
    });

    // Both tasks should be visible in the sidebar
    await expect(session.taskInSidebar("Archive Task A")).toBeVisible({ timeout: 15_000 });
    await expect(session.taskInSidebar("Archive Task B")).toBeVisible({ timeout: 15_000 });

    // Record the current URL so we can verify it changes after archival
    const urlBeforeArchive = testPage.url();

    // --- Archive the active task (A) — should switch to task B ---
    await session.archiveTaskInSidebar("Archive Task A");

    // Task A should disappear from sidebar
    await expect(session.taskInSidebar("Archive Task A")).not.toBeVisible({ timeout: 15_000 });

    // Task B should still be visible in sidebar
    await expect(session.taskInSidebar("Archive Task B")).toBeVisible({ timeout: 10_000 });

    // The chat panel should now show Task B's session content
    await expect(session.chat.getByText("beta archive response").last()).toBeVisible({
      timeout: 15_000,
    });

    // The URL should have changed to Task B's session (still a /t/ route, but different ID)
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 10_000 });
    expect(testPage.url()).not.toBe(urlBeforeArchive);

    // --- Archive the last remaining task (B) — should redirect home ---
    await session.archiveTaskInSidebar("Archive Task B");

    // Should redirect to the home page
    await expect(testPage).not.toHaveURL(/\/t\//, { timeout: 15_000 });
  });
});
