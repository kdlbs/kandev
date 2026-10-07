import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { SessionPage } from "../../pages/session-page";
import type { SidebarTaskColorAutomation } from "../../../lib/task-color-automation-settings";
import { scrollSidebarFilterListTopIntoView, touchDragToPoint } from "../../helpers/touch-drag";

const MOBILE_REPOSITORY_RULE_ID = "mobile-repository-rule";
let previousAutomaticColors: SidebarTaskColorAutomation = { enabled: false, rules: [] };

test.beforeEach(async ({ apiClient }) => {
  const { settings } = await apiClient.getUserSettings();
  previousAutomaticColors = (settings.sidebar_task_color_automation as
    | SidebarTaskColorAutomation
    | undefined) ?? {
    enabled: false,
    rules: [],
  };
});

test.afterEach(async ({ apiClient }) => {
  await apiClient.saveUserSettings({ sidebar_task_color_automation: previousAutomaticColors });
});

test.describe("Mobile sidebar automatic task colors", () => {
  test("keeps repository selection in the drawer and applies the stored rule", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Mobile automatic color task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const navTask = await apiClient.seedTask(seedData.workspaceId, "Automatic colors mobile nav", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const automation: SidebarTaskColorAutomation = {
      enabled: true,
      rules: [
        {
          id: MOBILE_REPOSITORY_RULE_ID,
          enabled: true,
          condition: {
            dimension: "repository",
            value: {
              kind: "local",
              path: seedData.repositoryPath,
            },
            label: "E2E Repo",
          },
          output: { kind: "fixed", color: "purple" },
        },
      ],
    };
    await apiClient.saveUserSettings({ sidebar_task_color_automation: automation });

    await testPage.goto(`/t/${navTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-task-picker-trigger").click();
    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    await expect(sheet.getByTestId("sidebar-filter-bar")).toBeVisible();

    const row = sheet
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Mobile automatic color task" })
      .first();
    await expect(row).toBeVisible();
    await expect(row.getByTestId("task-item-color-marker")).toHaveAttribute(
      "data-color-token",
      "purple",
    );

    await sheet.getByTestId("sidebar-filter-gear").tap();
    const popover = testPage.getByTestId("sidebar-filter-popover");
    await expect(popover).toBeVisible();
    const automaticSettings = popover.getByTestId("automatic-color-settings");
    await automaticSettings.getByTestId("automatic-color-settings-toggle").tap();
    await expect(automaticSettings.getByText("Rule 1 Repository", { exact: true })).toBeVisible();
    await expect(automaticSettings.getByTestId("automatic-colors-timing")).toHaveCount(0);
    await expect(automaticSettings.getByTestId("automatic-colors-help")).toHaveCount(1);
    await expect(
      automaticSettings.getByText(
        "Personal setting. Applies across sidebar views and workspaces.",
        { exact: true },
      ),
    ).toHaveCount(0);
    await automaticSettings.getByTestId("automatic-colors-help").focus();
    const timingTooltip = testPage.getByRole("tooltip");
    await expect(timingTooltip).toContainText("Rules apply to existing and new sidebar tasks.");

    const sectionPadding = await Promise.all(
      ["sidebar-sort-settings", "sidebar-group-settings", "task-row-settings"].map((testId) =>
        popover.getByTestId(testId).evaluate((element) => {
          const styles = getComputedStyle(element);
          return { paddingBottom: styles.paddingBottom, paddingTop: styles.paddingTop };
        }),
      ),
    );
    expect(sectionPadding).toEqual([
      { paddingBottom: "4px", paddingTop: "4px" },
      { paddingBottom: "4px", paddingTop: "4px" },
      { paddingBottom: "4px", paddingTop: "4px" },
    ]);

    const repositoryTrigger = automaticSettings.getByTestId(
      `automatic-color-repository-trigger-${MOBILE_REPOSITORY_RULE_ID}`,
    );
    await expect(repositoryTrigger).toBeVisible();
    await repositoryTrigger.tap();
    const repositoryPane = automaticSettings.getByTestId("automatic-color-repository-pane");
    await expect(repositoryPane).toBeVisible();
    const repositoryOption = repositoryPane.getByRole("button", { name: "E2E Repo", exact: false });
    await expect(repositoryOption).toBeVisible();
    await expect(testPage.getByTestId("sidebar-filter-drawer")).toContainText("E2E Repo");
    await prCapture.screenshot("mobile-automatic-task-colors", {
      caption: "Mobile repository target picker for automatic task colors",
    });

    await repositoryOption.tap();
    await expect(repositoryPane).toBeHidden();

    await testPage.reload();
    await session.waitForLoad();
    await testPage.getByTestId("mobile-task-picker-trigger").click();
    const reloadedSheet = testPage.getByRole("dialog", { name: "Tasks" });
    await expect(reloadedSheet.getByTestId("sidebar-filter-bar")).toBeVisible();
    await expect(
      reloadedSheet
        .getByTestId("sidebar-task-item")
        .filter({ hasText: "Mobile automatic color task" })
        .getByTestId("task-item-color-marker"),
    ).toHaveAttribute("data-color-token", "purple");
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.sidebar_task_color_automation)
      .toEqual(automation);
  });

  test("uses the More menu to change an overlapping color rule on phone and after reload", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const ruleId = "mobile-overlap";
    const automation: SidebarTaskColorAutomation = {
      enabled: true,
      rules: [
        {
          id: `${ruleId}-purple`,
          enabled: true,
          condition: { dimension: "task_state", value: "FAILED", label: "Failed" },
          output: { kind: "fixed", color: "purple" },
        },
        {
          id: `${ruleId}-green`,
          enabled: true,
          condition: { dimension: "task_state", value: "FAILED", label: "Failed" },
          output: { kind: "fixed", color: "green" },
        },
      ],
    };
    const task = await apiClient.seedTask(seedData.workspaceId, "Mobile overlapping color task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      state: "FAILED",
    });
    const navTask = await apiClient.seedTask(seedData.workspaceId, "Mobile color menu nav", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.saveUserSettings({ sidebar_task_color_automation: automation });

    await testPage.goto(`/t/${navTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-task-picker-trigger").tap();
    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    const row = sheet.locator(`[data-task-row-id="${task.task_id}"]`);
    const marker = row.getByTestId("task-item-color-marker");
    await expect(marker).toHaveAttribute("data-color-token", "purple");

    await sheet.getByTestId("sidebar-filter-gear").tap();
    const popover = testPage.getByTestId("sidebar-filter-popover");
    const settings = popover.getByTestId("automatic-color-settings");
    await settings.getByTestId("automatic-color-settings-toggle").tap();
    const firstCard = settings.getByTestId(`automatic-color-rule-${ruleId}-purple`);
    const secondCard = settings.getByTestId(`automatic-color-rule-${ruleId}-green`);
    const firstHandle = firstCard.getByTestId(`automatic-color-rule-handle-${ruleId}-purple`);
    const secondHandle = secondCard.getByTestId(`automatic-color-rule-handle-${ruleId}-green`);
    const secondMore = secondCard.getByTestId(`automatic-color-rule-more-${ruleId}-green`);
    const secondRemove = secondCard.getByTestId(`automatic-color-rule-remove-${ruleId}-green`);
    for (const control of [secondHandle, secondMore, secondRemove]) {
      const controlBox = await control.boundingBox();
      const cardBox = await secondCard.boundingBox();
      expect(controlBox?.height).toBeGreaterThanOrEqual(44);
      expect(controlBox?.width).toBeGreaterThanOrEqual(44);
      expect(controlBox?.x).toBeGreaterThanOrEqual(cardBox!.x);
      expect(controlBox!.x + controlBox!.width).toBeLessThanOrEqual(cardBox!.x + cardBox!.width);
      expect(controlBox?.y).toBeGreaterThanOrEqual(cardBox!.y);
      expect(controlBox!.y + controlBox!.height).toBeLessThanOrEqual(cardBox!.y + cardBox!.height);
    }

    const ruleList = settings.getByTestId("automatic-color-rule-list");
    if (prCapture.capturing) {
      await scrollSidebarFilterListTopIntoView(ruleList);
      await waitForFiniteAnimations(settings);
      await prCapture.screenshot("mobile-automatic-color-rule-order", {
        caption: "Phone automatic color rules with touch-sized reorder handles and move menus",
      });
    }

    let colorSettingsPatchCount = 0;
    testPage.on("request", (request) => {
      if (
        request.method() === "PATCH" &&
        new URL(request.url()).pathname === "/api/v1/user/settings"
      ) {
        colorSettingsPatchCount += 1;
      }
    });
    const settingsBeforeOutsideDrop = (await apiClient.getUserSettings()).settings
      .sidebar_task_color_automation;
    await scrollSidebarFilterListTopIntoView(ruleList);
    const readRuleOrder = () =>
      ruleList
        .locator(":scope > [data-testid^='automatic-color-rule-']")
        .evaluateAll((cards) =>
          cards.map((card) =>
            card.getAttribute("data-testid")?.replace("automatic-color-rule-", ""),
          ),
        );
    const ruleListBox = await ruleList.boundingBox();
    const editorBox = await popover.boundingBox();
    expect(ruleListBox).not.toBeNull();
    expect(editorBox).not.toBeNull();
    const outsideY = ruleListBox!.y - 8;
    expect(outsideY).toBeGreaterThan(editorBox!.y);
    await touchDragToPoint(testPage, secondHandle, {
      x: ruleListBox!.x + ruleListBox!.width / 2,
      y: outsideY,
    });
    await expect.poll(readRuleOrder).toEqual([`${ruleId}-purple`, `${ruleId}-green`]);
    expect(colorSettingsPatchCount).toBe(0);
    expect((await apiClient.getUserSettings()).settings.sidebar_task_color_automation).toEqual(
      settingsBeforeOutsideDrop,
    );

    const drawer = testPage.getByTestId("sidebar-filter-drawer");
    await scrollSidebarFilterListTopIntoView(ruleList);
    await waitForFiniteAnimations(drawer);
    const drawerBeforeDownwardDrag = await drawer.boundingBox();
    await expect(firstHandle).toHaveAttribute("data-vaul-no-drag", "");
    const secondCardBox = await secondCard.boundingBox();
    expect(secondCardBox).not.toBeNull();
    await touchDragToPoint(testPage, firstHandle, {
      x: secondCardBox!.x + secondCardBox!.width / 2,
      y: secondCardBox!.y + 2,
    });
    await expect(marker).toHaveAttribute("data-color-token", "green");
    await expect.poll(readRuleOrder).toEqual([`${ruleId}-green`, `${ruleId}-purple`]);
    await expect(drawer).toBeVisible();
    await waitForFiniteAnimations(drawer);
    const drawerAfterDownwardDrag = await drawer.boundingBox();
    expect(drawerBeforeDownwardDrag).not.toBeNull();
    expect(drawerAfterDownwardDrag).not.toBeNull();
    expect(Math.abs(drawerAfterDownwardDrag!.y - drawerBeforeDownwardDrag!.y)).toBeLessThan(1);

    await settings.getByTestId(`automatic-color-rule-more-${ruleId}-purple`).tap();
    const purpleMoveUp = testPage.getByTestId(`automatic-color-rule-more-${ruleId}-purple-move-up`);
    await waitForFiniteAnimations(testPage.getByRole("menu").filter({ has: purpleMoveUp }));
    await purpleMoveUp.tap();
    await expect(marker).toHaveAttribute("data-color-token", "purple");

    await secondMore.tap();
    const moveUp = testPage.getByTestId(`automatic-color-rule-more-${ruleId}-green-move-up`);
    await waitForFiniteAnimations(testPage.getByRole("menu").filter({ has: moveUp }));
    const moveUpBox = await moveUp.boundingBox();
    expect(moveUpBox?.height).toBeGreaterThanOrEqual(44);
    await moveUp.tap();
    await expect(marker).toHaveAttribute("data-color-token", "green");
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );

    await testPage.reload();
    await session.waitForLoad();
    await testPage.getByTestId("mobile-task-picker-trigger").tap();
    const reloadedRow = testPage
      .getByRole("dialog", { name: "Tasks" })
      .locator(`[data-task-row-id="${task.task_id}"]`);
    await expect(reloadedRow.getByTestId("task-item-color-marker")).toHaveAttribute(
      "data-color-token",
      "green",
    );
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.sidebar_task_color_automation)
      .toEqual({ enabled: true, rules: [automation.rules[1], automation.rules[0]] });
  });
});
