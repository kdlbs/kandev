import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { restoreSidebarLayout } from "../../helpers/sidebar-layout";

let baseline: Awaited<ReturnType<ApiClient["getUserSettings"]>>["settings"];
test.beforeEach(async ({ testPage, apiClient }) => {
  void testPage;
  baseline = (await apiClient.getUserSettings()).settings;
});
test.afterEach(async ({ apiClient, seedData }) => {
  await restoreSidebarLayout(
    apiClient,
    seedData.workspaceId,
    baseline.sidebar_layouts_by_workspace?.[seedData.workspaceId],
  );
  await apiClient.saveUserSettings({
    sidebar_fast_actions_enabled: baseline.sidebar_fast_actions_enabled,
    sidebar_new_task_style: baseline.sidebar_new_task_style,
  });
});

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-006.6 AC-UI-SIDEBAR-CUSTOMIZATION-006.7 AC-UI-SIDEBAR-CUSTOMIZATION-007.5
test("customizes the saved sidebar from an inset phone drawer", async ({
  testPage: page,
  apiClient,
  seedData,
}) => {
  const before = await apiClient.getUserSettings();
  await apiClient.saveUserSettings({
    sidebar_fast_actions_enabled: false,
    sidebar_new_task_style: "simple",
    sidebar_layout_state: {
      workspace_id: seedData.workspaceId,
      expected_revision:
        before.settings.sidebar_layouts_by_workspace?.[seedData.workspaceId]?.revision ?? 0,
      layout: {
        version: 1,
        revision: 0,
        navigation_height: 0,
        navigation_expanded: false,
        nodes: [
          { id: "home", kind: "builtin", destination_id: "home", visible: true },
          { id: "new-task", kind: "builtin", destination_id: "new_task", visible: true },
          { id: "integrations", kind: "builtin", destination_id: "integrations", visible: true },
        ],
      },
    },
  });
  await page.goto("/tasks");
  await page.getByTestId("app-nav-trigger").tap();
  await page.getByTestId("mobile-sidebar-customize").tap();
  const drawer = page.getByTestId("mobile-sidebar-customization");
  await expect(drawer).toBeVisible();
  await drawer.getByTestId("mobile-sidebar-visibility-integrations").tap();
  await expect
    .poll(
      async () =>
        (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
          seedData.workspaceId
        ]?.nodes.find((node) => node.id === "integrations")?.visible,
    )
    .toBe(false);
  await drawer.getByRole("button", { name: "Move Home down", exact: true }).tap();
  await expect
    .poll(
      async () =>
        (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
          seedData.workspaceId
        ]?.nodes[0]?.id,
    )
    .not.toBe("home");
  await drawer.getByRole("switch").tap();
  await expect
    .poll(async () => (await apiClient.getUserSettings()).settings.sidebar_fast_actions_enabled)
    .toBe(true);
  const layout = (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
    seedData.workspaceId
  ];
  expect(layout?.navigation_height).toBe(0);
  expect(layout?.navigation_expanded).toBeFalsy();
  const bounds = (await drawer.boundingBox())!;
  expect(bounds.y).toBeGreaterThan(0);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(page.viewportSize()!.width);
  await drawer.getByRole("button", { name: "Close", exact: true }).tap();
  await expect(page.getByTestId("app-nav-sheet")).toBeVisible();
  await page.reload();
  await page.getByTestId("app-nav-trigger").tap();
  await page.getByTestId("mobile-sidebar-customize").tap();
  await expect(drawer.getByTestId("mobile-sidebar-visibility-integrations")).not.toBeChecked();
  await drawer.getByRole("button", { name: "Sidebar layout settings" }).tap();
  await expect(page).toHaveURL(/\/settings\/preferences\/layouts\?tab=sidebar/);
});
