import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { SessionPage } from "../../pages/session-page";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";
import type { SidebarTaskColorAutomation } from "../../../lib/task-color-automation-settings";

const DESKTOP_AUTOMATION: SidebarTaskColorAutomation = {
  enabled: true,
  rules: [
    {
      id: "desktop-failed-rule",
      enabled: true,
      condition: { dimension: "task_state", value: "FAILED", label: "Failed" },
      output: { kind: "fixed", color: "red" },
    },
    {
      id: "desktop-review-rule",
      enabled: true,
      condition: { dimension: "task_state", value: "REVIEW", label: "Review" },
      output: { kind: "fixed", color: "blue" },
    },
  ],
};

function desktopRepositoryAutomation(repositoryPath: string): SidebarTaskColorAutomation {
  return {
    enabled: true,
    rules: [
      {
        id: "desktop-repository-rule",
        enabled: false,
        condition: {
          dimension: "repository",
          value: { kind: "local", path: repositoryPath },
          label: "E2E repository",
        },
        output: { kind: "fixed", color: "gray" },
      },
    ],
  };
}

async function openDesktopTask(
  testPage: import("@playwright/test").Page,
  apiClient: import("../../helpers/api-client").ApiClient,
  seedData: import("../../fixtures/test-base").SeedData,
  title: string,
  state: string,
) {
  const task = await apiClient.seedTask(seedData.workspaceId, title, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    state,
  });
  const navTask = await apiClient.seedTask(seedData.workspaceId, "Automatic colors desktop nav", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await testPage.goto(`/t/${navTask.task_id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  return { task, session };
}

function sidebarTaskRow(session: SessionPage, title: string) {
  return session.sidebar.getByTestId("sidebar-task-item").filter({ hasText: title }).first();
}

async function dragRuleByHandle(
  testPage: import("@playwright/test").Page,
  source: import("@playwright/test").Locator,
  target: import("@playwright/test").Locator,
) {
  await target.scrollIntoViewIfNeeded();
  await source.scrollIntoViewIfNeeded();
  const sourceBox = await source.boundingBox();
  const targetBox = await target.boundingBox();
  const listTop = await target.evaluate(
    (element) =>
      element.closest<HTMLElement>("[data-sidebar-reorder-list]")?.getBoundingClientRect().top,
  );
  expect(sourceBox).not.toBeNull();
  expect(targetBox).not.toBeNull();
  expect(listTop).not.toBeNull();
  const sourceX = sourceBox!.x + sourceBox!.width / 2;
  const sourceY = sourceBox!.y + sourceBox!.height / 2;
  await testPage.mouse.move(sourceX, sourceY);
  await testPage.mouse.down();
  await testPage.mouse.move(sourceX, sourceY + 12, { steps: 4 });
  await expect(testPage.locator('[data-dragging="true"]')).toHaveCount(1);
  await testPage.mouse.move(
    targetBox!.x + targetBox!.width / 2,
    Math.max(targetBox!.y + 2, listTop! + 8),
    { steps: 16 },
  );
  await testPage.mouse.up();
}

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

test.describe("Sidebar automatic task colors", () => {
  test("persists ordered rules and recolors a task when its state changes", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await apiClient.saveUserSettings({ sidebar_task_color_automation: DESKTOP_AUTOMATION });
    const { task, session } = await openDesktopTask(
      testPage,
      apiClient,
      seedData,
      "Desktop automatic color task",
      "FAILED",
    );
    const row = sidebarTaskRow(session, "Desktop automatic color task");
    const marker = row.getByTestId("task-item-color-marker");

    await expect(row).toBeVisible();
    await expect(marker).toHaveAttribute("data-color-token", "red");
    await expect(marker).toHaveClass(/bg-red-500/);

    const filters = new SidebarFilterPopoverPage(testPage);
    await filters.open();
    const disclosureSections = [
      filters.popover.getByTestId("sidebar-sort-settings"),
      filters.popover.getByTestId("sidebar-group-settings"),
      filters.popover.getByTestId("task-row-settings"),
    ];
    const disclosureBottomInsets: number[] = [];
    for (const section of disclosureSections) {
      await expect(section).toHaveCSS("border-bottom-width", "1px");
      disclosureBottomInsets.push(
        await section.evaluate((element) => {
          const toggle = element.querySelector("button");
          if (!toggle) throw new Error("Disclosure toggle not found");
          return element.getBoundingClientRect().bottom - toggle.getBoundingClientRect().bottom;
        }),
      );
    }
    expect(
      Math.max(...disclosureBottomInsets) - Math.min(...disclosureBottomInsets),
    ).toBeLessThanOrEqual(1);

    const automaticSettings = filters.popover.getByTestId("automatic-color-settings");
    await expect(automaticSettings).toBeVisible();
    await automaticSettings.getByTestId("automatic-color-settings-toggle").click();
    await expect(automaticSettings.getByTestId("automatic-color-enabled")).toHaveAttribute(
      "data-state",
      "checked",
    );
    await expect(automaticSettings.getByText("Rule 1 Task state", { exact: true })).toBeVisible();
    await expect(automaticSettings.getByTestId("automatic-colors-timing")).toHaveCount(0);
    await expect(
      automaticSettings.getByText(
        "Personal setting. Applies across sidebar views and workspaces.",
        { exact: true },
      ),
    ).toHaveCount(0);
    await expect(
      automaticSettings.getByText(
        "The first matching rule wins. Manual colors remain as fallback.",
        {
          exact: true,
        },
      ),
    ).toHaveCount(0);

    const dimension = automaticSettings.getByTestId(
      "automatic-color-dimension-desktop-failed-rule",
    );
    await dimension.click();
    for (const label of [
      "Workflow step",
      "Repository",
      "Workflow",
      "Executor profile",
      "Task state",
      "Priority",
      "Origin",
    ]) {
      await expect(testPage.getByRole("option", { name: label, exact: true })).toBeVisible();
    }
    await testPage.keyboard.press("Escape");

    const output = automaticSettings.getByTestId("automatic-color-output-desktop-failed-rule");
    await expect(output.locator("span.inline-block.rounded-full")).toHaveCount(1);

    const timingHelp = automaticSettings.getByTestId("automatic-colors-help");
    await timingHelp.hover();
    const timingTooltip = testPage.getByRole("tooltip");
    await expect(timingTooltip).toContainText(
      "Personal setting. Applies across sidebar views and workspaces.",
    );
    await expect(timingTooltip).toContainText("Rules apply to existing and new sidebar tasks.");
    await expect(timingTooltip).toContainText(
      "The first matching rule wins. Manual colors remain as fallback.",
    );
    await expect(timingTooltip).toContainText(
      "Rules run when tasks appear and whenever their workflow step",
    );

    const scrollMetrics = await filters.popover.evaluate((element) => ({
      clientHeight: element.clientHeight,
      overflowY: getComputedStyle(element).overflowY,
      scrollHeight: element.scrollHeight,
    }));
    expect(scrollMetrics.overflowY).toBe("auto");
    expect(scrollMetrics.scrollHeight).toBeGreaterThan(scrollMetrics.clientHeight);
    await filters.popover.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await expect(automaticSettings.getByTestId("automatic-color-add-rule")).toBeVisible();
    await prCapture.screenshot("desktop-automatic-task-colors", {
      caption: "Desktop automatic task color rules",
    });
    await testPage.mouse.move(0, 0);
    await expect(timingTooltip).toBeHidden();
    await filters.close();

    await apiClient.updateTaskState(task.task_id, "REVIEW");
    await expect(marker).toHaveAttribute("data-color-token", "blue");
    await expect(marker).toHaveClass(/bg-blue-500/);

    await testPage.reload();
    await session.waitForLoad();
    const reloadedMarker = sidebarTaskRow(session, "Desktop automatic color task").getByTestId(
      "task-item-color-marker",
    );
    await expect(reloadedMarker).toHaveAttribute("data-color-token", "blue");
    await expect
      .poll(async () => {
        const { settings } = await apiClient.getUserSettings();
        return settings.sidebar_task_color_automation;
      })
      .toEqual(DESKTOP_AUTOMATION);
  });

  test("aligns a repository target with the condition control", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({
      sidebar_task_color_automation: desktopRepositoryAutomation(seedData.repositoryPath),
    });
    await openDesktopTask(
      testPage,
      apiClient,
      seedData,
      "Desktop repository alignment task",
      "TODO",
    );

    const filters = new SidebarFilterPopoverPage(testPage);
    await filters.open();
    const automaticSettings = filters.popover.getByTestId("automatic-color-settings");
    await automaticSettings.getByTestId("automatic-color-settings-toggle").click();

    const condition = automaticSettings.getByTestId(
      "automatic-color-dimension-desktop-repository-rule",
    );
    const repository = automaticSettings.getByTestId(
      "automatic-color-repository-trigger-desktop-repository-rule",
    );
    await expect(condition).toBeVisible();
    await expect(repository).toBeVisible();

    const [conditionBox, repositoryBox] = await Promise.all([
      condition.boundingBox(),
      repository.boundingBox(),
    ]);
    expect(conditionBox).not.toBeNull();
    expect(repositoryBox).not.toBeNull();
    expect(Math.abs((repositoryBox?.y ?? 0) - (conditionBox?.y ?? 0))).toBeLessThanOrEqual(1);
  });

  test("reorders overlapping rules through More and restores the first match after reload", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const automation: SidebarTaskColorAutomation = {
      enabled: true,
      rules: [
        {
          id: "desktop-overlap-red",
          enabled: true,
          condition: { dimension: "task_state", value: "FAILED", label: "Failed" },
          output: { kind: "fixed", color: "red" },
        },
        {
          id: "desktop-overlap-blue",
          enabled: true,
          condition: { dimension: "task_state", value: "FAILED", label: "Failed" },
          output: { kind: "fixed", color: "blue" },
        },
      ],
    };
    await apiClient.saveUserSettings({ sidebar_task_color_automation: automation });
    const { task, session } = await openDesktopTask(
      testPage,
      apiClient,
      seedData,
      "Desktop overlapping automatic color rules",
      "FAILED",
    );
    const marker = sidebarTaskRow(session, "Desktop overlapping automatic color rules").getByTestId(
      "task-item-color-marker",
    );
    await expect(marker).toHaveAttribute("data-color-token", "red");

    const filters = new SidebarFilterPopoverPage(testPage);
    await filters.open();
    const settings = filters.popover.getByTestId("automatic-color-settings");
    await settings.getByTestId("automatic-color-settings-toggle").click();
    for (const control of [
      "automatic-color-rule-handle-desktop-overlap-red",
      "automatic-color-rule-more-desktop-overlap-red",
      "automatic-color-rule-remove-desktop-overlap-red",
    ]) {
      const box = await settings.getByTestId(control).boundingBox();
      expect(box?.height).toBe(28);
      expect(box?.width).toBe(28);
    }
    if (prCapture.capturing) {
      await filters.popover.evaluate((element) => {
        element.scrollTop = element.scrollHeight;
      });
      await expect(settings.getByTestId("automatic-color-add-rule")).toBeVisible();
      await waitForFiniteAnimations(settings);
      await prCapture.screenshot("automatic-color-rule-order-desktop", {
        caption: "Desktop automatic color rules with reorder handles and move menus",
      });
    }
    await dragRuleByHandle(
      testPage,
      settings.getByTestId("automatic-color-rule-handle-desktop-overlap-blue"),
      settings.getByTestId("automatic-color-rule-desktop-overlap-red"),
    );
    await expect
      .poll(async () => {
        const saved = (await apiClient.getUserSettings()).settings
          .sidebar_task_color_automation as SidebarTaskColorAutomation;
        return saved.rules.map((rule) => rule.id);
      })
      .toEqual(["desktop-overlap-blue", "desktop-overlap-red"]);
    await expect(marker).toHaveAttribute("data-color-token", "blue");
    const more = settings.getByTestId("automatic-color-rule-more-desktop-overlap-blue");
    await more.click();
    await expect(marker).toHaveAttribute("data-color-token", "blue");
    await testPage.getByTestId("automatic-color-rule-more-desktop-overlap-blue-move-down").click();
    await expect(marker).toHaveAttribute("data-color-token", "red");
    await more.click();
    await testPage.getByTestId("automatic-color-rule-more-desktop-overlap-blue-move-up").click();
    await expect(marker).toHaveAttribute("data-color-token", "blue");
    await testPage.goto(`/t/${task.task_id}`);
    await session.waitForLoad();
    await expect(
      sidebarTaskRow(session, "Desktop overlapping automatic color rules").getByTestId(
        "task-item-color-marker",
      ),
    ).toHaveAttribute("data-color-token", "blue");
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.sidebar_task_color_automation)
      .toEqual({ enabled: true, rules: [automation.rules[1], automation.rules[0]] });
  });
});
