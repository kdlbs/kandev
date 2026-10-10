import { expect, test } from "../../fixtures/test-base";
import { restoreSidebarLayout } from "../../helpers/sidebar-layout";
import { useRegularMode } from "../../helpers/regular-mode";
import { AppSidebarPage } from "../../pages/app-sidebar-page";
import { expectTaskDescription } from "../../pages/task-description-editor";
import {
  armWorkflowStepPreviewResponses,
  cleanupWorkflowStepPreviewScenario,
  expectSmallAndDiagonalWorkflowWheelDeltas,
  expectWorkflowOptionVisibleAndHitTestable,
  expectWorkflowPickerOverflow,
  expectWorkflowStepPreviewsLoaded,
  getWorkflowPickerEndOptions,
  expectStepsInOrder,
  expectUnbrokenStepToFitGroup,
  seedWorkflowStepPreviewScenario,
  wheelWorkflowOptionListToBoundary,
  workflowStepsResponse,
} from "./workflow-step-previews-helpers";

useRegularMode();

// @covers AC-TASKS-CREATE-WORKFLOW-STEPS-001.1 AC-TASKS-CREATE-WORKFLOW-STEPS-001.2 AC-TASKS-CREATE-WORKFLOW-STEPS-001.3 AC-TASKS-CREATE-WORKFLOW-STEPS-001.5 AC-TASKS-CREATE-WORKFLOW-STEPS-001.6
test("loads every workflow preview from a task page and retries one failed row", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const initialLayout = (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
    seedData.workspaceId
  ];
  const scenario = await seedWorkflowStepPreviewScenario(apiClient, seedData.workspaceId);
  let reviewAttempts = 0;
  let allowReviewRetry = () => {};
  const reviewRetryGate = new Promise<void>((resolve) => {
    allowReviewRetry = resolve;
  });
  let markRetryRequestStarted = () => {};
  const retryRequestStarted = new Promise<void>((resolve) => {
    markRetryRequestStarted = resolve;
  });
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
      markRetryRequestStarted();
      await reviewRetryGate;
      await route.continue();
    },
  );

  try {
    await testPage.goto("/t/" + scenario.taskId);
    await expect(testPage).toHaveURL(new RegExp("/t/" + scenario.taskId + "$"));
    await new AppSidebarPage(testPage).expandNavigationIfCollapsed();
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
    await expectUnbrokenStepToFitGroup(
      testPage,
      scenario.feature.id,
      scenario.feature.unbrokenStepName,
    );

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
    expect(await testPage.evaluate(() => matchMedia("(pointer: fine)").matches)).toBe(true);
    expect(wideRetryBox.height).toBe(28);

    await testPage.setViewportSize({ width: 640, height: 720 });
    await retry.scrollIntoViewIfNeeded();
    const retryResponse = workflowStepsResponse(testPage, scenario.review.id);
    const reviewOption = testPage.getByTestId("workflow-option-select-" + scenario.review.id);
    await retry.focus();
    await retry.press("Enter");
    await retryRequestStarted;
    await expect(retry).toBeFocused();
    await expect(retry).toHaveAttribute("aria-disabled", "true");
    await expect(reviewOption).toHaveAccessibleName("Contributor Review");
    await expect(testPage.getByTestId("workflow-selector-popover").getByRole("status")).toHaveCount(
      1,
    );
    await expect(testPage.getByTestId("workflow-preview-status-announcement")).toContainText(
      "Contributor Review: Loading steps",
    );
    allowReviewRetry();
    expect((await retryResponse).ok()).toBe(true);
    await expectStepsInOrder(testPage, scenario.review.id, scenario.review.stepNames);
    await expect(reviewOption).toBeFocused();
    await expect(workflowSelector).toHaveAttribute("aria-expanded", "true");
    const popover = testPage.getByTestId("workflow-selector-popover");
    await testPage.keyboard.press("Escape");
    await expect(popover).toHaveCount(0);
    await expect(workflowSelector).toBeFocused();
    const featureRefresh = workflowStepsResponse(testPage, scenario.feature.id);
    await workflowSelector.press("Enter");
    expect((await featureRefresh).ok()).toBe(true);
    await expect(popover).toBeVisible();
    await expectStepsInOrder(testPage, scenario.feature.id, scenario.feature.stepNames);
    await expect(title).toHaveValue("Preserve the task draft");
    await expectTaskDescription(description, "Compare the workflow steps before choosing.");

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
    allowReviewRetry();
    await cleanupWorkflowStepPreviewScenario(apiClient, scenario);
    await restoreSidebarLayout(apiClient, seedData.workspaceId, initialLayout);
  }
});

test("checks workflow preview order when titles repeat or contain another title", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const scenario = await seedWorkflowStepPreviewScenario(apiClient, seedData.workspaceId);
  const names = ["Review complete", "Review", "Review"];
  const { steps } = await apiClient.listWorkflowSteps(scenario.review.id);
  try {
    for (const step of steps) await apiClient.deleteWorkflowStep(step.id);
    for (const [position, name] of names.entries()) {
      await apiClient.createWorkflowStep(scenario.review.id, name, position);
    }
    await testPage.goto(`/t/${scenario.taskId}`);
    await testPage.getByTestId("create-task-button").first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const response = workflowStepsResponse(testPage, scenario.review.id);
    await dialog.getByTestId("workflow-selector-trigger").click();
    expect((await response).ok()).toBe(true);
    await expectStepsInOrder(testPage, scenario.review.id, names);
  } finally {
    await cleanupWorkflowStepPreviewScenario(apiClient, scenario);
  }
});

// @covers AC-TASKS-CREATE-WORKFLOW-STEPS-001.5 AC-TASKS-CREATE-WORKFLOW-STEPS-001.6 AC-TASKS-CREATE-WORKFLOW-STEPS-001.7
test("scrolls ten workflow options in both directions without losing the task draft", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const initialLayout = (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
    seedData.workspaceId
  ];
  const scenario = await seedWorkflowStepPreviewScenario(apiClient, seedData.workspaceId, {
    extraWorkflowCount: 7,
    longWorkflowSteps: true,
  });
  const { steps: seedSteps } = await apiClient.listWorkflowSteps(seedData.workflowId);
  const pickerWorkflows = [
    {
      id: seedData.workflowId,
      name: "E2E Workflow",
      stepNames: [...seedSteps].sort((a, b) => a.position - b.position).map(({ name }) => name),
    },
    ...scenario.allWorkflows,
  ];

  try {
    for (const workflow of scenario.allWorkflows) {
      expect(workflow.stepNames).toHaveLength(15);
    }
    await testPage.setViewportSize({ width: 1682, height: 768 });
    await testPage.goto("/t/" + scenario.taskId);
    await expect(testPage).toHaveURL(new RegExp("/t/" + scenario.taskId + "$"));
    await new AppSidebarPage(testPage).expandNavigationIfCollapsed();
    await testPage.getByTestId("create-task-button").first().click();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const title = dialog.getByTestId("task-title-input");
    const description = dialog.getByTestId("task-description-input");
    await title.fill("Preserve the task draft while scrolling");
    await description.fill("Review every long workflow preview before choosing.");
    const workflowSelector = dialog.getByTestId("workflow-selector-trigger");
    const previewResponses = armWorkflowStepPreviewResponses(testPage, pickerWorkflows);
    await workflowSelector.click();

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

    for (const viewport of [
      { width: 1682, height: 768 },
      { width: 1280, height: 600 },
    ]) {
      await testPage.setViewportSize(viewport);
      await expectWorkflowPickerOverflow(testPage, pickerWorkflows);
      await expectSmallAndDiagonalWorkflowWheelDeltas(
        testPage,
        optionList,
        pickerWorkflows.map(({ id }) => id),
      );
      await wheelWorkflowOptionListToBoundary(testPage, optionList, "down");
      await expectWorkflowOptionVisibleAndHitTestable(testPage, optionList, last);
      await expect(workflowSelector).toContainText("Preview Kanban");
      await expect(title).toHaveValue("Preserve the task draft while scrolling");
      await expectTaskDescription(
        description,
        "Review every long workflow preview before choosing.",
      );

      await wheelWorkflowOptionListToBoundary(testPage, optionList, "up");
      await expectWorkflowOptionVisibleAndHitTestable(testPage, optionList, first);
      await expect(optionList.locator("button[aria-pressed='true']")).toHaveAttribute(
        "data-testid",
        selectedBefore!,
      );
      await expect(title).toHaveValue("Preserve the task draft while scrolling");
      await expectTaskDescription(
        description,
        "Review every long workflow preview before choosing.",
      );
    }

    await testPage.keyboard.press("Escape");
    await expect(popover).toHaveCount(0);
    await expect(workflowSelector).toBeFocused();
    const keyboardPreviewResponses = armWorkflowStepPreviewResponses(testPage, pickerWorkflows);
    await workflowSelector.press("Enter");
    await expect(popover).toBeVisible();
    await expectWorkflowStepPreviewsLoaded(testPage, pickerWorkflows, keyboardPreviewResponses);
    await optionList.evaluate((element) => {
      element.scrollTop = 0;
    });
    let lastOptionFocused = false;
    for (let attempt = 0; attempt < 16; attempt += 1) {
      lastOptionFocused = await last.evaluate((element) => element === document.activeElement);
      if (lastOptionFocused) break;
      await testPage.keyboard.press("Tab");
    }
    expect(lastOptionFocused).toBe(true);
    await expectWorkflowOptionVisibleAndHitTestable(testPage, optionList, last);

    await optionList.evaluate((element) => {
      element.scrollTop = 0;
    });
    const firstPoint = await expectWorkflowOptionVisibleAndHitTestable(testPage, optionList, first);
    const firstName = await first.getAttribute("aria-label");
    await testPage.mouse.click(firstPoint.x, firstPoint.y);
    await expect(popover).toHaveCount(0);
    await expect(workflowSelector).toContainText(firstName!);
    await expect(workflowSelector).toBeFocused();
    await expect(title).toHaveValue("Preserve the task draft while scrolling");
    await expectTaskDescription(description, "Review every long workflow preview before choosing.");

    const lastSelectionPreviewResponses = armWorkflowStepPreviewResponses(
      testPage,
      pickerWorkflows,
    );
    await workflowSelector.press("Enter");
    await expect(popover).toBeVisible();
    await expectWorkflowStepPreviewsLoaded(
      testPage,
      scenario.allWorkflows,
      lastSelectionPreviewResponses,
    );
    await wheelWorkflowOptionListToBoundary(testPage, optionList, "down");
    const lastPoint = await expectWorkflowOptionVisibleAndHitTestable(testPage, optionList, last);
    const lastName = await last.getAttribute("aria-label");
    await testPage.mouse.click(lastPoint.x, lastPoint.y);
    await expect(popover).toHaveCount(0);
    await expect(workflowSelector).toContainText(lastName!);
    await expect(workflowSelector).toBeFocused();
    await expect(title).toHaveValue("Preserve the task draft while scrolling");
    await expectTaskDescription(description, "Review every long workflow preview before choosing.");
  } finally {
    await cleanupWorkflowStepPreviewScenario(apiClient, scenario);
    await restoreSidebarLayout(apiClient, seedData.workspaceId, initialLayout);
  }
});
