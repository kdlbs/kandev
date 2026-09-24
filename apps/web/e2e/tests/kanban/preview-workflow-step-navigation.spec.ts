import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import type { ApiClient } from "../../helpers/api-client";
import type { Locator, Page } from "@playwright/test";

const LONG_STEP_NAME = `Awaiting review from the platform team ${"x".repeat(60)}`;

// The header layout budget's ceiling is a two-digit step count on both sides
// of the indicator (e.g. "10/10"); see the system design's Header layout
// section. 100+ step workflows are out of scope (task-02).
const STEPS_FOR_MINIMUM_WIDTH_TEST = 10;

// Brings the shared workflow up to exactly STEPS_FOR_MINIMUM_WIDTH_TEST steps
// with a long-named target step last, so the task lands on a two-digit step
// position. Returns every step id this created, for best-effort cleanup.
async function seedStepsForTenthPosition(
  apiClient: ApiClient,
  workflowId: string,
  existingStepCount: number,
): Promise<{ longStepId: string; createdStepIds: string[] }> {
  const createdStepIds: string[] = [];
  const fillerCount = Math.max(0, STEPS_FOR_MINIMUM_WIDTH_TEST - existingStepCount - 1);
  for (let i = 0; i < fillerCount; i++) {
    const step = await apiClient.createWorkflowStep(
      workflowId,
      `Filler step ${i + 1}`,
      existingStepCount + i,
    );
    createdStepIds.push(step.id);
  }
  const longStep = await apiClient.createWorkflowStep(
    workflowId,
    LONG_STEP_NAME,
    existingStepCount + fillerCount,
  );
  createdStepIds.push(longStep.id);
  return { longStepId: longStep.id, createdStepIds };
}

async function cleanupCreatedSteps(apiClient: ApiClient, stepIds: string[]) {
  for (const stepId of stepIds) {
    await apiClient.deleteWorkflowStep(stepId).catch(() => {});
  }
}

// Confirms the panel rendered inline — no floating backdrop, and the panel
// sits beside the board rather than over it. Both the fine and coarse
// containment tests must prove this before asserting the header budget: the
// budget only binds in the inline layout (system design Risks section).
async function expectInlinePreviewLayout(page: Page, previewPanel: Locator) {
  await expect(page.locator('[aria-label="Close preview"]')).toHaveCount(0);

  const [boardBox, panelBox] = await Promise.all([
    page.getByTestId("kanban-board").boundingBox(),
    previewPanel.boundingBox(),
  ]);
  expect(boardBox).not.toBeNull();
  expect(panelBox).not.toBeNull();
  if (!boardBox || !panelBox) return;
  expect(boardBox.x + boardBox.width).toBeLessThanOrEqual(panelBox.x + 1);
}

// AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.4: the indicator's width is capped
// at half the title-and-indicator group's width (plus 1px of rounding slack).
async function expectIndicatorWithinCap(previewPanel: Locator, triggerBox: { width: number }) {
  const group = previewPanel.locator("div.flex.min-w-0.flex-1.items-center").first();
  const groupBox = await group.boundingBox();
  expect(groupBox).not.toBeNull();
  if (!groupBox) return;
  expect(triggerBox.width).toBeLessThanOrEqual(groupBox.width / 2 + 1);
}

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

// A task's very first page load in a fresh workspace runs a one-time settings
// reconciliation (`use-user-display-settings.ts`'s workspaceId-mismatch effect)
// that issues its own partial settings PATCH. That PATCH is built from a
// settings snapshot captured before this call's PATCH landed, so calling
// `saveUserSettings` before the first `goto()` gets silently clobbered back to
// its default and a card click falls through to full-page navigation instead
// of opening the preview. Letting that first-load reconciliation settle, then
// setting the flag and reloading, avoids the race.
async function enablePreviewOnClick(kanban: KanbanPage, apiClient: ApiClient) {
  await kanban.goto();
  await apiClient.saveUserSettings({ enable_preview_on_click: true });
  await kanban.goto();
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
    await enablePreviewOnClick(kanban, apiClient);
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

  test("opens the touch drawer and moves the task from a tablet preview", async ({
    tabletTestPage,
    apiClient,
    seedData,
  }) => {
    const targetStep = adjacentStep(seedData.steps, seedData.startStepId);
    const task = await apiClient.createTask(seedData.workspaceId, "Preview step nav tablet", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(tabletTestPage);
    await enablePreviewOnClick(kanban, apiClient);

    const card = kanban.taskCardByTitle("Preview step nav tablet");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.tap();

    const previewPanel = tabletTestPage.getByTestId("task-preview-panel");
    await expect(previewPanel).toBeVisible({ timeout: 10_000 });

    const trigger = previewPanel.getByTestId("workflow-stepper-minimal");
    const cue = trigger.getByTestId("workflow-stepper-touch-disclosure-cue");
    await expect(trigger).toBeVisible();
    await expect(cue).toBeVisible();
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox).not.toBeNull();
    if (!triggerBox) return;
    expect(triggerBox.width).toBeGreaterThanOrEqual(44);
    expect(triggerBox.height).toBeGreaterThanOrEqual(44);

    await trigger.tap();

    const disclosure = tabletTestPage.getByTestId("workflow-step-disclosure");
    await expect(disclosure).toBeVisible();
    const moveButton = disclosure.getByTestId(`workflow-step-disclosure-move-${targetStep.id}`);
    await expect(moveButton).toBeVisible();
    const moveButtonBox = await moveButton.boundingBox();
    expect(moveButtonBox).not.toBeNull();
    if (!moveButtonBox) return;
    expect(moveButtonBox.height).toBeGreaterThanOrEqual(44);

    await moveButton.tap();

    await expect
      .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id, {
        timeout: 15_000,
      })
      .toBe(targetStep.id);
    await expect(tabletTestPage.locator('[data-slot="drawer-content"]')).toHaveAttribute(
      "data-state",
      "closed",
    );
    await expect(previewPanel).toBeVisible();
  });

  test("keeps the header a single row at the panel's minimum width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { longStepId: longStep, createdStepIds } = await seedStepsForTenthPosition(
      apiClient,
      seedData.workflowId,
      seedData.steps.length,
    );

    await apiClient.createTask(seedData.workspaceId, "Preview step nav containment", {
      workflow_id: seedData.workflowId,
      workflow_step_id: longStep,
      repository_ids: [seedData.repositoryId],
    });

    await testPage.setViewportSize({ width: 1400, height: 900 });
    // Seed the persisted preview width below the panel's own fine-pointer
    // 320px floor so the inline layout — the binding case per the system
    // design — renders at exactly its minimum, rather than depending on a
    // fragile drag interaction.
    await testPage.addInitScript(() => {
      window.localStorage.setItem("kandev.kanban.preview.width", "1");
    });

    const kanban = new KanbanPage(testPage);
    await enablePreviewOnClick(kanban, apiClient);

    const card = kanban.taskCardByTitle("Preview step nav containment");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();

    const previewPanel = testPage.getByTestId("task-preview-panel");
    await expect(previewPanel).toBeVisible({ timeout: 10_000 });

    const trigger = previewPanel.getByTestId("workflow-stepper-minimal");
    await expect(trigger).toBeVisible();

    await expectInlinePreviewLayout(testPage, previewPanel);

    const title = previewPanel.locator("h2");
    const closeButton = previewPanel.getByRole("button", { name: "Close preview" });
    const maximizeButton = previewPanel.getByRole("button", { name: "Open full page" });
    const copyButton = previewPanel.getByRole("button", { name: "Copy task link" });
    await expect(closeButton).toBeVisible();
    await expect(closeButton).toBeEnabled();
    await expect(maximizeButton).toBeVisible();
    await expect(maximizeButton).toBeEnabled();
    await expect(copyButton).toBeVisible();
    await expect(copyButton).toBeEnabled();

    const [titleBox, triggerBox, closeBox, copyBox, maximizeBox] = await Promise.all([
      title.boundingBox(),
      trigger.boundingBox(),
      closeButton.boundingBox(),
      copyButton.boundingBox(),
      maximizeButton.boundingBox(),
    ]);
    expect(titleBox).not.toBeNull();
    expect(triggerBox).not.toBeNull();
    expect(closeBox).not.toBeNull();
    expect(copyBox).not.toBeNull();
    expect(maximizeBox).not.toBeNull();
    if (!titleBox || !triggerBox || !closeBox || !copyBox || !maximizeBox) return;

    // Single row: every header element shares the same vertical center. Comparing
    // raw tops would fail spuriously — items-center aligns centers, not tops, and
    // the h2 title's text line-box is naturally shorter than the icon buttons.
    const centerY = (box: { y: number; height: number }) => box.y + box.height / 2;
    expect(Math.abs(centerY(titleBox) - centerY(closeBox))).toBeLessThan(4);
    expect(Math.abs(centerY(triggerBox) - centerY(closeBox))).toBeLessThan(4);
    expect(Math.abs(centerY(copyBox) - centerY(closeBox))).toBeLessThan(4);

    // REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003.1: the copy control sits in the
    // panel controls cluster, before the open-full-page control.
    expect(copyBox.x).toBeLessThan(maximizeBox.x);

    // The title floor AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.3 requires, now
    // proven with three panel controls in the budget at this project's fine
    // pointer (system design's Header layout section derives the g<=6px
    // inline bound here; the tighter g<=7px floating bound is not exercised
    // by this inline-layout test).
    expect(titleBox.width).toBeGreaterThanOrEqual(88);

    await expectIndicatorWithinCap(previewPanel, triggerBox);

    // No horizontal scrolling in the header row.
    const headerScrollWidth = await previewPanel
      .locator(".border-b")
      .first()
      .evaluate((el) => el.scrollWidth - el.clientWidth);
    expect(headerScrollWidth).toBeLessThanOrEqual(1);

    await cleanupCreatedSteps(apiClient, createdStepIds);
  });

  test("keeps the header a single row at the coarse-pointer panel minimum", async ({
    coarseDesktopTestPage,
    apiClient,
    seedData,
  }) => {
    const { longStepId: longStep, createdStepIds } = await seedStepsForTenthPosition(
      apiClient,
      seedData.workflowId,
      seedData.steps.length,
    );

    await apiClient.createTask(seedData.workspaceId, "Preview step nav coarse containment", {
      workflow_id: seedData.workflowId,
      workflow_step_id: longStep,
      repository_ids: [seedData.repositoryId],
    });

    // Seed the persisted chosen width below the panel's own 320px fine-pointer
    // floor. At this fixture's coarse pointer, the rendered width floors
    // further to the 380px coarse minimum — the binding case for the coarse
    // budget (system design's Header layout section).
    await coarseDesktopTestPage.addInitScript(() => {
      window.localStorage.setItem("kandev.kanban.preview.width", "1");
    });

    const kanban = new KanbanPage(coarseDesktopTestPage);
    await enablePreviewOnClick(kanban, apiClient);

    const card = kanban.taskCardByTitle("Preview step nav coarse containment");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();

    const previewPanel = coarseDesktopTestPage.getByTestId("task-preview-panel");
    await expect(previewPanel).toBeVisible({ timeout: 10_000 });

    const trigger = previewPanel.getByTestId("workflow-stepper-minimal");
    await expect(trigger).toBeVisible();

    // coarseDesktopTestPage leaves a board container of about 1024px, so the
    // 380px panel stays inline; this is the binding case per the system
    // design's Risks note.
    await expectInlinePreviewLayout(coarseDesktopTestPage, previewPanel);

    const title = previewPanel.locator("h2");
    const closeButton = previewPanel.getByRole("button", { name: "Close preview" });
    const maximizeButton = previewPanel.getByRole("button", { name: "Open full page" });
    const copyButton = previewPanel.getByRole("button", { name: "Copy task link" });
    await expect(closeButton).toBeVisible();
    await expect(closeButton).toBeEnabled();
    await expect(maximizeButton).toBeVisible();
    await expect(maximizeButton).toBeEnabled();
    await expect(copyButton).toBeVisible();
    await expect(copyButton).toBeEnabled();

    const [titleBox, triggerBox, closeBox, copyBox, maximizeBox] = await Promise.all([
      title.boundingBox(),
      trigger.boundingBox(),
      closeButton.boundingBox(),
      copyButton.boundingBox(),
      maximizeButton.boundingBox(),
    ]);
    expect(titleBox).not.toBeNull();
    expect(triggerBox).not.toBeNull();
    expect(closeBox).not.toBeNull();
    expect(copyBox).not.toBeNull();
    expect(maximizeBox).not.toBeNull();
    if (!titleBox || !triggerBox || !closeBox || !copyBox || !maximizeBox) return;

    // Single row: every header element shares the same vertical center. Comparing
    // raw tops would fail spuriously — items-center aligns centers, not tops, and
    // the h2 title's text line-box is naturally shorter than the icon buttons.
    const centerY = (box: { y: number; height: number }) => box.y + box.height / 2;
    expect(Math.abs(centerY(titleBox) - centerY(closeBox))).toBeLessThan(4);
    expect(Math.abs(centerY(triggerBox) - centerY(closeBox))).toBeLessThan(4);
    expect(Math.abs(centerY(copyBox) - centerY(closeBox))).toBeLessThan(4);

    // REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003.1: the copy control sits in the
    // panel controls cluster, before the open-full-page control.
    expect(copyBox.x).toBeLessThan(maximizeBox.x);

    // AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.17: the indicator's hit area is
    // at least 44x44 at a coarse pointer.
    expect(triggerBox.width).toBeGreaterThanOrEqual(44);
    expect(triggerBox.height).toBeGreaterThanOrEqual(44);

    // The title floor AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.3 requires,
    // proven with three panel controls in the budget at this project's coarse
    // pointer (system design's Header layout section derives the g<=7px
    // floating bound; this inline-layout test exercises the tighter g<=6px
    // inline bound at the coarse minimum).
    expect(titleBox.width).toBeGreaterThanOrEqual(88);

    // AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.4: at the coarse minimum this
    // is the binding case — the cap, not the title floor, limits the
    // indicator's width.
    await expectIndicatorWithinCap(previewPanel, triggerBox);

    // No horizontal scrolling in the header row.
    const headerScrollWidth = await previewPanel
      .locator(".border-b")
      .first()
      .evaluate((el) => el.scrollWidth - el.clientWidth);
    expect(headerScrollWidth).toBeLessThanOrEqual(1);

    await cleanupCreatedSteps(apiClient, createdStepIds);
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
    await enablePreviewOnClick(kanban, apiClient);
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
