import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForLatestSessionDone } from "../../helpers/session";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("mobile: workflow move preview", () => {
  test("shows the same prediction and details inside the touch drawer", async ({
    tabletTestPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Move Preview Task",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await waitForLatestSessionDone(
      apiClient,
      task.id,
      1,
      "Mobile move preview task must finish before navigation",
      30_000,
    );
    const sortedSteps = [...seedData.steps].sort((left, right) => left.position - right.position);
    const currentIndex = sortedSteps.findIndex((step) => step.id === seedData.startStepId);
    const targetStep = sortedSteps[currentIndex + 1] ?? sortedSteps[currentIndex - 1];
    if (!targetStep) throw new Error("mobile move preview requires an adjacent target");

    const kanban = new KanbanPage(tabletTestPage);
    await kanban.goto();
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    await kanban.goto();
    const card = kanban.taskCardByTitle("Mobile Move Preview Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.tap();

    const previewPanel = tabletTestPage.getByTestId("task-preview-panel");
    await expect(previewPanel).toBeVisible({ timeout: 10_000 });
    const trigger = previewPanel.getByTestId("workflow-stepper-minimal");
    await expect(trigger).toBeVisible({ timeout: 15_000 });
    await trigger.tap();

    const drawer = tabletTestPage.locator('[data-slot="drawer-content"][data-state="open"]');
    await expect(drawer).toBeVisible();
    const row = drawer.getByTestId(`workflow-step-disclosure-row-${targetStep.id}`);
    await expect(row).toBeVisible();
    const preview = row.getByTestId("workflow-move-preview");
    await expect(preview).toBeVisible({ timeout: 15_000 });
    await expect(preview).toContainText("Reuse current session");
    await expect(preview).toContainText("mock-fast");

    const detailsToggle = preview.getByTestId("workflow-move-preview-details-toggle");
    const toggleBox = await detailsToggle.boundingBox();
    expect(toggleBox).not.toBeNull();
    if (!toggleBox) return;
    expect(toggleBox.height).toBeGreaterThanOrEqual(44);
    await detailsToggle.tap();
    await expect(row.getByTestId("workflow-move-preview-details")).toBeVisible();

    for (const testId of [
      "workflow-step-disclosure-options-" + targetStep.id,
      "workflow-step-disclosure-move-" + targetStep.id,
    ]) {
      const box = await drawer.getByTestId(testId).boundingBox();
      expect(box).not.toBeNull();
      if (!box) return;
      expect(box.height).toBeGreaterThanOrEqual(44);
    }
    await assertNoDocumentHorizontalOverflow(tabletTestPage);

    await tabletTestPage.keyboard.press("Escape");
    await expect(
      tabletTestPage.locator('[data-slot="drawer-content"][data-state="open"]'),
    ).toHaveCount(0);
    await expect(trigger).toBeFocused();
  });
});
