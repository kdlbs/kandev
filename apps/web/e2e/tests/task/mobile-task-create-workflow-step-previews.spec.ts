import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { expectTaskDescription } from "../../pages/task-description-editor";
import {
  armWorkflowStepPreviewResponses,
  cleanupWorkflowStepPreviewScenario,
  expectWorkflowOptionVisibleAndHitTestable,
  expectWorkflowPickerOverflow,
  expectWorkflowStepPreviewsLoaded,
  expectStepsInOrder,
  getWorkflowPickerEndOptions,
  seedWorkflowStepPreviewScenario,
  touchWorkflowOptionListToBoundary,
  workflowStepsResponse,
} from "./workflow-step-previews-helpers";

useRegularMode();

// @covers AC-TASKS-CREATE-WORKFLOW-STEPS-001.1 AC-TASKS-CREATE-WORKFLOW-STEPS-001.2 AC-TASKS-CREATE-WORKFLOW-STEPS-001.3 AC-TASKS-CREATE-WORKFLOW-STEPS-001.6
test("keeps long workflow previews contained and touch-usable on a phone", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const scenario = await seedWorkflowStepPreviewScenario(apiClient, seedData.workspaceId, {
    extraWorkflowCount: 4,
  });
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
    await testPage.setViewportSize({ width: 390, height: 640 });
    expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
    await testPage.goto("/t/" + scenario.taskId);
    await expect(testPage).toHaveURL(new RegExp("/t/" + scenario.taskId + "$"));
    await testPage.getByTestId("mobile-task-picker-trigger").tap();
    await testPage
      .getByRole("dialog", { name: "Tasks", exact: true })
      .getByRole("button", { name: "New", exact: true })
      .tap();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const title = dialog.getByTestId("task-title-input");
    const description = dialog.getByTestId("task-description-input");
    await title.fill("Keep the phone draft");
    await description.fill("Check long workflow previews on a phone.");

    const workflowSelector = dialog.getByTestId("workflow-selector-trigger");
    await workflowSelector.scrollIntoViewIfNeeded();
    const popover = testPage.getByTestId("workflow-selector-popover");
    const kanbanResponse = workflowStepsResponse(testPage, scenario.kanban.id);
    const featureResponse = workflowStepsResponse(testPage, scenario.feature.id);
    const reviewResponse = workflowStepsResponse(testPage, scenario.review.id);
    const extraResponses = scenario.extraWorkflows.map(({ id }) =>
      workflowStepsResponse(testPage, id),
    );
    await workflowSelector.tap();
    await expect(popover).toBeVisible();
    expect((await kanbanResponse).ok()).toBe(true);
    expect((await featureResponse).ok()).toBe(true);
    expect((await reviewResponse).status()).toBe(503);
    expect((await Promise.all(extraResponses)).every((response) => response.ok())).toBe(true);
    await expect(popover).toBeVisible();

    await expectStepsInOrder(testPage, scenario.kanban.id, scenario.kanban.stepNames);
    await expectStepsInOrder(testPage, scenario.feature.id, scenario.feature.stepNames);
    await Promise.all(
      scenario.extraWorkflows.map(({ id, stepName }) =>
        expectStepsInOrder(testPage, id, [stepName]),
      ),
    );
    await expect(testPage.getByTestId("workflow-option-" + scenario.review.id)).toContainText(
      "Failed to load workflow steps",
    );
    await expect(testPage.getByTestId("workflow-selector-popover").getByRole("status")).toHaveCount(
      1,
    );
    await expect(testPage.getByTestId("workflow-preview-status-announcement")).toContainText(
      "Contributor Review: Failed to load workflow steps",
    );

    const popoverBox = await popover.boundingBox();
    if (!popoverBox) throw new Error("Workflow selector has no layout box");
    expect(popoverBox.x).toBeGreaterThanOrEqual(0);
    expect(popoverBox.y).toBeGreaterThanOrEqual(0);
    expect(popoverBox.x + popoverBox.width).toBeLessThanOrEqual(390);
    expect(popoverBox.y + popoverBox.height).toBeLessThanOrEqual(640);

    const optionList = testPage.getByTestId("workflow-selector-option-list");
    const scrollState = await optionList.evaluate((element) => ({
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
      overflowY: getComputedStyle(element).overflowY,
    }));
    expect(scrollState.overflowY).toBe("auto");
    expect(scrollState.scrollHeight).toBeGreaterThan(scrollState.clientHeight);
    await optionList.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    expect(await optionList.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
    await optionList.evaluate((element) => {
      element.scrollTop = 0;
    });

    const retry = testPage.getByTestId("workflow-preview-retry-" + scenario.review.id);
    await optionList.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await expect(retry).toBeVisible();
    const retryBox = await retry.boundingBox();
    if (!retryBox) throw new Error("Workflow retry has no layout box");
    expect(await retry.evaluate((element) => getComputedStyle(element).minHeight)).toBe("48px");
    expect(retryBox.height).toBeGreaterThanOrEqual(44);
    expect(retryBox.width).toBeGreaterThanOrEqual(44);
    const featureOption = testPage.getByTestId("workflow-option-select-" + scenario.feature.id);
    const featureBox = await featureOption.boundingBox();
    if (!featureBox) throw new Error("Feature workflow option has no layout box");
    expect(featureBox.height).toBeGreaterThanOrEqual(44);

    const visibleRetryBox = await retry.boundingBox();
    const visibleListBox = await optionList.boundingBox();
    if (!visibleRetryBox || !visibleListBox) throw new Error("Workflow retry is not measurable");
    expect(visibleRetryBox.y).toBeGreaterThanOrEqual(visibleListBox.y);
    expect(visibleRetryBox.y + visibleRetryBox.height).toBeLessThanOrEqual(
      visibleListBox.y + visibleListBox.height + 2,
    );
    const retryTapPoint = {
      x: visibleRetryBox.x + visibleRetryBox.width / 2,
      y: visibleRetryBox.y + visibleRetryBox.height / 2,
    };
    const retryHitTarget = await testPage.evaluate(
      ({ x, y, testId }) => {
        const hit = document.elementFromPoint(x, y);
        return {
          isRetry: !!hit?.closest(`[data-testid="${testId}"]`),
          hitTag: hit?.tagName,
          hitTestId: hit?.getAttribute("data-testid"),
        };
      },
      {
        ...retryTapPoint,
        testId: "workflow-preview-retry-" + scenario.review.id,
      },
    );
    expect(retryHitTarget.isRetry, JSON.stringify(retryHitTarget)).toBe(true);

    const retryResponse = workflowStepsResponse(testPage, scenario.review.id);
    await testPage.touchscreen.tap(retryTapPoint.x, retryTapPoint.y);
    await expect(popover).toBeVisible();
    const retryResponseResult = await retryResponse;
    expect(retryResponseResult.ok()).toBe(true);
    expect(
      (await retryResponseResult.json()).steps.map((step: { name: string }) => step.name),
    ).toEqual(scenario.review.stepNames);
    await expect(workflowSelector).toHaveAttribute("aria-expanded", "true");
    await expect(popover).toBeVisible();
    await expect(workflowSelector).toContainText("Preview Kanban");
    await expect(testPage.getByTestId("workflow-option-" + scenario.review.id)).toContainText(
      scenario.review.stepNames[0],
    );
    await expectStepsInOrder(testPage, scenario.review.id, scenario.review.stepNames);

    const closePicker = popover.getByTestId("workflow-selector-close");
    const closePickerBox = await closePicker.boundingBox();
    if (!closePickerBox) throw new Error("Workflow selector close control is not measurable");
    expect(closePickerBox.width).toBeGreaterThanOrEqual(44);
    expect(closePickerBox.height).toBeGreaterThanOrEqual(44);
    await closePicker.tap();
    await expect(popover).toHaveCount(0);
    const featureRefresh = workflowStepsResponse(testPage, scenario.feature.id);
    await workflowSelector.tap();
    expect((await featureRefresh).ok()).toBe(true);
    await expectStepsInOrder(testPage, scenario.feature.id, scenario.feature.stepNames);
    await testPage.getByTestId("workflow-option-select-" + scenario.feature.id).tap();
    await expect(workflowSelector).toContainText("Feature Plan");
    await expect(dialog.getByTestId("task-create-launch-step")).toHaveText("Analysis");
    await expect(title).toHaveValue("Keep the phone draft");
    await expectTaskDescription(description, "Check long workflow previews on a phone.");

    const documentWidth = await testPage.evaluate(() => ({
      scroll: document.documentElement.scrollWidth,
      client: document.documentElement.clientWidth,
    }));
    expect(documentWidth.scroll).toBeLessThanOrEqual(documentWidth.client);
  } finally {
    await cleanupWorkflowStepPreviewScenario(apiClient, scenario);
  }
});

// @covers AC-TASKS-CREATE-WORKFLOW-STEPS-001.5 AC-TASKS-CREATE-WORKFLOW-STEPS-001.6 AC-TASKS-CREATE-WORKFLOW-STEPS-001.7
test("touch scrolls ten workflow options and selects either end", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const scenario = await seedWorkflowStepPreviewScenario(apiClient, seedData.workspaceId, {
    extraWorkflowCount: 7,
    longWorkflowSteps: true,
  });
  const pickerWorkflows = [
    {
      id: seedData.workflowId,
      name: "E2E Workflow",
      stepNames: seedData.steps.map(({ name }) => name),
    },
    ...scenario.allWorkflows,
  ];

  try {
    for (const workflow of scenario.allWorkflows) {
      expect(workflow.stepNames).toHaveLength(15);
    }
    await testPage.setViewportSize({ width: 390, height: 640 });
    expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
    await testPage.goto("/t/" + scenario.taskId);
    await expect(testPage).toHaveURL(new RegExp("/t/" + scenario.taskId + "$"));
    await testPage.getByTestId("mobile-task-picker-trigger").tap();
    await testPage
      .getByRole("dialog", { name: "Tasks", exact: true })
      .getByRole("button", { name: "New", exact: true })
      .tap();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const title = dialog.getByTestId("task-title-input");
    const description = dialog.getByTestId("task-description-input");
    await title.fill("Keep the phone draft while scrolling");
    await description.fill("Reach both ends of every long workflow preview.");
    const workflowSelector = dialog.getByTestId("workflow-selector-trigger");
    await workflowSelector.scrollIntoViewIfNeeded();
    const previewResponses = armWorkflowStepPreviewResponses(testPage, pickerWorkflows);
    await workflowSelector.tap();

    const popover = testPage.getByTestId("workflow-selector-popover");
    await expect(popover).toBeVisible();
    await expectWorkflowStepPreviewsLoaded(testPage, pickerWorkflows, previewResponses);
    const { optionList } = await expectWorkflowPickerOverflow(testPage, pickerWorkflows);
    const { first, last } = await getWorkflowPickerEndOptions(
      testPage,
      optionList,
      pickerWorkflows,
    );
    const renderedWorkflowOptionIds = await optionList
      .locator("button[data-testid^='workflow-option-select-']")
      .evaluateAll((buttons) => buttons.map((button) => button.getAttribute("data-testid")));
    expect(await first.getAttribute("data-testid")).toBe(renderedWorkflowOptionIds[0]);
    expect(await last.getAttribute("data-testid")).toBe(renderedWorkflowOptionIds.at(-1));

    const selectedBefore = await optionList
      .locator("button[aria-pressed='true']")
      .getAttribute("data-testid");
    expect(selectedBefore).toBe(`workflow-option-select-${scenario.kanban.id}`);
    await touchWorkflowOptionListToBoundary(testPage, optionList, "down");
    await expect(popover).toBeVisible();
    await expectWorkflowOptionVisibleAndHitTestable(testPage, optionList, last);
    await expect(optionList.locator("button[aria-pressed='true']")).toHaveAttribute(
      "data-testid",
      selectedBefore!,
    );
    await expect(workflowSelector).toContainText("Preview Kanban");
    await expect(title).toHaveValue("Keep the phone draft while scrolling");
    await expectTaskDescription(description, "Reach both ends of every long workflow preview.");

    await touchWorkflowOptionListToBoundary(testPage, optionList, "up");
    await expect(popover).toBeVisible();
    await expectWorkflowOptionVisibleAndHitTestable(testPage, optionList, first);
    await expect(optionList.locator("button[aria-pressed='true']")).toHaveAttribute(
      "data-testid",
      selectedBefore!,
    );
    await expect(title).toHaveValue("Keep the phone draft while scrolling");
    await expectTaskDescription(description, "Reach both ends of every long workflow preview.");

    const firstName = await first.getAttribute("aria-label");
    const firstPoint = await expectWorkflowOptionVisibleAndHitTestable(testPage, optionList, first);
    await testPage.touchscreen.tap(firstPoint.x, firstPoint.y);
    await expect(popover).toHaveCount(0);
    await expect(workflowSelector).toContainText(firstName!);
    await expect(workflowSelector).toBeFocused();
    await expect(title).toHaveValue("Keep the phone draft while scrolling");
    await expectTaskDescription(description, "Reach both ends of every long workflow preview.");

    const reopenedPreviewResponses = armWorkflowStepPreviewResponses(testPage, pickerWorkflows);
    await workflowSelector.tap();
    await expect(popover).toBeVisible();
    await expectWorkflowStepPreviewsLoaded(
      testPage,
      scenario.allWorkflows,
      reopenedPreviewResponses,
    );
    const selectedFirst = await optionList
      .locator("button[aria-pressed='true']")
      .getAttribute("data-testid");
    expect(selectedFirst).toBe(await first.getAttribute("data-testid"));
    await touchWorkflowOptionListToBoundary(testPage, optionList, "down");
    await expect(popover).toBeVisible();
    await expectWorkflowOptionVisibleAndHitTestable(testPage, optionList, last);
    await expect(optionList.locator("button[aria-pressed='true']")).toHaveAttribute(
      "data-testid",
      selectedFirst!,
    );
    await expect(title).toHaveValue("Keep the phone draft while scrolling");
    await expectTaskDescription(description, "Reach both ends of every long workflow preview.");

    const lastName = await last.getAttribute("aria-label");
    const lastPoint = await expectWorkflowOptionVisibleAndHitTestable(testPage, optionList, last);
    await testPage.touchscreen.tap(lastPoint.x, lastPoint.y);
    await expect(popover).toHaveCount(0);
    await expect(workflowSelector).toContainText(lastName!);
    await expect(workflowSelector).toBeFocused();
    await expect(title).toHaveValue("Keep the phone draft while scrolling");
    await expectTaskDescription(description, "Reach both ends of every long workflow preview.");

    const documentWidth = await testPage.evaluate(() => ({
      scroll: document.documentElement.scrollWidth,
      client: document.documentElement.clientWidth,
    }));
    expect(documentWidth.scroll).toBeLessThanOrEqual(documentWidth.client);
  } finally {
    await cleanupWorkflowStepPreviewScenario(apiClient, scenario);
  }
});
