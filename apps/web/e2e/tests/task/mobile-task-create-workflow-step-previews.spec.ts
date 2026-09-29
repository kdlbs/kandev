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

// @covers AC-TASKS-CREATE-WORKFLOW-STEPS-001.1 AC-TASKS-CREATE-WORKFLOW-STEPS-001.2 AC-TASKS-CREATE-WORKFLOW-STEPS-001.3 AC-TASKS-CREATE-WORKFLOW-STEPS-001.6
test("keeps long workflow previews contained and touch-usable on a phone", async ({
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
    await testPage.setViewportSize({ width: 390, height: 640 });
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
    const featureResponse = workflowStepsResponse(testPage, scenario.feature.id);
    const reviewResponse = workflowStepsResponse(testPage, scenario.review.id);
    await workflowSelector.tap();
    expect((await featureResponse).ok()).toBe(true);
    expect((await reviewResponse).status()).toBe(503);

    await expectStepsInOrder(testPage, scenario.kanban.id, scenario.kanban.stepNames);
    await expectStepsInOrder(testPage, scenario.feature.id, scenario.feature.stepNames);
    await expect(testPage.getByTestId("workflow-option-" + scenario.review.id)).toContainText(
      "Failed to load workflow steps",
    );

    const popover = testPage.getByTestId("workflow-selector-popover");
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

    await retry.evaluate((element) => {
      const list = element.closest<HTMLElement>("[data-testid='workflow-selector-option-list']");
      if (!list) throw new Error("Workflow retry is outside the option list");
      const retryBox = element.getBoundingClientRect();
      const listBox = list.getBoundingClientRect();
      if (retryBox.bottom > listBox.bottom) list.scrollTop += retryBox.bottom - listBox.bottom;
      if (retryBox.top < listBox.top) list.scrollTop -= listBox.top - retryBox.top;
    });
    const visibleRetryBox = await retry.boundingBox();
    const visibleListBox = await optionList.boundingBox();
    if (!visibleRetryBox || !visibleListBox) throw new Error("Workflow retry is not measurable");
    expect(visibleRetryBox.y).toBeGreaterThanOrEqual(visibleListBox.y);
    expect(visibleRetryBox.y + visibleRetryBox.height).toBeLessThanOrEqual(
      visibleListBox.y + visibleListBox.height + 2,
    );

    const retryResponse = workflowStepsResponse(testPage, scenario.review.id);
    await retry.tap();
    expect((await retryResponse).ok()).toBe(true);
    await expectStepsInOrder(testPage, scenario.review.id, scenario.review.stepNames);

    await testPage.keyboard.press("Escape");
    await expect(popover).toHaveCount(0);
    await expect(workflowSelector).toBeFocused();
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
