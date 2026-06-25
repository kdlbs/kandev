import { randomUUID } from "node:crypto";
import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

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

    // 1. Create a task, then seed two completed sessions directly. This spec
    // exercises kanban preview session selection, not mock-agent execution.
    const task = await apiClient.createTask(seedData.workspaceId, "Preview Primary Task", {
      description: "Preview primary session",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const now = Date.now();
    const primaryId = randomUUID();
    await apiClient.seedTaskSession(task.id, {
      sessionId: primaryId,
      state: "COMPLETED",
      startedAt: new Date(now).toISOString(),
      completedAt: new Date(now + 1_000).toISOString(),
    });
    await apiClient.seedSessionMessage(primaryId, {
      type: "message",
      content: "simple mock response from primary session",
    });
    await apiClient.setPrimarySession(primaryId);

    const secondary = await apiClient.seedTaskSession(task.id, {
      sessionId: randomUUID(),
      state: "COMPLETED",
      startedAt: new Date(now + 2_000).toISOString(),
      completedAt: new Date(now + 3_000).toISOString(),
    });
    await apiClient.seedSessionMessage(secondary.session_id, {
      type: "message",
      content: "secondary-agent-response",
    });

    // 4. Wait for both sessions to be visible to the API.
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

    // 5. Enable preview-on-click and navigate to kanban
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // 6. Verify the workflow snapshot query cache has primary_session_id
    await expect
      .poll(
        async () =>
          testPage.evaluate((taskId) => {
            type WorkflowSnapshot = {
              tasks?: Array<{ id: string; primary_session_id?: string | null }>;
            };
            type QueryClient = {
              getQueryCache: () => {
                findAll: () => Array<{
                  queryKey: readonly unknown[];
                  state: { data: unknown };
                }>;
              };
            };
            const win = window as unknown as { __KANDEV_E2E_QUERY_CLIENT__?: QueryClient };
            const queryClient = win.__KANDEV_E2E_QUERY_CLIENT__;
            if (!queryClient) return null;
            const snapshots = queryClient
              .getQueryCache()
              .findAll()
              .filter(
                (query) => query.queryKey[0] === "workflows" && query.queryKey[2] === "snapshot",
              )
              .map((query) => query.state.data as WorkflowSnapshot);
            const tasks = snapshots.flatMap((snapshot) => snapshot.tasks ?? []);
            return tasks.find((item) => item.id === taskId)?.primary_session_id ?? null;
          }, task.id),
        { timeout: 10_000, message: "workflow snapshot query includes primary session" },
      )
      .toBe(primaryId);

    const storeData = await testPage.evaluate((taskId) => {
      type WorkflowSnapshot = {
        tasks?: Array<{ id: string; primary_session_id?: string | null }>;
      };
      type QueryClient = {
        getQueryCache: () => {
          findAll: () => Array<{
            queryKey: readonly unknown[];
            state: { data: unknown };
          }>;
        };
      };
      const win = window as unknown as { __KANDEV_E2E_QUERY_CLIENT__?: QueryClient };
      const queryClient = win.__KANDEV_E2E_QUERY_CLIENT__;
      if (!queryClient) return { error: "no query client" };
      const snapshots = queryClient
        .getQueryCache()
        .findAll()
        .filter((query) => query.queryKey[0] === "workflows" && query.queryKey[2] === "snapshot")
        .map((query) => query.state.data as WorkflowSnapshot);
      const tasks = snapshots.flatMap((snapshot) => snapshot.tasks ?? []);
      const task = tasks.find((item) => item.id === taskId);
      return {
        primarySessionId: task?.primary_session_id ?? null,
        taskFound: !!task,
        taskCount: tasks.length,
      };
    }, task.id);
    expect(storeData).toEqual({
      primarySessionId: primaryId,
      taskFound: true,
      taskCount: expect.any(Number),
    });

    // 7. Click the task card to open the preview panel.
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

    // 8. The preview should show the primary session's content
    const previewPanel = testPage.getByTestId("task-preview-panel");
    await expect(previewPanel.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 9. The secondary session's content should NOT be visible
    await expect(
      previewPanel.getByText("secondary-agent-response", { exact: false }),
    ).not.toBeVisible({ timeout: 3_000 });
  });
});
