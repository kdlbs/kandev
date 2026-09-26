import { test, expect } from "../../fixtures/test-base";

/**
 * Same control, phone presentation: the shared dropdown gets the global
 * inset bottom-sheet treatment below 640px, so the rows are reachable with a
 * thumb instead of being a desktop popover squeezed against the header.
 */
test.describe("Mobile workspace settings switcher", () => {
  test("opens as a bottom sheet and switches workspace on the same tab", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const other = await apiClient.createWorkspace("Mobile Switcher Workspace");

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
    await testPage.getByTestId("workspace-settings-switcher").click();

    const menu = testPage.locator('[data-slot="dropdown-menu-content"]:visible');
    const activeRow = menu.getByTestId(`workspace-settings-switcher-item-${seedData.workspaceId}`);
    const targetRow = menu.getByTestId(`workspace-settings-switcher-item-${other.id}`);
    await expect(activeRow).toBeVisible();
    await expect(activeRow).toContainText("Active");
    await menu.evaluate((element) =>
      Promise.all(element.getAnimations({ subtree: true }).map((animation) => animation.finished)),
    );

    const [menuBox, rowBox, viewport] = await Promise.all([
      menu.boundingBox(),
      targetRow.boundingBox(),
      testPage.evaluate(() => ({ width: window.innerWidth, height: window.innerHeight })),
    ]);
    if (!menuBox || !rowBox) throw new Error("mobile workspace switcher has no layout box");
    expect(menuBox.x).toBeGreaterThanOrEqual(7);
    expect(menuBox.x).toBeLessThanOrEqual(10);
    expect(menuBox.width).toBeGreaterThanOrEqual(viewport.width - 20);
    expect(viewport.height - (menuBox.y + menuBox.height)).toBeGreaterThanOrEqual(7);
    expect(viewport.height - (menuBox.y + menuBox.height)).toBeLessThanOrEqual(10);
    expect(rowBox.height).toBeGreaterThanOrEqual(36);

    await targetRow.tap();

    await expect(testPage).toHaveURL(new RegExp(`/settings/workspaces/${other.id}/repositories$`));
    await expect(testPage.getByTestId("workspace-settings-switcher")).toContainText(
      "Mobile Switcher Workspace",
    );
  });
});
