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

test.describe("Plugin action UX, composer", () => {
  let statusBarBaseline: AppStatusBarSettingsBaseline | undefined;

  test.afterEach(async ({ apiClient }) => {
    await uninstallFixturePlugin(apiClient);
    await restoreAppStatusBarSettings(apiClient, statusBarBaseline);
    statusBarBaseline = undefined;
  });

  test("standard plugin actions share native composer geometry and group spacing", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Plugin action composer geometry",
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
    const actions = chat.locator('[data-slot="surface-action"][data-testid^="e2e-"]');
    const attach = chat.getByTestId("chat-attachments-button");
    await expect(actions).toHaveCount(2);
    await expect(attach).toBeVisible();
    await assertGlyphSize(actions.nth(0), 16);

    const [firstBox, secondBox, attachBox, groupGap] = await Promise.all([
      actions.nth(0).boundingBox(),
      actions.nth(1).boundingBox(),
      attach.boundingBox(),
      actions
        .first()
        .evaluate((element) =>
          Number.parseFloat(getComputedStyle(element.parentElement!).columnGap),
        ),
    ]);
    expect(firstBox).not.toBeNull();
    expect(secondBox).not.toBeNull();
    expect(attachBox).not.toBeNull();
    expect(firstBox!.height).toBeCloseTo(attachBox!.height, 1);
    expect(firstBox!.width).toBeCloseTo(firstBox!.height, 1);
    expect(secondBox!.height).toBeCloseTo(firstBox!.height, 1);
    expect(groupGap).toBeCloseTo(2, 1);
  });

  test("chrome: topbar and sidebar actions share native control geometry", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);
    await testPage.goto("/tasks");

    const mainAction = testPage.getByTestId("e2e-main-topbar-action");
    await expect(mainAction).toBeVisible();
    await expect(mainAction).toHaveAttribute("data-surface", "topbar");
    await expect(mainAction).toHaveAccessibleName("Fixture workspace topbar action");
    const mainBox = await mainAction.boundingBox();
    expect(mainBox).not.toBeNull();
    expect(mainBox!.width).toBeCloseTo(28, 0);
    expect(mainBox!.height).toBeCloseTo(28, 0);
    const mainChrome = await mainAction.evaluate((element) => {
      const style = getComputedStyle(element);
      const glyph = element.querySelector<HTMLElement>('[data-slot="surface-action-icon"]');
      const glyphBox = glyph?.getBoundingClientRect();
      return {
        borderWidth: style.borderTopWidth,
        borderStyle: style.borderTopStyle,
        radius: style.borderTopLeftRadius,
        padding: style.padding,
        glyphWidth: glyphBox?.width,
        glyphHeight: glyphBox?.height,
        groupGap: getComputedStyle(element.parentElement!).columnGap,
        groupFlexWrap: getComputedStyle(element.parentElement!).flexWrap,
      };
    });
    expect(mainChrome).toMatchObject({
      borderWidth: "1px",
      borderStyle: "solid",
      radius: "6px",
      padding: "0px",
      glyphWidth: 16,
      glyphHeight: 16,
      groupGap: "4px",
      groupFlexWrap: "nowrap",
    });
    await assertGlyphSize(mainAction, 16);
    await expect(mainAction.getByTestId("e2e-component-action-glyph")).toHaveCount(1);

    const sidebarNative = testPage.getByTestId("sidebar-quick-terminal-shortcut");
    const sidebarAction = testPage.getByTestId("e2e-sidebar-standard-action");
    await expect(sidebarAction).toBeVisible();
    await expect(sidebarAction).toHaveAccessibleName("Fixture sidebar workspace action");
    const [nativeSidebarBox, pluginSidebarBox] = await Promise.all([
      sidebarNative.boundingBox(),
      sidebarAction.boundingBox(),
    ]);
    expect(nativeSidebarBox).not.toBeNull();
    expect(pluginSidebarBox).not.toBeNull();
    expect(pluginSidebarBox!.width).toBeCloseTo(nativeSidebarBox!.width, 0);
    expect(pluginSidebarBox!.height).toBeCloseTo(nativeSidebarBox!.height, 0);
    const [nativeSidebarGlyph, pluginSidebarGlyph, sidebarGroupGap] = await Promise.all([
      sidebarNative.locator("svg").boundingBox(),
      sidebarAction.locator('[data-slot="surface-action-icon"]').boundingBox(),
      sidebarAction.evaluate((element) => getComputedStyle(element.parentElement!).columnGap),
    ]);
    expect(nativeSidebarGlyph).not.toBeNull();
    expect(pluginSidebarGlyph).not.toBeNull();
    expect(pluginSidebarGlyph!.width).toBeCloseTo(nativeSidebarGlyph!.width, 0);
    expect(pluginSidebarGlyph!.height).toBeCloseTo(nativeSidebarGlyph!.height, 0);
    expect(sidebarGroupGap).toBe("2px");
    await assertGlyphSize(sidebarAction, 14);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Plugin topbar action geometry",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await openTaskSession(testPage, task.id);
    const taskAction = testPage.getByTestId("e2e-chat-top-bar-standard-action");
    const taskNative = testPage.getByTestId("task-right-panels-toggle");
    await expect(taskAction).toBeVisible();
    await expect(taskNative).toBeVisible();
    const [pluginTaskBox, nativeTaskBox] = await Promise.all([
      taskAction.boundingBox(),
      taskNative.boundingBox(),
    ]);
    expect(pluginTaskBox).not.toBeNull();
    expect(nativeTaskBox).not.toBeNull();
    expect(pluginTaskBox!.width).toBeCloseTo(nativeTaskBox!.width, 0);
    expect(pluginTaskBox!.height).toBeCloseTo(nativeTaskBox!.height, 0);
    const taskChrome = await taskNative.evaluate((element) => {
      const style = getComputedStyle(element);
      const glyph = element.querySelector<HTMLElement>('[data-slot="surface-action-icon"]');
      const glyphBox = glyph?.getBoundingClientRect();
      return {
        borderWidth: style.borderTopWidth,
        borderStyle: style.borderTopStyle,
        radius: style.borderTopLeftRadius,
        padding: style.padding,
        glyphWidth: glyphBox?.width,
        glyphHeight: glyphBox?.height,
      };
    });
    expect(taskChrome).toMatchObject({
      borderWidth: mainChrome.borderWidth,
      borderStyle: mainChrome.borderStyle,
      radius: mainChrome.radius,
      padding: mainChrome.padding,
      glyphWidth: mainChrome.glyphWidth,
      glyphHeight: mainChrome.glyphHeight,
    });
    await assertGlyphSize(taskAction, 16);
  });

  test("status: inline Actions fit the 24px bar and keep their ordering identity", async ({
    testPage,
    apiClient,
  }) => {
    statusBarBaseline = await captureAppStatusBarSettings(apiClient);
    await installFixturePlugin(testPage);
    await setAppStatusBarEnabled(apiClient, true);
    await testPage.goto("/tasks");

    const bar = testPage.getByTestId("app-status-bar");
    await expect(bar).toBeVisible();
    const leftId = `plugin:kandev-plugin-e2e:app-status-bar-left:0`;
    const leftContribution = bar.locator(`[data-status-item-id="${leftId}"]`);
    const actions = leftContribution.locator('[data-slot="surface-action"]');
    const group = leftContribution.locator('[data-slot="surface-action-group"]');
    await expect(actions).toHaveCount(2);
    await expect(group).toHaveCount(1);
    const action = leftContribution.getByTestId("e2e-status-bar-action");
    await expect(action).toHaveAttribute("data-surface", "status-bar");
    await expect(action).toHaveAccessibleName("Fixture service status");

    const [barBox, actionBox, glyphBox, groupGap] = await Promise.all([
      bar.boundingBox(),
      action.boundingBox(),
      action.locator('[data-slot="surface-action-icon"]').boundingBox(),
      action.evaluate((element) => getComputedStyle(element.parentElement!).columnGap),
    ]);
    expect(barBox).not.toBeNull();
    expect(actionBox).not.toBeNull();
    expect(glyphBox).not.toBeNull();
    expect(barBox!.height).toBe(24);
    expect(actionBox!.height).toBe(24);
    expect(glyphBox!.width).toBe(12);
    expect(glyphBox!.height).toBe(12);
    await assertGlyphSize(action, 12);
    expect(groupGap).toBe("2px");

    await actions.locator('[data-slot="surface-action-text"]').evaluateAll((elements) => {
      elements.forEach((element) => {
        element.textContent = "A deliberately long status value ".repeat(8);
      });
    });
    const groupLayout = await group.evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      const style = getComputedStyle(element);
      const actionBounds = Array.from(element.children, (child) => {
        const rect = child.getBoundingClientRect();
        return {
          x: rect.x,
          right: rect.right,
          y: rect.y,
          bottom: rect.bottom,
          height: rect.height,
        };
      });
      return {
        x: bounds.x,
        right: bounds.right,
        y: bounds.y,
        bottom: bounds.bottom,
        maxWidth: style.maxWidth,
        flexWrap: style.flexWrap,
        actionFlexShrink: Array.from(
          element.children,
          (child) => getComputedStyle(child).flexShrink,
        ),
        actionBounds,
      };
    });
    expect(groupLayout.maxWidth).toBe("288px");
    expect(groupLayout.flexWrap).toBe("nowrap");
    expect(groupLayout.actionFlexShrink).toEqual(["1", "1"]);
    for (const box of groupLayout.actionBounds) {
      expect(box.x).toBeGreaterThanOrEqual(groupLayout.x - 1);
      expect(box.right).toBeLessThanOrEqual(groupLayout.right + 1);
      expect(box.y).toBeGreaterThanOrEqual(groupLayout.y - 1);
      expect(box.bottom).toBeLessThanOrEqual(groupLayout.bottom + 1);
      expect(box.height).toBeCloseTo(24, 0);
    }
    await action.click();
    await expect(action).toHaveAttribute("aria-pressed", "true");

    await testPage.reload();
    const reloaded = testPage
      .getByTestId("app-status-bar")
      .locator(`[data-status-item-id="${leftId}"]`);
    await expect(reloaded.getByTestId("e2e-status-bar-action")).toBeVisible();
    await expect(reloaded).toHaveAttribute("data-status-item-id", leftId);
  });
});
