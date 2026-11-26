import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

test("kanban preview reveals terminal helper history on demand", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Preview session history", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const primary = await apiClient.seedTaskSession(task.id, {
    state: "WAITING_FOR_INPUT",
    agentProfileId: seedData.agentProfileId,
    sessionId: `preview-history-primary-${task.id}`,
  });
  const helper = await apiClient.seedTaskSession(task.id, {
    state: "COMPLETED",
    agentProfileId: seedData.agentProfileId,
    sessionId: `preview-history-helper-${task.id}`,
    completedAt: "2026-09-01T00:00:00Z",
  });
  await apiClient.setPrimarySession(primary.session_id);
  await apiClient.saveUserSettings({ enable_preview_on_click: true });

  const kanban = new KanbanPage(testPage);
  await kanban.goto(seedData.workflowId);
  const card = kanban.taskCardByTitle("Preview session history");
  await expect(card).toBeVisible();
  await card.click();

  const preview = testPage.getByTestId("task-preview-panel");
  const helperTab = preview.getByTestId(`preview-session-tab-${helper.session_id}`);
  const primaryTab = preview.getByTestId(`preview-session-tab-${primary.session_id}`);
  const historyToggle = preview.getByTestId("preview-session-history-toggle");
  await expect(primaryTab).toBeVisible();
  await expect(helperTab).toHaveCount(0);
  await expect(historyToggle).toBeVisible();
  await expect(testPage.getByText("Kandev update available")).toHaveCount(0, {
    timeout: 10_000,
  });
  await prCapture.screenshot("session-history-hidden-preview", {
    caption: "Kanban preview hides the terminal helper by default",
  });

  await historyToggle.click();
  await expect(helperTab).toBeVisible();
  await prCapture.screenshot("session-history-visible-preview", {
    caption: "Kanban preview reveals the terminal helper after Show history",
  });
});
