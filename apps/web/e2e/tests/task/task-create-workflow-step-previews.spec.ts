import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { expectTaskDescription } from "../../pages/task-description-editor";
import {
  cleanupWorkflowStepPreviewScenario,
  seedWorkflowStepPreviewScenario,
} from "./workflow-step-previews-helpers";

useRegularMode();

function workflowStepsResponse(page: import("@playwright/test").Page, workflowId: string) {
  return page.waitForResponse((response) =>
    response.url().includes("/api/v1/workflows/" + workflowId + "/workflow/steps"),
  );
}

async function expectStepsInOrder(
  page: import("@playwright/test").Page,
  workflowId: string,
  stepNames: string[],
) {
  const group = page.getByTestId("workflow-option-steps-" + workflowId);
  await expect(group).toBeVisible();
  const text = (await group.textContent()) ?? "";
  let previousPosition = -1;
  for (const name of stepNames) {
    const position = text.indexOf(name);
    expect(position).toBeGreaterThan(previousPosition);
    previousPosition = position;
  }
}

// @covers AC-TASKS-CREATE-WORKFLOW-STEPS-001.1 AC-TASKS-CREATE-WORKFLOW-STEPS-001.2 AC-TASKS-CREATE-WORKFLOW-STEPS-001.3 AC-TASKS-CREATE-WORKFLOW-STEPS-001.5 AC-TASKS-CREATE-WORKFLOW-STEPS-001.6
test("loads every workflow preview from a task page and retries one failed row", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const scenario = await seedWorkflowStepPreviewScenario(apiClient, seedData.workspaceId);
  let reviewAttempts = 0;
  await testPage.route(
    "**/api/v1/workflows/" + scenario.review.id + "/workflow/steps",
    async (route) => {
      reviewAttempts += 1;
      if (reviewAttempts === 1) {
        await route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ error: "private test server detail" }),
        });
        return;
      }
      await route.continue();
    },
  );

  try {
    await testPage.goto("/t/" + scenario.taskId);
    await expect(testPage).toHaveURL(new RegExp("/t/" + scenario.taskId + "$"));
    await testPage.getByTestId("create-task-button").first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const title = dialog.getByTestId("task-title-input");
    const description = dialog.getByTestId("task-description-input");
    await title.fill("Preserve the task draft");
    await description.fill("Compare the workflow steps before choosing.");

    await testPage.setViewportSize({ width: 1280, height: 900 });
    const workflowSelector = dialog.getByTestId("workflow-selector-trigger");
    await workflowSelector.scrollIntoViewIfNeeded();
    const featureResponse = workflowStepsResponse(testPage, scenario.feature.id);
    const reviewResponse = workflowStepsResponse(testPage, scenario.review.id);
    await workflowSelector.click();
    expect((await featureResponse).ok()).toBe(true);
    expect((await reviewResponse).status()).toBe(503);

    await expectStepsInOrder(testPage, scenario.kanban.id, scenario.kanban.stepNames);
    await expectStepsInOrder(testPage, scenario.feature.id, scenario.feature.stepNames);
    await expect(testPage.getByTestId("workflow-option-" + scenario.review.id)).toContainText(
      "Failed to load workflow steps",
    );
    await expect(workflowSelector).toContainText("Preview Kanban");
    await expect(testPage.getByText("private test server detail")).toHaveCount(0);

    const retry = testPage.getByTestId("workflow-preview-retry-" + scenario.review.id);
    await expect(retry).toBeVisible();
    await testPage.setViewportSize({ width: 640, height: 720 });
    const optionList = testPage.getByTestId("workflow-selector-option-list");
    await optionList.evaluate((element) => {
      element.scrollTop = 0;
    });
    const featureStepGroup = testPage.getByTestId("workflow-option-steps-" + scenario.feature.id);
    const unbrokenStep = featureStepGroup.getByText(scenario.feature.unbrokenStepName, {
      exact: true,
    });
    const [stepBox, stepGroupBox] = await Promise.all([
      unbrokenStep.boundingBox(),
      featureStepGroup.boundingBox(),
    ]);
    if (!stepBox || !stepGroupBox) throw new Error("Unbroken workflow step has no layout box");
    expect(stepBox.x).toBeGreaterThanOrEqual(stepGroupBox.x - 1);
    expect(stepBox.x + stepBox.width).toBeLessThanOrEqual(stepGroupBox.x + stepGroupBox.width + 1);

    await retry.scrollIntoViewIfNeeded();
    const narrowRetryBox = await retry.boundingBox();
    if (!narrowRetryBox) throw new Error("Workflow retry has no narrow layout box");
    expect(await retry.evaluate((element) => getComputedStyle(element).height)).toBe("44px");
    expect(narrowRetryBox.height).toBeGreaterThanOrEqual(44);
    expect(narrowRetryBox.height).toBeLessThan(48);

    await testPage.setViewportSize({ width: 1280, height: 900 });
    await retry.scrollIntoViewIfNeeded();
    const wideRetryBox = await retry.boundingBox();
    if (!wideRetryBox) throw new Error("Workflow retry has no wide layout box");
    expect(wideRetryBox.height).toBe(28);

    await testPage.setViewportSize({ width: 640, height: 720 });
    await retry.scrollIntoViewIfNeeded();
    const retryResponse = workflowStepsResponse(testPage, scenario.review.id);
    await retry.click();
    expect((await retryResponse).ok()).toBe(true);
    await expectStepsInOrder(testPage, scenario.review.id, scenario.review.stepNames);
    await expect(workflowSelector).toHaveAttribute("aria-expanded", "true");
    await expect(title).toHaveValue("Preserve the task draft");
    await expectTaskDescription(description, "Compare the workflow steps before choosing.");

    const popover = testPage.getByTestId("workflow-selector-popover");
    const box = await popover.boundingBox();
    if (!box) throw new Error("Workflow selector has no layout box");
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(640);
    const documentWidth = await testPage.evaluate(() => ({
      scroll: document.documentElement.scrollWidth,
      client: document.documentElement.clientWidth,
    }));
    expect(documentWidth.scroll).toBeLessThanOrEqual(documentWidth.client);

    const featureOption = testPage.getByTestId("workflow-option-select-" + scenario.feature.id);
    await featureOption.click();
    await expect(workflowSelector).toContainText("Feature Plan");
    await expect(dialog.getByTestId("task-create-launch-step")).toHaveText("Analysis");
    await expect(title).toHaveValue("Preserve the task draft");
    await expectTaskDescription(description, "Compare the workflow steps before choosing.");
  } finally {
    await cleanupWorkflowStepPreviewScenario(apiClient, scenario);
  }
});
