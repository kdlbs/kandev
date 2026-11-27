import { expect, test } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";

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

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-006.9 AC-UI-SIDEBAR-CUSTOMIZATION-006.7
test("keeps mobile prompt gestures out of sidebar settings", async ({
  testPage: page,
  apiClient,
  seedData,
}) => {
  const before = await apiClient.getUserSettings();
  const savedLayout = before.settings.sidebar_layouts_by_workspace?.[seedData.workspaceId];
  await apiClient.saveUserSettings({
    sidebar_layout_state: {
      workspace_id: seedData.workspaceId,
      expected_revision: savedLayout?.revision ?? 0,
      layout: null,
    },
  });
  try {
    await page.goto("/tasks");
    await page.getByTestId("app-nav-trigger").tap();
    await page.getByTestId("mobile-new-task-button").tap();
    const dialog = page.getByTestId("create-task-dialog");
    const prompt = dialog.getByTestId("task-description-input");
    const menu = page.getByTestId("sidebar-customize-menu");
    await prompt.tap();
    await expect(prompt).toBeFocused();
    await page.keyboard.insertText("Keep this mobile draft");
    const bounds = (await prompt.boundingBox())!;
    const client = await page.context().newCDPSession(page);
    try {
      await client.send("Input.dispatchTouchEvent", {
        type: "touchStart",
        touchPoints: [{ x: bounds.x + 24, y: bounds.y + 16 }],
      });
      await dwell(page, 800, "library-timer", "Hold beyond Radix's 700 ms context-menu timer");
      await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
      await expect(menu).toBeHidden();
    } finally {
      await client.detach();
    }
    await prompt.dispatchEvent("contextmenu");
    await expect(menu).toBeHidden();
    await expect(prompt).toHaveText("Keep this mobile draft");
    await prompt.fill("Still editable after holding");
    await expect(prompt).toHaveText("Still editable after holding");
    await test.info().attach("mobile-prompt-after-hold", {
      body: await page.screenshot(),
      contentType: "image/png",
    });
    await dialog.getByTestId("submit-cancel").tap();
    await expect(dialog).toBeHidden();
    await page.getByTestId("app-nav-trigger").tap();
    await page.getByTestId("mobile-new-task-button").tap();
    await prompt.tap();
    await expect(prompt).toBeFocused();
    await expect(menu).toBeHidden();
    await dialog.getByTestId("submit-cancel").tap();
    await page.getByTestId("app-nav-trigger").tap();
    await page.getByTestId("mobile-sidebar-customize").tap();
    await expect(page.getByTestId("mobile-sidebar-customization")).toBeVisible();
  } finally {
    const current = await apiClient.getUserSettings();
    await apiClient.saveUserSettings({
      sidebar_layout_state: {
        workspace_id: seedData.workspaceId,
        expected_revision:
          current.settings.sidebar_layouts_by_workspace?.[seedData.workspaceId]?.revision ?? 0,
        layout: savedLayout ?? null,
      },
    });
  }
});
