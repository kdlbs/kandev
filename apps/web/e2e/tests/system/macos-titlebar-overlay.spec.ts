import { expect, test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";

test("macOS Tauri overlay keeps native controls clear in both sidebar states", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 960, height: 640 });
  const longWorkspaceName =
    "A workspace name long enough to verify that the macOS title bar preserves the switcher chevron";
  const originalUserSettings = await apiClient.getUserSettings();

  try {
    await apiClient.saveUserSettings({
      sidebar_hover_enabled: true,
      sidebar_hover_delay_ms: 0,
    });
    await apiClient.updateWorkspace(seedData.workspaceId, { name: longWorkspaceName });
    await testPage.goto("/settings/system/status");

    const shell = testPage.getByTestId("app-shell");
    await expect(shell).toBeVisible();
    await shell.evaluate((element) => element.setAttribute("data-macos-tauri-overlay", "true"));

    const sidebar = testPage.getByTestId("app-sidebar");
    await expect(sidebar).toHaveAttribute("data-collapsed", "false");
    const sidebarHeader = sidebar.getByTestId("app-sidebar-header");
    const workspaceTrigger = sidebar.getByTestId("sidebar-workspace-trigger");
    const workspaceName = workspaceTrigger.locator("span").first();
    const chevron = sidebar.getByTestId("sidebar-workspace-trigger-chevron");
    const collapse = sidebar.getByRole("button", { name: "Collapse sidebar" });

    const expanded = await sidebarHeader.evaluate((element) => ({
      width: element.getBoundingClientRect().width,
      paddingLeft: getComputedStyle(element).paddingLeft,
    }));
    expect(expanded.width).toBeGreaterThanOrEqual(319);
    expect(expanded.width).toBeLessThanOrEqual(320);
    expect(expanded.paddingLeft).toBe("84px");
    await expect(workspaceName).toHaveText(longWorkspaceName);
    await expect(chevron).toBeVisible();
    const [nameMetrics, pickerBox, collapseBox] = await Promise.all([
      workspaceName.evaluate((element) => ({
        clientWidth: element.clientWidth,
        scrollWidth: element.scrollWidth,
        textOverflow: getComputedStyle(element).textOverflow,
      })),
      workspaceTrigger.boundingBox(),
      collapse.boundingBox(),
    ]);
    expect(nameMetrics.textOverflow).toBe("ellipsis");
    expect(nameMetrics.scrollWidth).toBeGreaterThan(nameMetrics.clientWidth);
    expect(pickerBox).not.toBeNull();
    expect(collapseBox).not.toBeNull();
    expect(pickerBox!.x + pickerBox!.width).toBeLessThanOrEqual(collapseBox!.x);

    await workspaceTrigger.click();
    await expect(testPage.getByRole("menu")).toBeVisible();
    await testPage.keyboard.press("Escape");

    await collapse.click();
    await expect(sidebar).toHaveAttribute("data-collapsed", "true");
    await testPage.mouse.move(950, 620);
    await expect(sidebar).toHaveAttribute("data-hover-revealed", "false");
    await waitForFiniteAnimations(sidebar);
    const collapsedHeader = sidebar.getByTestId("app-sidebar-header");
    const [brandBox, expandBox, collapsedHeaderBox] = await Promise.all([
      sidebar.getByRole("link", { name: "Kandev home" }).boundingBox(),
      sidebar.getByRole("button", { name: "Expand sidebar" }).boundingBox(),
      collapsedHeader.boundingBox(),
    ]);
    expect(brandBox).not.toBeNull();
    expect(expandBox).not.toBeNull();
    expect(collapsedHeaderBox).not.toBeNull();
    expect(collapsedHeaderBox!.height).toBeGreaterThanOrEqual(112);
    expect(brandBox!.y).toBeGreaterThanOrEqual(40);
    expect(expandBox!.y).toBeGreaterThanOrEqual(40);

    const contentHeader = testPage
      .locator('[data-window-controls-overlay-region="content"]')
      .first();
    await expect(contentHeader).toBeVisible();
    const contentHeaderStyle = await contentHeader.evaluate((element) => ({
      paddingLeft: getComputedStyle(element).paddingLeft,
      left: element.getBoundingClientRect().left,
    }));
    expect(contentHeaderStyle.paddingLeft).toBe("80px");
    expect(contentHeaderStyle.left).toBe(56);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    await sidebar.getByRole("button", { name: "Expand sidebar" }).click();
    await expect(sidebar).toHaveAttribute("data-collapsed", "false");
    await expect(sidebar.getByTestId("sidebar-workspace-trigger")).toBeVisible();

    await sidebar.getByRole("button", { name: "Collapse sidebar" }).click();
    await expect(sidebar).toHaveAttribute("data-collapsed", "true");
    await testPage.mouse.move(950, 620);
    await sidebar.hover();
    await expect(sidebar).toHaveAttribute("data-hover-revealed", "true");
    await waitForFiniteAnimations(sidebar);
    const hoverHeader = sidebar.getByTestId("app-sidebar-header");
    await expect(hoverHeader).toHaveAttribute("data-sidebar-header-collapsed", "false");
    await expect(sidebar.getByTestId("sidebar-workspace-trigger")).toBeVisible();
    expect(await hoverHeader.evaluate((element) => getComputedStyle(element).paddingLeft)).toBe(
      "84px",
    );
    await testPage.mouse.move(950, 620);
    await expect(sidebar).toHaveAttribute("data-hover-revealed", "false");
  } finally {
    await apiClient.saveUserSettings({
      sidebar_hover_enabled: originalUserSettings.settings.sidebar_hover_enabled,
      sidebar_hover_delay_ms: originalUserSettings.settings.sidebar_hover_delay_ms,
    });
    await apiClient.updateWorkspace(seedData.workspaceId, { name: "E2E Workspace" });
  }
});
