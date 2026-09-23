import { expect, test } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { installFixturePlugin, uninstallFixturePlugin } from "../../helpers/plugin-fixture";

let previousLayout: unknown;
test.beforeEach(async ({ testPage, apiClient, seedData }) => {
  void testPage;
  previousLayout = (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
    seedData.workspaceId
  ];
});
test.afterEach(async ({ apiClient, seedData }) => {
  await uninstallFixturePlugin(apiClient);
  const current = (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
    seedData.workspaceId
  ];
  await apiClient.saveUserSettings({
    sidebar_layout_state: {
      workspace_id: seedData.workspaceId,
      expected_revision: current?.revision ?? 0,
      layout: previousLayout ?? null,
    },
  });
});

type MobileSidebarLayoutNode = {
  id: string;
  kind: "builtin" | "shortcuts";
  visible: boolean;
  destination_id?: string;
  name?: string;
  shortcuts?: Array<{
    id: string;
    target: { kind: "host_action"; id: "quick_chat" | "quick_terminal" };
  }>;
};

async function seedMobileLayout(
  apiClient: ApiClient,
  workspaceId: string,
  homeVisible = true,
): Promise<void> {
  const current = await apiClient.getUserSettings();
  const expectedRevision =
    current.settings.sidebar_layouts_by_workspace?.[workspaceId]?.revision ?? 0;
  await apiClient.saveUserSettings({
    sidebar_layout_state: {
      workspace_id: workspaceId,
      expected_revision: expectedRevision,
      layout: {
        version: 1,
        revision: expectedRevision,
        nodes: [
          { id: "home", kind: "builtin", visible: homeVisible, destination_id: "home" },
          { id: "new-task", kind: "builtin", visible: true, destination_id: "new_task" },
          { id: "automations", kind: "builtin", visible: true, destination_id: "automations" },
          { id: "canvases", kind: "builtin", visible: true, destination_id: "canvases" },
          { id: "integrations", kind: "builtin", visible: true, destination_id: "integrations" },
          {
            id: "group-1",
            kind: "shortcuts",
            visible: true,
            name: "Pinned",
            shortcuts: [
              { id: "chat", target: { kind: "host_action", id: "quick_chat" } },
              { id: "terminal", target: { kind: "host_action", id: "quick_terminal" } },
            ],
          } satisfies MobileSidebarLayoutNode,
        ],
      },
    },
  });
}

async function addMobileShortcut(page: Page, label: string): Promise<void> {
  const picker = page.getByRole("dialog", { name: "Add shortcut" });
  await expect(picker).toBeVisible();
  await picker.getByRole("button", { name: label, exact: true }).click();
}

test.describe("Sidebar customization on phone", () => {
  // @covers AC-UI-SIDEBAR-CUSTOMIZATION-005.5 AC-UI-MOBILE-MENU-007.4
  test("first visibility edit keeps populated Tasks before workspace tools", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await installFixturePlugin(testPage);
    for (const title of [
      "Review checkout accessibility",
      "Add order tracking",
      "Polish receipt emails",
    ]) {
      await apiClient.seedTask(seedData.workspaceId, title, {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      });
    }
    await testPage.goto("/settings/preferences/layouts?tab=sidebar");
    const visibility = testPage.getByTestId("sidebar-layout-node-canvases").getByRole("switch");
    await expect(visibility).toBeChecked();
    await visibility.tap();
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .tap();
    await expect(testPage.getByTestId("settings-floating-save")).toBeHidden();
    await testPage.reload();
    await expect(visibility).not.toBeChecked();
    const saved = (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
      seedData.workspaceId
    ];
    await testPage.goto("/tasks");
    await testPage.getByTestId("app-nav-trigger").tap();
    const menu = testPage.getByTestId("app-nav-sheet");
    const tasks = menu.getByTestId("mobile-navigation-tasks-toggle");
    const automations = menu.getByRole("button", { name: "Automations", exact: true });
    const plugins = menu.getByRole("region", { name: "Plugins", exact: true });
    await expect(plugins).toHaveCount(1);
    await expect(menu.locator("#hello-main-top-bar")).toHaveCount(1);
    await expect(plugins.locator("#hello-main-top-bar")).toBeVisible();
    const workspaceAction = plugins.getByTestId("e2e-sidebar-workspace-actions");
    await expect(workspaceAction).toHaveCount(1);
    await expect(menu.getByRole("link", { name: "Hello E2E", exact: true })).toHaveCount(1);
    await expect(menu.getByRole("button", { name: "Canvases", exact: true })).toHaveCount(0);
    await expect(menu.locator("[data-task-row-id]")).toHaveCount(3);
    for (const width of [360, 393, 767]) {
      await testPage.setViewportSize({ width, height: 851 });
      expect((await tasks.boundingBox())!.y).toBeLessThan((await automations.boundingBox())!.y);
      expect((await automations.boundingBox())!.y).toBeLessThan((await plugins.boundingBox())!.y);
      expect(await menu.locator("nav").evaluate((el) => getComputedStyle(el).overflowY)).toBe(
        "auto",
      );
      expect(
        await menu
          .getByTestId("mobile-task-switcher-list")
          .evaluate((el) => getComputedStyle(el).overflowY),
      ).toBe("visible");
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      ).toBe(true);
      const box = (await menu.boundingBox())!;
      expect(box.x).toBeGreaterThanOrEqual(0);
      expect(box.x + box.width).toBeLessThanOrEqual(width);
      expect(box.height).toBeLessThanOrEqual(851);
    }
    await workspaceAction.tap();
    await expect(workspaceAction).toHaveAttribute("data-clicked", "true");
    await expect(menu).toBeVisible();
    await automations.tap();
    await expect(menu.getByRole("link", { name: "Set up an automation" })).toBeVisible();
    await menu.getByRole("button", { name: "Integrations", exact: true }).tap();
    const setup = menu.getByTestId("mobile-integration-settings");
    await setup.scrollIntoViewIfNeeded();
    const control = (await setup.boundingBox())!;
    const surface = (await menu.boundingBox())!;
    expect(control.height).toBeGreaterThanOrEqual(44);
    expect(control.x).toBeGreaterThanOrEqual(surface.x);
    expect(control.x + control.width).toBeLessThanOrEqual(surface.x + surface.width);
    await setup.tap();
    await expect(testPage).toHaveURL(
      new RegExp(`/settings/workspaces/${seedData.workspaceId}/integrations`),
    );
    expect(
      (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
        seedData.workspaceId
      ],
    ).toEqual(saved);
  });

  test("uses the focused editor flow and preserves touch-sized reorder controls", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedMobileLayout(apiClient, seedData.workspaceId);
    await testPage.goto("/settings/preferences/layouts?tab=sidebar");
    await expect(testPage.getByRole("tab", { name: "Sidebar", exact: true })).toBeVisible();

    const group = testPage.getByTestId("sidebar-layout-node-group-1");
    await group.getByRole("button", { name: "Edit section", exact: true }).tap();
    const focused = testPage.getByTestId("sidebar-layout-focused-group");
    await expect(focused).toBeVisible();

    const moveUp = focused.getByRole("button", { name: "Move up" });
    const moveDown = focused.getByRole("button", { name: "Move down" });
    await expect(moveUp).toHaveCount(2);
    await expect(moveDown).toHaveCount(2);
    for (const button of [moveUp.nth(0), moveDown.nth(0), moveUp.nth(1), moveDown.nth(1)]) {
      const box = await button.boundingBox();
      expect(box?.height).toBeGreaterThanOrEqual(44);
    }

    await focused.getByRole("button", { name: "Add shortcut", exact: true }).tap();
    await addMobileShortcut(testPage, "New Task");
    await focused.getByRole("button", { name: "Add shortcut", exact: true }).tap();
    await addMobileShortcut(testPage, "Stats");

    const saveButton = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" });
    await saveButton.tap();
    await expect
      .poll(
        async () =>
          (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
            seedData.workspaceId
          ]?.revision,
      )
      .toBeGreaterThan(0);

    await testPage.reload();
    await expect(testPage.getByTestId("sidebar-layout-editor")).toBeVisible();
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  });
});

for (const homeVisible of [true, false]) {
  test(`saved phone layout preserves unified task access with Home visible=${homeVisible}`, async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedMobileLayout(apiClient, seedData.workspaceId, homeVisible);
    await apiClient.mockGitHubSetUser("navigation-demo");
    const task = await apiClient.seedTask(seedData.workspaceId, "Polish checkout summary", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto("/tasks");
    await testPage.getByTestId("app-nav-trigger").tap();
    const menu = testPage.getByTestId("app-nav-sheet");
    const layout = menu.getByTestId("mobile-sidebar-layout-navigation");
    await expect(layout).toBeVisible();
    const chat = layout.getByTestId("mobile-quick-chat-button");
    await expect(chat).toBeVisible();
    await expect(layout.getByTestId("mobile-quick-terminal-button")).toBeVisible();
    const home = layout.getByRole("link", { name: "Home", exact: true });
    await expect(home).toHaveCount(homeVisible ? 1 : 0);
    if (homeVisible) {
      await expect(home).toHaveAttribute("aria-current", "page");
      const order = await layout
        .locator("a, button")
        .evaluateAll((nodes) =>
          nodes.map((node) => node.getAttribute("data-testid") ?? node.textContent?.trim()),
        );
      expect(order.indexOf("mobile-quick-chat-button")).toBe(order.indexOf("Home") + 1);
    }
    await expect(menu.getByRole("link", { name: /^(Tasks|Threads)$/ })).toHaveCount(0);
    await expect(layout.getByRole("button", { name: "New Task", exact: true })).toHaveCount(0);
    const tasks = menu.getByTestId("mobile-navigation-tasks-toggle");
    await expect(tasks).toBeVisible();
    const automations = menu.getByRole("button", { name: "Automations", exact: true });
    // @covers AC-UI-MOBILE-MENU-007.2 AC-UI-SIDEBAR-CUSTOMIZATION-005.5
    expect((await tasks.boundingBox())!.y).toBeLessThan((await automations.boundingBox())!.y);
    const integrations = layout.getByRole("button", { name: "Integrations", exact: true });
    const github = layout.getByRole("link", { name: "GitHub", exact: true });
    await expect(integrations).toHaveAttribute("aria-expanded", "false");
    await expect(github).toBeHidden();
    await integrations.tap();
    await expect(integrations).toHaveAttribute("aria-expanded", "true");
    await expect(github).toHaveText("GitHub");
    await expect(menu.getByTestId("mobile-integration-settings")).toBeVisible();
    await expect(menu.getByTestId("mobile-customize-sidebar-button")).toBeVisible();
    await menu.getByText("Polish checkout summary", { exact: true }).tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.task_id}`));
  });
}

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-005.5
test("saved canvas disclosure recovers a failed read and retains settings", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  const release = await backend.useEnv({ KANDEV_FEATURES_CANVASES: "true" });
  try {
    await seedMobileLayout(apiClient, seedData.workspaceId);
    let unavailable = true;
    await testPage.route("**/api/v1/workspaces/*/canvases", (route) =>
      unavailable
        ? route.fulfill({ status: 503, json: { error: "unavailable" } })
        : route.continue(),
    );
    await testPage.goto("/tasks");
    await testPage.getByTestId("app-nav-trigger").tap();
    const menu = testPage.getByTestId("app-nav-sheet");
    const toggle = menu.getByRole("button", { name: "Canvases", exact: true });
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await toggle.tap();
    await expect(menu.getByRole("alert")).toContainText("Request failed");
    await expect(menu.getByRole("link", { name: "Open canvas settings" })).toBeVisible();
    unavailable = false;
    await menu.getByRole("button", { name: "Try again", exact: true }).tap();
    await expect(menu.getByRole("alert")).toHaveCount(0);
    await menu.getByRole("link", { name: "Open canvas settings" }).tap();
    await expect(testPage).toHaveURL(
      new RegExp(`/settings/workspaces/${seedData.workspaceId}/canvases`),
    );
    await testPage.goto("/settings/preferences/layouts?tab=sidebar");
    await testPage.getByTestId("sidebar-layout-node-canvases").getByRole("switch").tap();
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .tap();
    await expect(testPage.getByTestId("settings-floating-save")).toBeHidden();
    await testPage.goto("/tasks");
    await testPage.getByTestId("app-nav-trigger").tap();
    await expect(menu.getByRole("button", { name: "Canvases", exact: true })).toHaveCount(0);
    await expect(menu.getByRole("link", { name: "Open canvas settings" })).toHaveCount(0);
  } finally {
    await release();
  }
});
