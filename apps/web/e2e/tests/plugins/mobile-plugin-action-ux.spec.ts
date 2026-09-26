import { expect, test } from "../../fixtures/test-base";
import type { Locator } from "@playwright/test";
import { installFixturePlugin, uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { SessionPage } from "../../pages/session-page";
import { openTaskSession } from "../../helpers/session";
import {
  captureAppStatusBarSettings,
  restoreAppStatusBarSettings,
  setAppStatusBarEnabled,
  type AppStatusBarSettingsBaseline,
} from "../../helpers/app-status-bar-settings";

async function assertGlyphSize(action: Locator, size: number): Promise<void> {
  const svg = action.locator('[data-slot="surface-action-icon"] > svg');
  await expect(svg).toHaveCount(1);
  const box = await svg.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.width).toBeCloseTo(size, 0);
  expect(box!.height).toBeCloseTo(size, 0);
}

test.describe("Plugin action UX, composer on phone", () => {
  let statusBarBaseline: AppStatusBarSettingsBaseline | undefined;

  test.afterEach(async ({ apiClient }) => {
    await uninstallFixturePlugin(apiClient);
    await restoreAppStatusBarSettings(apiClient, statusBarBaseline);
    statusBarBaseline = undefined;
  });

  test("keeps actions beside the composer with touch targets and working busy stop", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Plugin action phone composer",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.composerReady();
    const chat = session.activeChat();
    const action = chat.getByTestId("e2e-standard-composer-action");
    const busyStop = chat.getByTestId("e2e-busy-stop-action");
    const attach = chat.getByTestId("chat-attachments-button");
    await expect(action).toBeVisible();
    await expect(busyStop).toBeEnabled();
    await assertGlyphSize(action, 16);
    const [actionBox, stopBox, attachBox] = await Promise.all([
      action.boundingBox(),
      busyStop.boundingBox(),
      attach.boundingBox(),
    ]);
    expect(actionBox).not.toBeNull();
    expect(stopBox).not.toBeNull();
    expect(attachBox).not.toBeNull();
    for (const box of [actionBox!, stopBox!, attachBox!]) {
      expect(box.height).toBeGreaterThanOrEqual(44);
    }
    expect(actionBox!.width).toBeGreaterThanOrEqual(44);
    expect(actionBox!.width).toBeCloseTo(actionBox!.height, 1);

    await action.tap();
    await expect(action).toHaveAttribute("aria-pressed", "true");
    await busyStop.tap();
    await expect(chat.getByTestId("e2e-composer-action")).toHaveAttribute(
      "data-status",
      "busy-stop-activated",
    );
    await expect
      .poll(() =>
        testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      )
      .toBe(true);
  });

  test("chrome: topbar and workspace actions remain touch-sized in the Plugins section", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Plugin phone toolbar actions",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await openTaskSession(testPage, task.id);
    await testPage.getByTestId("app-nav-trigger").tap();

    const section = testPage.getByTestId("mobile-plugin-nav-section");
    const taskAction = section.getByTestId("e2e-chat-top-bar-standard-action");
    const workspaceAction = section.getByTestId("e2e-sidebar-standard-action");
    await expect(taskAction).toBeVisible();
    await expect(workspaceAction).toBeVisible();
    await expect(taskAction).toHaveAttribute("data-surface", "topbar");
    await expect(workspaceAction).toHaveAttribute("data-surface", "sidebar");

    for (const action of [taskAction, workspaceAction]) {
      const box = await action.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.width).toBeGreaterThanOrEqual(44);
      await action.tap();
      await expect(action).toHaveAttribute("aria-pressed", "true");
    }
    const taskGroup = taskAction.locator('xpath=ancestor::*[@data-slot="surface-action-group"][1]');
    const taskActions = taskGroup.locator('[data-slot="surface-action"]');
    await expect(taskActions).toHaveCount(4);
    const sectionBox = await section.boundingBox();
    expect(sectionBox).not.toBeNull();
    const topbarRows = new Set<number>();
    for (const action of await taskActions.all()) {
      const box = await action.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.x).toBeGreaterThanOrEqual(sectionBox!.x - 1);
      expect(box!.y).toBeGreaterThanOrEqual(sectionBox!.y - 1);
      expect(box!.x + box!.width).toBeLessThanOrEqual(sectionBox!.x + sectionBox!.width + 1);
      expect(box!.y + box!.height).toBeLessThanOrEqual(sectionBox!.y + sectionBox!.height + 1);
      topbarRows.add(Math.round(box!.y));
      await action.tap();
    }
    expect(topbarRows.size).toBeGreaterThan(1);
    await assertGlyphSize(taskAction, 16);
    await assertGlyphSize(workspaceAction, 14);
    await expect(testPage.getByRole("tooltip")).toHaveCount(0);
    await expect
      .poll(() =>
        testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      )
      .toBe(true);

    await testPage.setViewportSize({ width: 1200, height: 900 });
    await testPage.goto("/tasks");
    expect(await testPage.evaluate(() => window.matchMedia("(pointer: coarse)").matches)).toBe(
      true,
    );
    const wideTopbarAction = testPage.getByTestId("e2e-main-topbar-action");
    await expect(wideTopbarAction).toBeVisible();
    const wideTopbarGroup = wideTopbarAction.locator(
      'xpath=ancestor::*[@data-slot="surface-action-group"][1]',
    );
    const wideGroupLayout = await wideTopbarGroup.evaluate((element) => {
      const style = getComputedStyle(element);
      return { flexWrap: style.flexWrap, maxWidth: style.maxWidth };
    });
    expect(wideGroupLayout).toEqual({ flexWrap: "wrap", maxWidth: "100%" });
    const wideSidebarNative = testPage.getByTestId("sidebar-quick-terminal-shortcut");
    const wideSidebarAction = testPage.getByTestId("e2e-sidebar-standard-action");
    await expect(wideSidebarAction).toBeVisible();
    for (const action of [wideSidebarNative, wideSidebarAction]) {
      const box = await action.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.width).toBeGreaterThanOrEqual(44);
    }

    await testPage.setViewportSize({ width: 800, height: 900 });
    await testPage.goto("/tasks");
    const tabletNativeAction = testPage.getByTestId("tablet-quick-terminal-button");
    const tabletPluginAction = testPage.getByTestId("e2e-main-topbar-action");
    await expect(tabletNativeAction).toBeVisible();
    await expect(tabletPluginAction).toBeVisible();
    await expect(tabletNativeAction).toHaveAttribute("data-slot", "surface-action");
    for (const action of [tabletNativeAction, tabletPluginAction]) {
      const box = await action.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.width).toBeGreaterThanOrEqual(44);
    }
  });

  test("status: plugin actions use touch-sized native Status drawer rows", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(90_000);
    statusBarBaseline = await captureAppStatusBarSettings(apiClient);
    await installFixturePlugin(testPage);
    await setAppStatusBarEnabled(apiClient, true);

    await testPage.goto("/");
    await testPage.getByTestId("app-nav-trigger").tap();
    const statusEntry = testPage.getByTestId("mobile-home-status-button");
    await expect(statusEntry).toBeVisible();
    await statusEntry.tap();

    const drawer = testPage.getByTestId("app-status-drawer");
    await expect(drawer).toBeVisible();
    const pluginRow = drawer.locator(
      '[data-status-item-id="plugin:kandev-plugin-e2e:app-status-bar-left:0"]',
    );
    const action = pluginRow.getByTestId("e2e-status-drawer-action");
    const busyAction = pluginRow.getByTestId("e2e-status-busy-action");
    const disabledAction = pluginRow.getByTestId("e2e-status-disabled-action");
    await expect(action).toHaveAttribute("data-surface", "status-drawer");
    await expect(action).toBeEnabled();
    await expect(busyAction).toHaveAttribute("aria-busy", "true");
    await expect(busyAction).toBeEnabled();
    await expect(disabledAction).toBeDisabled();
    await assertGlyphSize(action, 16);

    for (const control of [action, busyAction, disabledAction]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }

    await action.tap();
    await expect(action).toHaveAttribute("aria-pressed", "true");
    await busyAction.tap();
    await expect(testPage.getByRole("tooltip")).toHaveCount(0);
  });
});
