import { expect, test } from "../../fixtures/test-base";
import {
  assertLocatorWithinViewportX,
  assertNoDocumentHorizontalOverflow,
  assertNoElementHorizontalOverflow,
} from "../../helpers/layout-assertions";
import { useRegularMode } from "../../helpers/regular-mode";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

useRegularMode();

test.describe("Create task dependency selector on mobile", () => {
  test("keeps the selector contained and supports touch selection and help", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const predecessor = await apiClient.createTask(
      seedData.workspaceId,
      "Mobile dependency predecessor with a long title",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.mockGitHubAssociateTaskPR({
      workspace_id: seedData.workspaceId,
      task_id: predecessor.id,
      owner: "kandev-e2e",
      repo: "dependency-fixtures",
      pr_number: 188,
      pr_url: "https://github.test/kandev-e2e/dependency-fixtures/pull/188",
      pr_title: "Dependency fixture PR",
      head_branch: "feature/dependency-fixture",
      base_branch: "main",
      author_login: "e2e",
      state: "open",
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileFab.tap();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const advancedTrigger = dialog.getByTestId("task-create-advanced-settings-trigger");
    const advancedTriggerBox = await advancedTrigger.boundingBox();
    expect(advancedTriggerBox).not.toBeNull();
    expect(advancedTriggerBox!.height).toBeGreaterThanOrEqual(44);
    await assertLocatorWithinViewportX(advancedTrigger, "mobile advanced settings trigger");
    await expect(dialog.getByTestId("task-create-dependencies-trigger")).toHaveCount(0);

    await advancedTrigger.tap();
    await expect(advancedTrigger).toHaveAttribute("aria-expanded", "true");
    const settingLabel = dialog.getByTestId("task-create-dependency-setting-label");
    await expect(settingLabel).toContainText("Depends on");
    const settingRow = dialog.getByTestId("task-create-dependency-setting-row");
    const settingRowBox = await settingRow.boundingBox();
    const settingLabelBox = await settingLabel.boundingBox();
    const selectorContainer = dialog.getByTestId("task-create-dependency-selector-container");
    const selectorContainerBox = await selectorContainer.boundingBox();
    expect(settingRowBox).not.toBeNull();
    expect(settingLabelBox).not.toBeNull();
    expect(selectorContainerBox).not.toBeNull();
    expect(selectorContainerBox!.x).toBeGreaterThan(settingLabelBox!.x + settingLabelBox!.width);
    expect(Math.abs(selectorContainerBox!.y - settingLabelBox!.y)).toBeLessThanOrEqual(4);
    const settingInfo = dialog.getByTestId("task-create-dependency-setting-info");
    const settingInfoBox = await settingInfo.boundingBox();
    expect(settingInfoBox).not.toBeNull();
    expect(settingInfoBox!.height).toBeGreaterThanOrEqual(44);
    expect(settingInfoBox!.width).toBeGreaterThanOrEqual(44);
    await settingInfo.tap();
    await expect(testPage.locator('[data-slot="tooltip-content"]:visible').last()).toContainText(
      "This task waits until every selected task completes successfully.",
    );
    const trigger = dialog.getByTestId("task-create-dependencies-trigger");
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox).not.toBeNull();
    expect(triggerBox!.height).toBeGreaterThanOrEqual(44);
    await assertLocatorWithinViewportX(trigger, "mobile dependency trigger");
    await assertNoElementHorizontalOverflow(trigger, "mobile dependency trigger text");

    await trigger.tap();
    const picker = dialog.getByTestId("task-create-dependencies-popover");
    await expect(picker).toBeVisible();
    await assertLocatorWithinViewportX(picker, "mobile dependency picker");
    await assertNoElementHorizontalOverflow(picker, "mobile dependency picker");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile dependency picker");

    const info = picker.getByTestId("task-create-dependency-info");
    const infoBox = await info.boundingBox();
    expect(infoBox).not.toBeNull();
    expect(infoBox!.height).toBeGreaterThanOrEqual(44);
    expect(infoBox!.width).toBeGreaterThanOrEqual(44);
    await info.tap();
    await expect(testPage.locator('[data-slot="tooltip-content"]:visible').last()).toContainText(
      "This task waits until every selected task completes successfully.",
    );

    const option = picker.getByTestId(`task-create-dependency-option-${predecessor.id}`);
    await expect(option).toBeVisible();
    await expect(option.getByTestId("task-create-dependency-task-icon")).toBeVisible();
    const search = picker.getByPlaceholder("Search tasks or #PR/MR number...");
    await search.fill("188");
    await expect(option).toBeVisible();
    await expect(option.locator('[aria-label="Linked pull or merge request #188"]')).toHaveText(
      "#188",
    );
    await option.tap();
    await expect(trigger).toContainText("#188 · Mobile dependency predecessor");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile dependency selection");
  });
});
