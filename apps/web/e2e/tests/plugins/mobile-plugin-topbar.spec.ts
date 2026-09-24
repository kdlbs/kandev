import path from "node:path";
import { waitForFiniteAnimations } from "../../helpers/animations";
import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import {
  captureAppStatusBarSettings,
  restoreAppStatusBarSettings,
  type AppStatusBarSettingsBaseline,
} from "../../helpers/app-status-bar-settings";
import { assertNoDocumentHorizontalOverflow, requireBox } from "../../helpers/layout-assertions";
import { openTaskSession } from "../../helpers/session";

const PLUGIN_ID = "kandev-plugin-e2e";
const PACKAGE_PATH = path.resolve(
  __dirname,
  "../../../../../apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz",
);

type SystemMetricsDisplayBaseline = {
  show_in_topbar: boolean;
  simplified?: boolean;
};

async function installFixture(page: Page) {
  await page.goto("/settings/plugins");
  await page.getByTestId("install-plugin-trigger").tap();
  await page.getByTestId("install-plugin-tab-upload").tap();
  await page.getByTestId("install-plugin-file-input").setInputFiles(PACKAGE_PATH);
  await page.getByTestId("install-plugin-upload-submit").tap();
  await expect(page.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible({ timeout: 30_000 });
}

test.describe("Mobile plugin menu actions", () => {
  let metricsBaseline: SystemMetricsDisplayBaseline;
  let statusBarBaseline: AppStatusBarSettingsBaseline;
  let createdTaskId: string | undefined;

  test.beforeEach(async ({ apiClient, testPage }) => {
    void testPage;
    createdTaskId = undefined;
    const settings = await apiClient.getUserSettings();
    metricsBaseline = (settings.settings.system_metrics_display as
      | SystemMetricsDisplayBaseline
      | undefined) ?? { show_in_topbar: false };
    statusBarBaseline = await captureAppStatusBarSettings(apiClient);

    const response = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      app_status_bar_enabled: false,
      system_metrics_display: { show_in_topbar: true, simplified: false },
    });
    expect(response.ok).toBe(true);
  });

  test.afterEach(async ({ apiClient }) => {
    if (createdTaskId) await apiClient.deleteTask(createdTaskId).catch(() => undefined);
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      system_metrics_display: metricsBaseline,
    });
    await restoreAppStatusBarSettings(apiClient, statusBarBaseline);
  });

  test("keeps plugins, opted-in metrics, and native tools accessible in every mode", async ({
    testPage,
  }) => {
    test.setTimeout(120_000);
    await installFixture(testPage);
    for (const [route, pluginPage] of [
      ["/?home=overview", "kanban"],
      ["/tasks", "tasks"],
      ["/threads", "kanban"],
    ]) {
      await testPage.goto(route);
      await expect(testPage.getByTestId("mobile-topbar-action-strip")).toHaveCount(0);
      await expect(testPage.getByTestId("app-status-metrics")).toHaveCount(0);
      await testPage.getByTestId("app-nav-trigger").tap();
      const menu = testPage.getByRole("dialog", { name: "Menu", exact: true });
      const plugin = menu.locator("#hello-main-top-bar");
      const metrics = menu.getByTestId("app-status-metrics");
      await expect(plugin).toHaveAccessibleName(`Hello ${pluginPage}`);
      expect(
        await plugin.evaluate((element) =>
          Boolean(element.closest('[data-testid="mobile-plugin-nav-section"]')),
        ),
        "Workspace controls belong to the labeled Plugins section",
      ).toBe(true);
      expect(
        await menu
          .getByTestId("e2e-sidebar-workspace-actions")
          .evaluate((element) =>
            Boolean(element.closest('[data-testid="mobile-plugin-nav-section"]')),
          ),
        "Sidebar plugin controls belong to the same Plugins section",
      ).toBe(true);
      await expect(metrics).toBeVisible();
      await expect(metrics.getByLabel(/^CPU /)).toBeVisible();
      await waitForFiniteAnimations(menu);
      const pluginSection = menu.getByTestId("mobile-plugin-nav-section");
      const pluginBox = await requireBox(pluginSection, "Plugins section");
      const metricsBox = await requireBox(metrics, "system metrics");
      expect(metricsBox.y).toBeGreaterThanOrEqual(pluginBox.y + pluginBox.height);
      await expect(pluginSection.getByRole("heading", { name: "Task", exact: true })).toHaveCount(
        0,
      );
      for (const target of [
        plugin,
        menu.getByTestId("mobile-quick-chat-button"),
        menu.getByTestId("mobile-quick-terminal-button"),
      ]) {
        const box = await requireBox(target, "menu action");
        expect(box.height).toBeGreaterThanOrEqual(44);
        expect(box.width).toBeGreaterThanOrEqual(44);
        expect(box.x + box.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
      }
      const icon = await requireBox(plugin.locator("svg").first(), "plugin icon");
      expect(icon.width).toBeCloseTo(16, 0);
      expect(icon.height).toBeCloseTo(16, 0);
      await assertNoDocumentHorizontalOverflow(testPage, `menu tools on ${route}`);
      await expect(menu.getByRole("textbox")).toHaveCount(0);
      await testPage.keyboard.press("Escape");
      if (route !== "/threads") {
        await testPage.getByTestId("mobile-topbar-page-context").tap();
        const options = testPage.getByRole("dialog", { name: "View options", exact: true });
        await options.getByTestId("mobile-search-toggle").tap();
        const search = testPage.getByTestId("mobile-search-bar");
        await expect(menu).toHaveCount(0);
        await expect(search.getByRole("textbox")).toBeFocused();
        await assertNoDocumentHorizontalOverflow(testPage, "phone search");
      }
      await expect(testPage.getByTestId("app-status-metrics")).toHaveCount(0);
    }
  });

  // @covers AC-UI-MOBILE-TASK-CHROME-001.7
  test("moves session contributions from a long task header into the Plugins section", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    await installFixture(testPage);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Move plug-in controls into the mobile menu without crowding",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    createdTaskId = task.id;
    if (!task.session_id) throw new Error("Fixture task did not create a session");
    await openTaskSession(testPage, task.id);

    const fixedActions = testPage.getByTestId("mobile-topbar-actions");
    const title = testPage.getByTestId("mobile-task-picker-trigger");
    const menuTrigger = testPage.getByTestId("app-nav-trigger");
    await expect(fixedActions.getByTestId("e2e-chat-top-bar-status")).toHaveCount(0);
    await expect(fixedActions.getByTestId("e2e-chat-top-bar-action")).toHaveCount(0);
    await expect(title).toContainText("Move plug-in controls");
    const titleBox = await requireBox(title, "mobile task title");
    const actionsBox = await requireBox(fixedActions, "mobile task actions");
    expect(titleBox.width).toBeGreaterThanOrEqual(120);
    expect(titleBox.x + titleBox.width).toBeLessThanOrEqual(actionsBox.x);
    const triggerBox = await requireBox(menuTrigger, "mobile menu trigger");
    expect(triggerBox.width).toBeGreaterThanOrEqual(44);
    expect(triggerBox.height).toBeGreaterThanOrEqual(44);
    await assertNoDocumentHorizontalOverflow(testPage, "task header with plugin contributions");
    await menuTrigger.tap();
    const menu = testPage.getByRole("dialog", { name: "Menu", exact: true });
    const pluginSection = menu.getByTestId("mobile-plugin-nav-section");
    const status = pluginSection.getByTestId("e2e-chat-top-bar-status");
    const action = pluginSection.getByTestId("e2e-chat-top-bar-action");
    await expect(pluginSection).toBeVisible();
    await pluginSection.scrollIntoViewIfNeeded();
    await waitForFiniteAnimations(menu);
    await expect(menu.getByText("Plugins", { exact: true })).toHaveCount(1);
    await expect(pluginSection.getByRole("heading", { name: "Workspace" })).toBeVisible();
    await expect(pluginSection.getByRole("heading", { name: "Task", exact: true })).toBeVisible();
    await expect(pluginSection.locator("#hello-main-top-bar")).toHaveCount(1);
    const workspaceAction = pluginSection.getByTestId("e2e-sidebar-workspace-actions");
    await expect(workspaceAction).toHaveCount(1);
    await workspaceAction.tap();
    await expect(workspaceAction).toHaveAttribute("data-clicked", "true");
    await expect(menu.locator("nav.overflow-y-auto")).toHaveCount(1);
    const metrics = menu.getByTestId("app-status-metrics");
    await expect(metrics.getByLabel(/^CPU /)).toBeVisible();
    const metricsBox = await requireBox(metrics, "system metrics");
    const pluginBox = await requireBox(pluginSection, "Plugins section");
    expect(metricsBox.y).toBeGreaterThanOrEqual(pluginBox.y + pluginBox.height);
    await expect(status).toHaveAttribute("data-task-id", task.id);
    await expect(status).toHaveAttribute("data-workspace-id", seedData.workspaceId);
    await expect(status).toHaveAttribute("data-active-session-id", task.session_id);
    await expect(status).toHaveAttribute("data-session-ids", task.session_id);
    await expect(status).toHaveAttribute("data-presentation", "mobile");
    await expect(action).toHaveAttribute("data-presentation", "mobile");
    for (const contribution of [status, action]) {
      const box = await requireBox(contribution, "session plugin contribution");
      const menuBox = await requireBox(menu, "mobile menu");
      expect(box.x).toBeGreaterThanOrEqual(menuBox.x);
      expect(box.x + box.width).toBeLessThanOrEqual(menuBox.x + menuBox.width);
    }
    const actionBox = await requireBox(action, "session plugin action");
    expect(actionBox.height).toBeGreaterThanOrEqual(44);
    expect(actionBox.width).toBeGreaterThanOrEqual(44);
    await action.tap();
    await expect(action).toHaveAttribute("data-activated", "true");
    await expect(menu).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "task plugin menu");
    for (const width of [393, 320, 767]) {
      await testPage.setViewportSize({ width, height: 851 });
      await pluginSection.evaluate((element) => element.scrollIntoView({ block: "start" }));
      const sectionBox = await requireBox(pluginSection, "Plugins section");
      for (const control of [
        action,
        workspaceAction,
        pluginSection.locator("#hello-main-top-bar"),
      ]) {
        const box = await requireBox(control, "plugin control");
        expect(box.height).toBeGreaterThanOrEqual(44);
        expect(box.width).toBeGreaterThanOrEqual(44);
        expect(box.x).toBeGreaterThanOrEqual(sectionBox.x);
        expect(box.x + box.width).toBeLessThanOrEqual(sectionBox.x + sectionBox.width);
      }
      await assertNoDocumentHorizontalOverflow(testPage, `plugin menu at ${width}px`);
      if (width === 393) {
        for (const colorScheme of ["dark", "light"] as const) {
          await testPage.emulateMedia({ colorScheme });
          await waitForFiniteAnimations(menu);
          await prCapture.screenshot(`mobile-task-plugin-menu-${colorScheme}`, {
            caption: `Workspace and task plugins grouped together, followed by resources (${colorScheme})`,
          });
        }
      }
    }

    await testPage.keyboard.press("Escape");
    await expect(menu).toBeHidden();
    await expect(menuTrigger).toBeFocused();

    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      app_status_bar_enabled: true,
    });
    await testPage.reload();
    await testPage.getByTestId("app-nav-trigger").tap();
    await expect(menu.getByTestId("mobile-plugin-nav-section")).toBeVisible();
    await expect(menu.getByTestId("app-status-metrics")).toHaveCount(0);
    if (prCapture.capturing) {
      await testPage.setViewportSize({ width: 393, height: 851 });
      await testPage.goto("/?home=overview");
      await testPage.getByTestId("app-nav-trigger").tap();
      const listingPlugins = menu.getByTestId("mobile-plugin-nav-section");
      await expect(listingPlugins).toBeVisible();
      await listingPlugins.evaluate((element) => element.scrollIntoView({ block: "start" }));
      await waitForFiniteAnimations(menu);
      await prCapture.screenshot("mobile-listing-plugin-menu", {
        caption:
          "Listing menu with a single plugin group; resources are available through Status when enabled",
      });
    }
  });
});
