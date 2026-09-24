/**
 * Mobile parity for the `sidebar-workspace-actions` slot. The desktop sidebar
 * is hidden below `md`, so the shared phone navigation sheet must expose the
 * same workspace action with a touch-sized target.
 */
import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

const SLOT_TEST_ID = "e2e-sidebar-workspace-actions";
const TOUCH_TARGET_LAYOUT_TOLERANCE_PX = 0.5;

test.describe("Mobile plugin workspace actions", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("exposes the workspace action in the phone navigation sheet", async ({
    testPage,
    seedData,
  }) => {
    test.setTimeout(60_000);

    await installFixturePlugin(testPage);
    const kanban = new MobileKanbanPage(testPage);
    await kanban.goto();
    await kanban.mobileMenuButton.click();

    const sheet = testPage.getByRole("dialog");
    const slot = sheet.getByTestId(SLOT_TEST_ID);
    await expect(slot).toBeVisible({ timeout: 15_000 });
    await expect(slot).toHaveAttribute("data-workspace-id", seedData.workspaceId);
    await expect(slot).toHaveAttribute("data-presentation", "mobile");

    const box = await slot.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.width).toBeGreaterThanOrEqual(44);
    // 2.75rem is the authored 44px touch target. Chromium can report the
    // computed height just below that integer after rem-to-device-pixel
    // conversion (for example, 43.99993896484375px).
    expect(box!.height).toBeGreaterThanOrEqual(44 - TOUCH_TARGET_LAYOUT_TOLERANCE_PX);

    await slot.tap();
    await expect(slot).toHaveAttribute("data-clicked", "true");
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
  });
});
