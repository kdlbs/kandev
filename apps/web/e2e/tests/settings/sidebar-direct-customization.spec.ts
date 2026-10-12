import { expect, test } from "../../fixtures/test-base";

test.beforeEach(async ({ apiClient, seedData }) => {
  const current = await apiClient.getUserSettings();
  await apiClient.saveUserSettings({
    sidebar_fast_actions_enabled: false,
    sidebar_new_task_style: "simple",
    sidebar_layout_state: {
      workspace_id: seedData.workspaceId,
      expected_revision:
        current.settings.sidebar_layouts_by_workspace?.[seedData.workspaceId]?.revision ?? 0,
      layout: null,
    },
  });
});

// @covers AC-UI-NAV-HIERARCHY-004.1 AC-UI-NAV-HIERARCHY-004.4 AC-UI-NAV-HIERARCHY-004.5 AC-UI-SIDEBAR-CUSTOMIZATION-006.1 AC-UI-SIDEBAR-CUSTOMIZATION-006.2 AC-UI-SIDEBAR-CUSTOMIZATION-006.3
test("customizes visibility and presentation from the menu and settings", async ({
  testPage: page,
  apiClient,
}) => {
  await page.goto("/tasks");
  const sidebar = page.getByTestId("app-sidebar");
  const create = sidebar.getByTestId("create-task-button");
  await expect(create).toBeVisible();
  expect((await create.boundingBox())!.height).toBe(28);
  await expect(sidebar.getByTestId("sidebar-labelled-utilities")).toBeVisible();
  await sidebar.getByRole("link", { name: "Home", exact: true }).click({ button: "right" });
  const menu = page.getByTestId("sidebar-customize-menu");
  await expect(menu.getByText("Sidebar settings", { exact: true })).toBeVisible();
  await menu.getByTestId("sidebar-visibility-home").click();
  await expect(sidebar.getByRole("link", { name: "Home", exact: true })).toHaveCount(0);
  await page.reload();
  await expect(sidebar.getByRole("link", { name: "Home", exact: true })).toHaveCount(0);
  await create.click({ button: "right" });
  await expect(menu.getByTestId("sidebar-visibility-home")).toHaveAttribute(
    "data-state",
    "unchecked",
  );
  await menu.getByRole("menuitemcheckbox", { name: "Show fast action icons" }).click();
  await expect(sidebar.getByTestId("sidebar-quick-actions")).toBeVisible();
  await expect(sidebar.getByTestId("sidebar-labelled-utilities")).toHaveCount(0);
  await create.click({ button: "right" });
  await menu.getByRole("menuitemradio", { name: "Compact New Task row" }).click();
  await expect
    .poll(async () => (await apiClient.getUserSettings()).settings.sidebar_new_task_style)
    .toBe("compact");
  await expect.poll(async () => (await create.boundingBox())?.height).toBe(36);
  await page.reload();
  await expect(sidebar.getByTestId("sidebar-quick-actions")).toBeVisible();
  await expect.poll(async () => (await create.boundingBox())?.height).toBe(36);
  await create.click({ button: "right" });
  await menu.getByRole("menuitem", { name: "Sidebar layout settings" }).click();
  await expect(page).toHaveURL(/\/settings\/preferences\/layouts\?tab=sidebar/);
  await expect(page.getByTestId("sidebar-fast-actions-setting")).toBeChecked();
  await expect(page.getByTestId("sidebar-new-task-style-setting")).toHaveText(
    "Compact New Task row",
  );
  await page.getByTestId("sidebar-fast-actions-setting").click();
  await expect(page.getByTestId("settings-floating-save")).toBeVisible();
  await page
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Save changes" })
    .click();
  await expect(page.getByTestId("settings-floating-save")).toBeHidden();
  await page.goto("/tasks");
  await expect(sidebar.getByTestId("sidebar-labelled-utilities")).toBeVisible();
});

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-006.4 AC-UI-SIDEBAR-CUSTOMIZATION-006.5
test("drags entries without a handle and persists their order", async ({
  testPage: page,
  apiClient,
  seedData,
}) => {
  await page.goto("/tasks");
  const home = page.getByTestId("sidebar-node-home");
  const integrations = page.getByTestId("sidebar-node-integrations");
  const source = (await home.boundingBox())!;
  const destination = (await integrations.boundingBox())!;
  await page.mouse.move(source.x + 80, source.y + source.height / 2);
  await page.mouse.down();
  await page.mouse.move(destination.x + 80, destination.y + destination.height / 2, { steps: 12 });
  await expect
    .poll(() => page.evaluate(() => getComputedStyle(document.body).cursor))
    .toBe("grabbing");
  await page.mouse.up();
  await expect
    .poll(async () => {
      const nodes =
        (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
          seedData.workspaceId
        ]?.nodes ?? [];
      return (
        nodes.findIndex((node) => node.id === "home") >
        nodes.findIndex((node) => node.id === "integrations")
      );
    })
    .toBe(true);
  await page.reload();
  expect((await home.boundingBox())!.y).toBeGreaterThan((await integrations.boundingBox())!.y);
  expect(await page.evaluate(() => getComputedStyle(document.body).cursor)).not.toBe("grabbing");
  await home.getByRole("link").focus();
  await page.keyboard.press("Alt+ArrowUp");
  await expect
    .poll(async () => {
      const nodes =
        (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
          seedData.workspaceId
        ]?.nodes ?? [];
      return (
        nodes.findIndex((node) => node.id === "home") <
        nodes.findIndex((node) => node.id === "integrations")
      );
    })
    .toBe(true);
});

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-007.1 AC-UI-SIDEBAR-CUSTOMIZATION-007.2 AC-UI-SIDEBAR-CUSTOMIZATION-007.3 AC-UI-SIDEBAR-CUSTOMIZATION-007.4
test("saves the split and expanded state, showing a small chevron and fade", async ({
  testPage: page,
  apiClient,
  seedData,
}) => {
  await page.goto("/tasks");
  const divider = page.getByTestId("sidebar-navigation-divider");
  await expect(divider).toBeVisible();
  const box = (await divider.boundingBox())!;
  await page.mouse.move(box.x + 5, box.y + 5);
  await page.mouse.down();
  const navigation = (await page.locator("#sidebar-navigation-content").boundingBox())!;
  await page.mouse.move(box.x + 5, navigation.y + 95, { steps: 10 });
  await page.mouse.up();
  await expect
    .poll(
      async () =>
        (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
          seedData.workspaceId
        ]?.navigation_height,
    )
    .toBe(90);
  await expect(page.getByTestId("sidebar-navigation-fade")).toBeVisible();
  expect((await divider.boundingBox())!.height).toBe(12);
  const chevron = page.getByTestId("sidebar-navigation-expand");
  await chevron.click();
  await expect(chevron).toHaveAttribute("aria-expanded", "true");
  await expect(page.getByTestId("sidebar-node-integrations")).toBeVisible();
  const expandedY = (await divider.boundingBox())!.y;
  await page.reload();
  await expect(chevron).toHaveAttribute("aria-expanded", "true");
  await chevron.click();
  await expect(chevron).toHaveAttribute("aria-expanded", "false");
  expect((await divider.boundingBox())!.y).toBeLessThan(expandedY);
  await page.reload();
  await expect(chevron).toHaveAttribute("aria-expanded", "false");
  await divider.focus();
  await page.keyboard.press("End");
  await expect(page.getByTestId("sidebar-navigation-fade")).toHaveCount(0);
});

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-007.1 AC-UI-SIDEBAR-CUSTOMIZATION-007.4
test("cancels resizing and clamps a short viewport without rewriting the saved height", async ({
  testPage: page,
  apiClient,
  seedData,
}) => {
  await page.goto("/tasks");
  const divider = page.getByTestId("sidebar-navigation-divider");
  await expect(divider).toBeVisible();
  const before = (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
    seedData.workspaceId
  ];
  const box = (await divider.boundingBox())!;
  await page.mouse.move(box.x + 5, box.y + 5);
  await page.mouse.down();
  await page.mouse.move(box.x + 5, box.y - 100, { steps: 10 });
  await page.keyboard.press("Escape");
  await page.mouse.up();
  expect(
    (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
      seedData.workspaceId
    ]?.revision,
  ).toBe(before?.revision);
  await divider.focus();
  await page.keyboard.press("End");
  await expect
    .poll(
      async () =>
        (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
          seedData.workspaceId
        ]?.navigation_height ?? 0,
    )
    .toBeGreaterThan(200);
  const stored = (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
    seedData.workspaceId
  ];
  const tallY = (await divider.boundingBox())!.y;
  await page.setViewportSize({ width: 1280, height: 400 });
  await expect.poll(async () => (await divider.boundingBox())!.y).toBeLessThan(tallY);
  const shortY = (await divider.boundingBox())!.y;
  await expect(page.getByTestId("sidebar-navigation-fade")).toBeVisible();
  expect(
    (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
      seedData.workspaceId
    ]?.navigation_height,
  ).toBe(stored?.navigation_height);
  await page.setViewportSize({ width: 1280, height: 900 });
  await expect.poll(async () => (await divider.boundingBox())!.y).toBeGreaterThan(shortY);
});

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-006.1 AC-UI-SIDEBAR-CUSTOMIZATION-006.7
test("keeps the blank customization region reachable when every entry is hidden", async ({
  testPage: page,
  apiClient,
  seedData,
}) => {
  const layout = (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace![
    seedData.workspaceId
  ];
  await apiClient.saveUserSettings({
    sidebar_layout_state: {
      workspace_id: seedData.workspaceId,
      expected_revision: layout.revision,
      layout: { ...layout, nodes: layout.nodes.map((node) => ({ ...node, visible: false })) },
    },
  });
  await page.goto("/tasks");
  const region = page.getByTestId("sidebar-customize-region");
  await expect(region).toBeVisible();
  await region.click({ button: "right" });
  const menu = page.getByTestId("sidebar-customize-menu");
  await expect(menu.getByTestId("sidebar-visibility-home")).toHaveAttribute(
    "data-state",
    "unchecked",
  );
  await menu.getByTestId("sidebar-visibility-home").click();
  await expect(
    page.getByTestId("app-sidebar").getByRole("link", { name: "Home", exact: true }),
  ).toBeVisible();
});

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-007.1 AC-UI-SIDEBAR-CUSTOMIZATION-007.3
test("collapses every navigation entry and restores zero after expansion and reload", async ({
  testPage: page,
  apiClient,
  seedData,
}) => {
  await page.goto("/tasks");
  const divider = page.getByTestId("sidebar-navigation-divider");
  await expect(divider).toBeVisible();
  const box = (await divider.boundingBox())!;
  await page.mouse.move(box.x + 5, box.y + 5);
  await page.mouse.down();
  await page.mouse.move(box.x + 5, 0, { steps: 10 });
  await page.mouse.up();
  const height = async () =>
    (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
      seedData.workspaceId
    ]?.navigation_height;
  await expect.poll(height).toBe(0);
  await expect(divider).toHaveAttribute("aria-valuenow", "0");
  await expect(page.locator("#sidebar-navigation-content")).toHaveCSS("height", "0px");
  await expect(page.getByTestId("sidebar-node-home")).toHaveAttribute("inert", "");
  await page.reload();
  await expect(divider).toHaveAttribute("aria-valuenow", "0");
  const chevron = page.getByTestId("sidebar-navigation-expand");
  await chevron.click();
  await expect(chevron).toHaveAttribute("aria-expanded", "true");
  await expect(page.getByTestId("sidebar-node-home")).toBeVisible();
  await expect.poll(height).toBe(0);
  await chevron.click();
  await expect(divider).toHaveAttribute("aria-valuenow", "0");
  await page.reload();
  await expect(divider).toHaveAttribute("aria-valuenow", "0");
  await chevron.click();
  await expect(chevron).toHaveAttribute("aria-expanded", "true");
  await divider.focus();
  await page.keyboard.press("Home");
  await expect(divider).toHaveAttribute("aria-valuenow", "0");
  await expect.poll(height).toBe(0);
});
