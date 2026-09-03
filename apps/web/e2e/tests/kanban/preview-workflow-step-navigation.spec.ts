import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

const LONG_STEP_NAME = `Awaiting review from the platform team ${"x".repeat(60)}`;

// Column visibility lives on the swimlane header of the workflow it
// configures. Mirrors the same helper in step-visibility-filter.spec.ts.
async function openColumnsMenu(kanban: KanbanPage, workflowId: string) {
  const trigger = kanban.page.getByTestId(`columns-menu-${workflowId}`);
  await expect(async () => {
    if ((await trigger.getAttribute("data-state")) !== "open") {
      await trigger.click();
    }
    await expect(trigger).toHaveAttribute("data-state", "open", { timeout: 1_000 });
  }).toPass({ timeout: 15_000 });
}

async function closeColumnsMenu(kanban: KanbanPage, workflowId: string) {
  const trigger = kanban.page.getByTestId(`columns-menu-${workflowId}`);
  if ((await trigger.getAttribute("data-state")) === "open") {
    await kanban.page.keyboard.press("Escape");
  }
  await expect(trigger).not.toHaveAttribute("data-state", "open");
}

async function hideColumn(kanban: KanbanPage, workflowId: string, stepId: string) {
  await openColumnsMenu(kanban, workflowId);
  await kanban.page.getByTestId(`columns-menu-step-${stepId}`).click();
  await closeColumnsMenu(kanban, workflowId);
}

function adjacentStep(
  steps: Array<{ id: string; position: number }>,
  currentStepId: string,
): { id: string; position: number } {
  const sorted = [...steps].sort((left, right) => left.position - right.position);
  const currentIndex = sorted.findIndex((step) => step.id === currentStepId);
  const target = sorted[currentIndex + 1] ?? sorted[currentIndex - 1];
  if (!target) throw new Error("preview step navigation test requires an adjacent target");
  return target;
}

test.describe("Kanban preview workflow step navigation", () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
  });

  test("moves the task to a step whose board column is hidden", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const targetStep = adjacentStep(seedData.steps, seedData.startStepId);

    const task = await apiClient.createTask(
      seedData.workspaceId,
      "Preview step nav hidden column",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await hideColumn(kanban, seedData.workflowId, targetStep.id);
    await expect(kanban.columnByStepId(targetStep.id)).toBeHidden();

    const card = kanban.taskCardByTitle("Preview step nav hidden column");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();

    const previewPanel = testPage.getByTestId("task-preview-panel");
    await expect(previewPanel).toBeVisible({ timeout: 10_000 });

    const trigger = previewPanel.getByTestId("workflow-stepper-minimal");
    await expect(trigger).toBeVisible();
    await trigger.hover();

    const disclosure = testPage.getByTestId("workflow-step-disclosure");
    await expect(disclosure).toBeVisible();
    // The hidden column still lists in the disclosure — that is the whole point.
    await expect(
      testPage.getByTestId(`workflow-step-disclosure-row-${targetStep.id}`),
    ).toBeVisible();

    await testPage.getByTestId(`workflow-step-disclosure-move-${targetStep.id}`).click();

    await expect
      .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id, { timeout: 15_000 })
      .toBe(targetStep.id);

    // Success closes the disclosure and leaves the preview open on the same task.
    await expect(disclosure).toBeHidden();
    await expect(previewPanel).toBeVisible();
  });

  test("keeps the header a single row at the panel's minimum width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const longStep = await apiClient.createWorkflowStep(
      seedData.workflowId,
      LONG_STEP_NAME,
      seedData.steps.length,
    );

    await apiClient.createTask(seedData.workspaceId, "Preview step nav containment", {
      workflow_id: seedData.workflowId,
      workflow_step_id: longStep.id,
      repository_ids: [seedData.repositoryId],
    });

    await testPage.setViewportSize({ width: 1400, height: 900 });
    // Seed the persisted preview width below the panel's own 300px floor so the
    // inline layout — the binding case per the system design — renders at
    // exactly its minimum, rather than depending on a fragile drag interaction.
    await testPage.addInitScript(() => {
      window.localStorage.setItem("kandev.kanban.preview.width", "1");
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Preview step nav containment");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();

    const previewPanel = testPage.getByTestId("task-preview-panel");
    await expect(previewPanel).toBeVisible({ timeout: 10_000 });

    const trigger = previewPanel.getByTestId("workflow-stepper-minimal");
    await expect(trigger).toBeVisible();

    const title = previewPanel.locator("h2");
    const closeButton = previewPanel.getByRole("button", { name: "Close preview" });
    const maximizeButton = previewPanel.getByRole("button", { name: "Open full page" });
    await expect(closeButton).toBeVisible();
    await expect(closeButton).toBeEnabled();
    await expect(maximizeButton).toBeVisible();
    await expect(maximizeButton).toBeEnabled();

    const [titleBox, triggerBox, closeBox] = await Promise.all([
      title.boundingBox(),
      trigger.boundingBox(),
      closeButton.boundingBox(),
    ]);
    expect(titleBox).not.toBeNull();
    expect(triggerBox).not.toBeNull();
    expect(closeBox).not.toBeNull();
    if (!titleBox || !triggerBox || !closeBox) return;

    // Single row: every header element sits at the same vertical position.
    expect(Math.abs(titleBox.y - closeBox.y)).toBeLessThan(4);
    expect(Math.abs(triggerBox.y - closeBox.y)).toBeLessThan(4);

    // The title floor AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.3 requires.
    expect(titleBox.width).toBeGreaterThanOrEqual(88);

    // No horizontal scrolling in the header row.
    const headerScrollWidth = await previewPanel
      .locator(".border-b")
      .first()
      .evaluate((el) => el.scrollWidth - el.clientWidth);
    expect(headerScrollWidth).toBeLessThanOrEqual(1);
  });

  test("dismisses the disclosure on the first Escape and the preview on the second", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Preview step nav escape", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCardByTitle("Preview step nav escape");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();

    const previewPanel = testPage.getByTestId("task-preview-panel");
    await expect(previewPanel).toBeVisible({ timeout: 10_000 });

    const trigger = previewPanel.getByTestId("workflow-stepper-minimal");
    await trigger.focus();
    const disclosure = testPage.getByTestId("workflow-step-disclosure");
    await expect(disclosure).toBeVisible();

    await testPage.keyboard.press("Escape");
    await expect(disclosure).toBeHidden();
    await expect(previewPanel).toBeVisible();
    await expect(trigger).toBeFocused();

    await testPage.keyboard.press("Escape");
    await expect(previewPanel).toBeHidden();
  });
});
