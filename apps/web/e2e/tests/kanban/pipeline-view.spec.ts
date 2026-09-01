import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { waitForHttp } from "../../helpers/causal-waits";

test.describe("Pipeline view", () => {
  test.beforeEach(async ({ testPage }) => {
    // The view toggle and multi-select toggle are only rendered on desktop layouts.
    await testPage.setViewportSize({ width: 1280, height: 800 });
  });

  test("shows the linked repository name beneath the task title", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline Repo Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    const repoName = kanban.pipelineTaskRepoName(task.id);
    await expect(repoName).toBeVisible();
    // seedData seeds the repository with the default name from createRepository().
    await expect(repoName).toHaveText("E2E Repo");
  });

  test("does not show a repo name when the task has no linked repository", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline No Repo Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    await expect(kanban.pipelineTask(task.id)).toBeVisible();
    await expect(kanban.pipelineTaskRepoName(task.id)).toHaveCount(0);
  });

  test("multi-select checkbox appears and toolbar reflects the selection", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [t1, t2] = await Promise.all([
      apiClient.createTask(seedData.workspaceId, "Pipeline MS 1", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
      apiClient.createTask(seedData.workspaceId, "Pipeline MS 2", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
    ]);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    // Checkbox is hidden until multi-select mode is enabled.
    await expect(kanban.taskSelectCheckbox(t1.id)).toHaveCount(0);
    await expect(kanban.multiSelectToolbar).not.toBeVisible();

    await kanban.selectPipelineTask(t1.id);
    await expect(kanban.taskSelectCheckbox(t1.id)).toBeVisible();
    await expect(kanban.multiSelectToolbar).toBeVisible();
    await expect(kanban.multiSelectToolbar).toContainText("1 selected");

    // Once multi-select mode is on, every pipeline task exposes a checkbox.
    await kanban.taskSelectCheckbox(t2.id).click();
    await expect(kanban.multiSelectToolbar).toContainText("2 selected");
  });

  test("bulk delete from pipeline view removes the selected tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [t1, t2] = await Promise.all([
      apiClient.createTask(seedData.workspaceId, "Pipeline Delete 1", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
      apiClient.createTask(seedData.workspaceId, "Pipeline Delete 2", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
    ]);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    await kanban.selectPipelineTask(t1.id);
    await kanban.taskSelectCheckbox(t2.id).click();
    await expect(kanban.multiSelectToolbar).toContainText("2 selected");

    await kanban.bulkDeleteButton.click();
    await expect(kanban.bulkDeleteConfirm).toBeVisible();
    await kanban.bulkDeleteConfirm.click();

    await expect(kanban.pipelineTask(t1.id)).toHaveCount(0, { timeout: 10000 });
    await expect(kanban.pipelineTask(t2.id)).toHaveCount(0);
    await expect(kanban.multiSelectToolbar).not.toBeVisible();
  });

  test("title click opens the preview panel when 'Open preview on click' is on", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({ enable_preview_on_click: true });

    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline Preview Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    await kanban
      .pipelineTask(task.id)
      .getByRole("button", { name: "Pipeline Preview Task" })
      .click();

    // Preview panel opens in place (URL carries taskId=), not a full-page navigation to /t/:id.
    // This is a synchronous client-side route update, not a backend round trip,
    // so the default assertion timeout is enough - no hand-picked budget needed.
    await expect(testPage).toHaveURL(/taskId=/);
    await expect(testPage.getByTestId("task-preview-panel")).toBeVisible();

    await apiClient.saveUserSettings({ enable_preview_on_click: false });
  });

  test("the 3-dots actions trigger stays reachable without scrolling on a long pipeline", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 1600, height: 900 });

    // Use a dedicated workflow for the step growth rather than the shared
    // worker-scoped seedData.workflowId: e2eReset keeps that workflow between
    // tests but never trims its steps, so growing it here would leak a 9-step
    // workflow into every later spec in this worker.
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Pipeline Overflow Workflow",
    );
    const targetStepCount = 9;
    const steps: { id: string; title: string }[] = [];
    for (let position = 0; position < targetStepCount; position++) {
      const title = `Step ${position}`;
      const step = await apiClient.createWorkflowStep(workflow.id, title, position);
      steps.push({ id: step.id, title });
    }
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
    });

    // Current step is second-to-last so it has both a prev and a next move
    // target (needed for the F1 regression below, which hovers its right
    // chevron), matching the geometry measured for defect 2 (130px pill +
    // 25px connector each).
    const currentStepIndex = targetStepCount - 2;
    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline Overflow Task", {
      workflow_id: workflow.id,
      workflow_step_id: steps[currentStepIndex].id,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    const scrollport = kanban
      .pipelineTask(task.id)
      .locator(
        'xpath=ancestor::div[contains(concat(" ", normalize-space(@class), " "), " overflow-x-auto ")]',
      )
      .first();

    // The row must genuinely overflow its scrollport, otherwise the reachability
    // assertions below would pass vacuously even if the pipeline stopped overflowing.
    await expect
      .poll(() => scrollport.evaluate((el) => el.scrollWidth > el.clientWidth))
      .toBe(true);

    const trigger = kanban.pipelineTaskActionsTrigger(task.id);
    await expect(trigger).toBeVisible();

    const box = await trigger.boundingBox();
    const viewportSize = testPage.viewportSize();
    expect(box).not.toBeNull();
    expect(viewportSize).not.toBeNull();
    expect(box!.x + box!.width).toBeLessThanOrEqual(viewportSize!.width);

    // Reachable without scrolling: click opens the dropdown menu directly.
    await trigger.click();
    await expect(testPage.getByRole("menuitem", { name: "Delete task" })).toBeVisible();
    await testPage.keyboard.press("Escape");

    // F1 regression: scroll the current pill directly under the pinned actions
    // cluster, hover it so its move chevron renders, then click at the trigger's
    // raw coordinates. A missing z-index on the sticky wrapper lets the chevron
    // paint (and hit-test) above the trigger, so the menu would not open.
    const currentPill = kanban
      .pipelineTask(task.id)
      .getByRole("button", { name: steps[currentStepIndex].title });
    const [pillBox, stickyBox] = [await currentPill.boundingBox(), await trigger.boundingBox()];
    expect(pillBox).not.toBeNull();
    expect(stickyBox).not.toBeNull();
    const overlapPx = 15;
    const scrollDelta = pillBox!.x + pillBox!.width - stickyBox!.x - overlapPx;
    await scrollport.evaluate((el, delta) => {
      el.scrollLeft += delta;
    }, scrollDelta);

    const shiftedPillBox = await currentPill.boundingBox();
    expect(shiftedPillBox).not.toBeNull();
    await testPage.mouse.move(
      shiftedPillBox!.x + shiftedPillBox!.width / 2,
      shiftedPillBox!.y + shiftedPillBox!.height / 2,
    );

    // Confirm the hover actually rendered the move chevron before clicking past
    // it - otherwise this scenario would pass vacuously if the hover never
    // registered, including under the exact regression (missing z-index) it
    // exists to catch.
    const nextStepTitle = steps[currentStepIndex + 1].title;
    await expect(
      kanban.pipelineTask(task.id).getByRole("button", { name: `Move to ${nextStepTitle}` }),
    ).toBeVisible();

    const shiftedTriggerBox = await trigger.boundingBox();
    expect(shiftedTriggerBox).not.toBeNull();
    await testPage.mouse.click(
      shiftedTriggerBox!.x + shiftedTriggerBox!.width / 2,
      shiftedTriggerBox!.y + shiftedTriggerBox!.height / 2,
    );
    await expect(testPage.getByRole("menuitem", { name: "Delete task" })).toBeVisible();
  });

  test("keeps the 3-dots actions trigger reachable on a coarse-pointer tablet", async ({
    tabletTestPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Pipeline Tablet Overflow Workflow",
    );
    const targetStepCount = 9;
    const steps: { id: string; title: string }[] = [];
    for (let position = 0; position < targetStepCount; position++) {
      const title = `Tablet Step ${position}`;
      const step = await apiClient.createWorkflowStep(workflow.id, title, position);
      steps.push({ id: step.id, title });
    }
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
    });

    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline Tablet Task", {
      workflow_id: workflow.id,
      workflow_step_id: steps[targetStepCount - 1].id,
    });
    const kanban = new KanbanPage(tabletTestPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    const row = kanban.pipelineTask(task.id);
    const scrollport = row
      .locator(
        'xpath=ancestor::div[contains(concat(" ", normalize-space(@class), " "), " overflow-x-auto ")]',
      )
      .first();
    await expect
      .poll(() => scrollport.evaluate((element) => element.scrollWidth > element.clientWidth))
      .toBe(true);

    const trigger = kanban.pipelineTaskActionsTrigger(task.id);
    await expect(trigger).toBeVisible();
    const box = await trigger.boundingBox();
    const viewportSize = tabletTestPage.viewportSize();
    expect(box).not.toBeNull();
    expect(viewportSize).not.toBeNull();
    expect(box!.width).toBeGreaterThanOrEqual(44);
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.x + box!.width).toBeLessThanOrEqual(viewportSize!.width);

    const hitTarget = await tabletTestPage.evaluate(
      ({ x, y }) =>
        document
          .elementFromPoint(x, y)
          ?.closest<HTMLElement>('[data-testid^="pipeline-task-actions-trigger-"]')?.dataset
          .testid ?? null,
      { x: box!.x + box!.width / 2, y: box!.y + box!.height / 2 },
    );
    expect(hitTarget).toBe(`pipeline-task-actions-trigger-${task.id}`);

    await trigger.tap();
    await expect(tabletTestPage.getByRole("menuitem", { name: "Delete task" })).toBeVisible();
  });

  test("the current pill's right move chevron stays clickable when it is the last visible pill", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Auto-hide-empty-steps is the common configuration that makes a task's
    // own pill the LAST VISIBLE pill while it still has a next move target:
    // the trailing step has no tasks, so it is hidden from `steps` but
    // remains in `moveTargetSteps`, which is exactly the hidden-destination-
    // disclosure affordance (nextStepHidden). Dedicated workflow, not
    // seedData.workflowId, so the auto-hide setting doesn't leak into other
    // specs sharing this worker's seed data.
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Pipeline Last Visible Pill Workflow",
    );
    const currentStep = await apiClient.createWorkflowStep(workflow.id, "Current", 0);
    await apiClient.createWorkflowStep(workflow.id, "Hidden Next", 1);
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      workflow_ids_with_auto_hide_empty_steps: [workflow.id],
    });

    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline Last Pill Task", {
      workflow_id: workflow.id,
      workflow_step_id: currentStep.id,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    const row = kanban.pipelineTask(task.id);
    const currentPill = row.getByRole("button", { name: "Current" });
    await expect(currentPill).toBeVisible();
    // Hidden Next never renders: it has no tasks and auto-hide is on.
    await expect(row.getByRole("button", { name: "Hidden Next" })).toHaveCount(0);

    await currentPill.hover();
    const moveChevron = row.getByRole("button", { name: "Move to Hidden Next" });
    await expect(moveChevron).toBeVisible();

    // A plain click exercises Playwright's actionability check, which fails
    // if another element (the sticky actions wrapper) intercepts the pointer
    // event at the chevron's location - the exact F6 regression.
    const moved = waitForHttp(testPage, "POST", /\/tasks\/[^/]+\/move$/);
    await moveChevron.click();
    await moved;

    // The move actually landed: the task's current pill is now the
    // previously-hidden step, which is no longer empty and so no longer
    // auto-hidden. The backend round trip is already awaited above, so this
    // needs no hand-picked budget beyond the default.
    await expect(row.getByRole("button", { name: "Hidden Next" })).toBeVisible();
  });
});
