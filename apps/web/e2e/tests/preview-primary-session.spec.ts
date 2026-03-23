import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

/**
 * Tests that the kanban preview panel shows the primary session's chat,
 * not the most recently created session.
 */
test.describe("Preview primary session", () => {
  test("preview panel shows primary session content, not latest session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // 1. Create a task with agent — first session becomes primary
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Preview Primary Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Wait for first session to finish
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state);
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // Capture the primary session ID
    const { sessions: sessionsAfterFirst } = await apiClient.listTaskSessions(task.id);
    const primaryId = sessionsAfterFirst[0].id;

    // 3. Navigate to the task session page
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Preview Primary Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 4. Create a second session via the new session dialog
    await session.addPanelButton().click();
    await testPage.getByTestId("new-session-button").click();

    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    await dialog.locator("textarea").fill('e2e:message("secondary-agent-response")');
    await dialog.getByRole("button").filter({ hasText: /Start/ }).click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // 5. Wait for second session to finish
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.filter((s) => DONE_STATES.includes(s.state)).length;
        },
        { timeout: 60_000, message: "Waiting for second session to finish" },
      )
      .toBe(2);

    // Verify backend has primary_session_id pointing to the first session
    const taskData = await apiClient.getTask(task.id);
    expect(taskData.primary_session_id).toBe(primaryId);

    // 6. Enable preview-on-click and navigate back to kanban
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    await kanban.goto();

    // 7. Verify the kanban store has primarySessionId
    const storeData = await testPage.evaluate((taskId) => {
      // Access zustand store from the window (exposed by state-provider)
      type KandevStore = {
        getState: () => { kanban: { tasks: { id: string; primarySessionId?: string | null }[] } };
      };
      const win = window as unknown as { __KANDEV_STORE?: KandevStore };
      const store = win.__KANDEV_STORE;
      if (!store) return { error: "no store" };
      const state = store.getState();
      const kanbanTask = state.kanban.tasks.find((t) => t.id === taskId);
      return {
        primarySessionId: kanbanTask?.primarySessionId ?? null,
        taskFound: !!kanbanTask,
        taskCount: state.kanban.tasks.length,
      };
    }, task.id);
    console.log("Store debug:", JSON.stringify(storeData));

    // 8. Click the task card to open the preview panel.
    // Wait for the "Open full page" button to appear on the card — this button is only
    // rendered when enablePreviewOnClick: true, so its presence confirms the SSR-hydrated
    // settings have been applied to the store before we click.
    const previewCard = kanban.taskCardByTitle("Preview Primary Task");
    await expect(previewCard).toBeVisible({ timeout: 10_000 });
    await expect(previewCard.getByRole("button", { name: "Open full page" })).toBeVisible({
      timeout: 10_000,
    });
    await previewCard.click();

    // Wait for preview panel to appear
    await expect(testPage).toHaveURL(/taskId=/, { timeout: 10_000 });

    // 9. The preview should show the primary session's content
    const previewPanel = testPage.getByTestId("task-preview-panel");
    await expect(previewPanel.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 10. The secondary session's content should NOT be visible
    await expect(
      previewPanel.getByText("secondary-agent-response", { exact: false }),
    ).not.toBeVisible({ timeout: 3_000 });
  });
});
