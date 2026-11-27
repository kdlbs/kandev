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
  const recovery = preview.getByTestId("session-recovery-card");
  await expect(recovery).toHaveCount(1);
  await expect(recovery).toBeVisible();
  await expect(preview.getByTestId("session-recovery-error")).toHaveCount(0);
  const tabsBox = await preview.getByTestId("preview-plan-tab").boundingBox();
  const recoveryBox = await recovery.boundingBox();
  const panelBox = await preview.boundingBox();
  expect(recoveryBox!.y).toBeGreaterThan(tabsBox!.y + tabsBox!.height);
  expect(recoveryBox!.y + recoveryBox!.height).toBeGreaterThan(panelBox!.y + panelBox!.height - 30);
  await recovery.getByText("Technical details", { exact: true }).click();
  await expect(recovery.locator("pre")).toContainText("Automatic resume transport failed");
  await expect(recovery.locator("pre")).toContainText("Automatic workspace restore failed");
  await prCapture.screenshot("plan-recovery-feedback", {
    caption: "Preview Plan uses the shared recovery card in the lower composer area.",
  });
  const resume = recovery.getByTestId("recovery-resume-button");
  await resume.click();
  await expect.poll(() => attempts.manual).toBe(1);
  await expect(resume).toBeDisabled();
  await expect(recovery.getByTestId("recovery-fresh-button")).toBeDisabled();
  attempts.finishManual();
  await expect(resume).toBeEnabled();
  await expect(recovery.getByTestId("session-recovery-error")).toBeVisible();
  await preview.getByTestId(`preview-session-tab-${sessionId}`).click();
  await expect(preview.getByTestId("session-recovery-card")).toBeVisible();
  await expect(preview.getByTestId("session-recovery-error")).toHaveCount(0);
});
