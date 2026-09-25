import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { SessionPage } from "../../pages/session-page";

const COMPACT_TASK_TITLE = `Compact workflow navigation ${"W".repeat(90)}`;
const WIDTH_REGRESSION_WORKFLOW_NAME = "Compact disclosure width workflow";
const WIDTH_REGRESSION_WORKFLOW_YAML = `version: 1
type: kandev_workflow
workflows:
  - name: ${WIDTH_REGRESSION_WORKFLOW_NAME}
    steps:
      - name: Backlog
        position: 0
        color: bg-neutral-400
        allow_manual_move: true
        events:
          on_turn_start:
            - type: move_to_next
      - name: Implementation
        position: 1
        color: bg-blue-500
        allow_manual_move: true
        events:
          on_enter:
            - type: auto_start_agent
          on_turn_complete:
            - type: move_to_next
      - name: Review
        position: 2
        color: bg-yellow-500
        is_start_step: true
        allow_manual_move: true`;

function adjacentStep(
  steps: Array<{ id: string; position: number }>,
  currentStepId: string,
): { id: string; position: number } {
  const sorted = [...steps].sort((left, right) => left.position - right.position);
  const currentIndex = sorted.findIndex((step) => step.id === currentStepId);
  const target = sorted[currentIndex + 1] ?? sorted[currentIndex - 1];
  if (!target) throw new Error("compact workflow step test requires an adjacent target");
  return target;
}

test.describe("Compact task topbar workflow stepper", () => {
  test("compact disclosure keeps ordinary step names readable", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const imported = await apiClient.importWorkflows(
      seedData.workspaceId,
      WIDTH_REGRESSION_WORKFLOW_YAML,
    );
    expect(imported.created).toContain(WIDTH_REGRESSION_WORKFLOW_NAME);

    const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
    const workflow = workflows.find(({ name }) => name === WIDTH_REGRESSION_WORKFLOW_NAME);
    if (!workflow) throw new Error("compact disclosure width workflow was not imported");

    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const backlog = steps.find(({ name }) => name === "Backlog");
    const implementation = steps.find(({ name }) => name === "Implementation");
    const review = steps.find(({ name }) => name === "Review");
    if (!backlog || !implementation || !review) {
      throw new Error("compact disclosure width workflow is missing its regression steps");
    }

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
    });
    const task = await apiClient.seedTask(seedData.workspaceId, COMPACT_TASK_TITLE, {
      workflow_id: workflow.id,
      workflow_step_id: review.id,
      state: "REVIEW",
    });

    await testPage.setViewportSize({ width: 900, height: 800 });
    await testPage.goto(`/t/${task.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    for (const locale of ["en", "pt-pt"] as const) {
      if (locale === "pt-pt") {
        await testPage.evaluate((nextLocale) => {
          document.cookie = `kandev_locale=${nextLocale}; path=/; max-age=31536000; SameSite=Lax`;
        }, locale);
        await testPage.reload();
        await session.waitForLoad();
      }
      await expect(testPage.locator("html")).toHaveAttribute("lang", locale);

      const trigger = testPage.getByTestId("workflow-stepper-minimal");
      await expect(trigger).toBeVisible();
      await trigger.hover();

      const disclosure = testPage.getByTestId("workflow-step-disclosure");
      const disclosureSurface = testPage
        .locator('[role="dialog"]:visible')
        .filter({ has: disclosure });
      await expect(disclosure).toBeVisible();
      await expect(disclosureSurface).toHaveCount(1);

      await waitForFiniteAnimations(disclosureSurface.first());
      const surfaceBox = await disclosureSurface.boundingBox();
      expect(surfaceBox).not.toBeNull();
      if (!surfaceBox) return;
      const viewport = await testPage.evaluate(() => ({
        height: window.innerHeight,
        width: window.innerWidth,
      }));
      expect(surfaceBox.x).toBeGreaterThanOrEqual(0);
      expect(surfaceBox.y).toBeGreaterThanOrEqual(0);
      expect(surfaceBox.x + surfaceBox.width).toBeLessThanOrEqual(viewport.width);
      expect(surfaceBox.y + surfaceBox.height).toBeLessThanOrEqual(viewport.height);

      for (const { step, capabilityCount } of [
        { step: backlog, capabilityCount: 1 },
        { step: implementation, capabilityCount: 2 },
      ]) {
        const row = disclosure.getByTestId(`workflow-step-disclosure-row-${step.id}`);
        const label = row.locator("span.truncate").first();
        await expect(label).toHaveText(step.name);
        const labelWidths = await label.evaluate((element) => ({
          clientWidth: element.clientWidth,
          scrollWidth: element.scrollWidth,
        }));
        expect(
          labelWidths.scrollWidth,
          `${locale} ${step.name} label should not be truncated`,
        ).toBeLessThanOrEqual(labelWidths.clientWidth + 1);

        await expect(row.locator('[data-slot="tooltip-trigger"]')).toHaveCount(capabilityCount);
        const actionButtons = row.locator(
          '[data-testid^="workflow-step-disclosure-options-"], [data-testid^="workflow-step-disclosure-move-"]',
        );
        await expect(actionButtons).toHaveCount(2);
        for (let index = 0; index < (await actionButtons.count()); index += 1) {
          const actionBox = await actionButtons.nth(index).boundingBox();
          expect(actionBox).not.toBeNull();
          if (!actionBox) return;
          expect(actionBox.x).toBeGreaterThanOrEqual(surfaceBox.x);
          expect(actionBox.y).toBeGreaterThanOrEqual(surfaceBox.y);
          expect(actionBox.x + actionBox.width).toBeLessThanOrEqual(
            surfaceBox.x + surfaceBox.width,
          );
          expect(actionBox.y + actionBox.height).toBeLessThanOrEqual(
            surfaceBox.y + surfaceBox.height,
          );
        }
      }

      await prCapture.screenshot(`compact-disclosure-${locale}`, {
        caption: `Compact workflow disclosure with ${locale} labels and actions`,
      });
      await testPage.keyboard.press("Escape");
      await expect(disclosureSurface).toBeHidden();
    }
  });

  test("opens ordered steps on hover and moves the task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.seedTask(seedData.workspaceId, COMPACT_TASK_TITLE, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    // A worker can refresh the workflow while another test is completing its
    // setup. Read the workflow after creating the task so the disclosure
    // assertion follows the current backend step set instead of stale worker
    // fixture data.
    const { steps: currentSteps } = await apiClient.listWorkflowSteps(seedData.workflowId);
    const targetStep = adjacentStep(currentSteps, seedData.startStepId);

    await testPage.setViewportSize({ width: 900, height: 800 });
    await testPage.goto(`/t/${task.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const trigger = testPage.getByTestId("workflow-stepper-minimal");
    await expect(trigger).toBeVisible();
    await expect(trigger).toHaveAttribute("aria-haspopup", "dialog");
    await trigger.hover();

    const disclosure = testPage.getByTestId("workflow-step-disclosure");
    const disclosureSurface = testPage.getByRole("dialog", { name: "Move to" });
    await expect(disclosure).toBeVisible();
    await expect(disclosureSurface).toBeVisible();
    await testPage.mouse.move(0, 0);
    await expect(disclosureSurface).toBeHidden();
    await trigger.focus();
    await expect(disclosureSurface).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expect(disclosureSurface).toBeHidden();
    await expect(trigger).toBeFocused();
    await testPage.keyboard.press("Tab");
    await trigger.focus();
    await expect(disclosureSurface).toBeVisible();
    await expect(disclosure.locator('[data-testid^="workflow-step-disclosure-row-"]')).toHaveCount(
      currentSteps.length,
    );

    const moveButton = testPage.getByTestId(`workflow-step-disclosure-move-${targetStep.id}`);
    await expect(moveButton).toBeVisible();
    const moveButtonBox = await moveButton.boundingBox();
    expect(moveButtonBox).not.toBeNull();
    if (!moveButtonBox) return;
    expect(moveButtonBox.height).toBeLessThan(40);

    let moveButtonFocused = false;
    for (let tabCount = 0; tabCount < currentSteps.length + 2; tabCount += 1) {
      if (await moveButton.evaluate((element) => element === document.activeElement)) {
        moveButtonFocused = true;
        break;
      }
      await testPage.keyboard.press("Tab");
    }
    expect(moveButtonFocused).toBe(true);
    await testPage.keyboard.press("Enter");

    await expect
      .poll(async () => (await apiClient.getTask(task.task_id)).workflow_step_id, {
        timeout: 15_000,
      })
      .toBe(targetStep.id);
  });

  test("opens the same steps in a contained tablet touch drawer", async ({
    tabletTestPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.seedTask(seedData.workspaceId, COMPACT_TASK_TITLE, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const targetStep = adjacentStep(seedData.steps, seedData.startStepId);

    await tabletTestPage.goto(`/t/${task.task_id}`);
    const session = new SessionPage(tabletTestPage);
    await session.waitForLoad();

    const trigger = tabletTestPage.getByTestId("workflow-stepper-minimal");
    await expect(trigger).toBeVisible();
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox).not.toBeNull();
    if (!triggerBox) return;
    expect(triggerBox.height).toBeGreaterThanOrEqual(44);
    await trigger.tap();

    const drawer = tabletTestPage.getByRole("dialog", { name: "Move to" });
    await expect(drawer).toBeVisible();
    await waitForFiniteAnimations(drawer);
    const drawerBox = await drawer.boundingBox();
    expect(drawerBox).not.toBeNull();
    if (!drawerBox) return;
    const viewport = await tabletTestPage.evaluate(() => ({
      height: innerHeight,
      width: innerWidth,
    }));
    expect(drawerBox.x).toBeGreaterThanOrEqual(0);
    expect(drawerBox.y).toBeGreaterThanOrEqual(0);
    expect(drawerBox.x + drawerBox.width).toBeLessThanOrEqual(viewport.width);
    expect(drawerBox.y + drawerBox.height).toBeLessThanOrEqual(viewport.height);
    expect(
      await tabletTestPage.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(await tabletTestPage.evaluate(() => window.innerWidth));

    const targetRow = tabletTestPage.getByTestId(`workflow-step-disclosure-row-${targetStep.id}`);
    const targetRowBox = await targetRow.boundingBox();
    expect(targetRowBox).not.toBeNull();
    if (!targetRowBox) return;
    expect(targetRowBox.height).toBeGreaterThanOrEqual(44);

    const targetButton = tabletTestPage.getByTestId(
      `workflow-step-disclosure-move-${targetStep.id}`,
    );
    const targetButtonBox = await targetButton.boundingBox();
    expect(targetButtonBox).not.toBeNull();
    if (!targetButtonBox) return;
    expect(targetButtonBox.height).toBeGreaterThanOrEqual(44);
    await targetButton.tap();
    await expect
      .poll(async () => (await apiClient.getTask(task.task_id)).workflow_step_id, {
        timeout: 15_000,
      })
      .toBe(targetStep.id);
  });
});
