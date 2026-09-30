import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { waitForSessionDone } from "../../helpers/session";
import { failAutomaticRecovery } from "../../helpers/automatic-recovery-owner";

test("Plan keeps automatic recovery reachable in the preview", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const title = "Preview recovery on Plan";
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("fixture session missing");
  const sessionId = task.session_id;
  await waitForSessionDone(apiClient, task.id, sessionId, "preview recovery fixture settled");
  await apiClient.seedTaskSession(task.id, {
    sessionId,
    state: "FAILED",
    completedAt: new Date().toISOString(),
    agentProfileId: seedData.agentProfileId,
    errorMessage: "Connection interrupted",
    metadata: {},
  });
  await apiClient.saveUserSettings({ enable_preview_on_click: true });
  const attempts = await failAutomaticRecovery(testPage, task.id, sessionId);
  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  await kanban.taskCardByTitle(title).click();
  const preview = testPage.getByTestId("task-preview-panel");
  await expect.poll(() => ({ ...attempts })).toMatchObject({ resume: 1, restore: 1 });
  await expect(preview.getByTestId("session-recovery-card")).toBeVisible();
  await preview.getByTestId("preview-plan-tab").click();
  const fallback = preview.getByTestId("session-recovery-error");
  await expect(fallback).toBeVisible();
  await expect(preview.getByTestId("session-recovery-card")).toHaveCount(0);
  await expect(fallback.getByRole("button", { name: "Retry" })).toBeEnabled();
  await prCapture.screenshot("plan-recovery-feedback", {
    caption: "Preview Plan keeps recovery details and retry available.",
  });
  await preview.getByTestId(`preview-session-tab-${sessionId}`).click();
  await expect(preview.getByTestId("session-recovery-card")).toBeVisible();
  await expect(fallback).toHaveCount(0);
});
