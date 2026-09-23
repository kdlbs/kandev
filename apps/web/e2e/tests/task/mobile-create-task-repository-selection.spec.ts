import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { dwell } from "../../helpers/causal-waits";

useRegularMode();

test.describe("Create task workspace repository picker on mobile", () => {
  test("opens parent workspace help and persists the selected layout", async ({
    testPage,
    apiClient,
  }) => {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileFab.tap();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByTestId("task-create-advanced-settings-trigger").tap();
    const layoutSetting = dialog.getByTestId("task-create-initial-workspace-layout-setting");
    await expect(layoutSetting).toBeVisible();
    const info = layoutSetting.getByTestId("task-create-initial-workspace-layout-info");
    await info.tap();
    await expect(
      testPage.getByTestId("task-create-initial-workspace-layout-help-drawer"),
    ).toBeVisible();
    await testPage.keyboard.press("Escape");

    const checkbox = layoutSetting.getByTestId("task-create-initial-workspace-layout-checkbox");
    await checkbox.tap();
    await expect(checkbox).toHaveAttribute("data-state", "checked");
    await dialog.getByTestId("task-title-input").fill("Mobile parent workspace task");
    await dialog.getByTestId("task-description-input").fill("Create beside the repository");
    await dialog.getByRole("button", { name: "Create only", exact: true }).tap();
    await expect(dialog).toHaveCount(0);

    const card = new KanbanPage(testPage).taskCardByTitle("Mobile parent workspace task");
    await expect(card).toBeVisible({ timeout: 20_000 });
    const cardTestId = await card.getAttribute("data-testid");
    const taskId = cardTestId?.replace(/^task-card-/, "");
    if (!taskId || taskId === cardTestId) throw new Error(`unexpected task card: ${cardTestId}`);
    await expect
      .poll(() => apiClient.getTask(taskId), { timeout: 15_000 })
      .toMatchObject({ initial_workspace_layout: "task_root" });
  });

  test("marks another row's repository while keeping it selectable", async ({
    testPage,
    prCapture,
  }) => {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileFab.click();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const repositoryChips = dialog.getByTestId("repo-chip-trigger");
    await expect(repositoryChips.first()).toContainText("E2E Repo");

    await dialog.getByTestId("add-repository").click();
    await expect(repositoryChips).toHaveCount(2);
    await repositoryChips.nth(1).tap();

    const selectedElsewhere = testPage.getByRole("option", { name: /^E2E Repo/ });
    await expect(selectedElsewhere).toBeVisible();
    await expect(selectedElsewhere.getByTestId("already-added-repository-marker")).toBeVisible();
    await selectedElsewhere.tap();
    await expect(repositoryChips.nth(1)).toContainText("E2E Repo");
    await dwell(
      testPage,
      300,
      "negative-assertion",
      "a tap must not leave a hover tooltip behind on touch; nothing is rendered to wait for, so the check has to outlast Radix's open delay first",
    );
    await expect(testPage.getByRole("tooltip")).toBeHidden();
    await assertNoDocumentHorizontalOverflow(testPage, "repository picker selection");
    await prCapture.screenshot("mobile-repository-chip-selection", {
      caption:
        "After mobile repository selection, the chip stays contained and no tooltip is shown.",
    });
  });
});
