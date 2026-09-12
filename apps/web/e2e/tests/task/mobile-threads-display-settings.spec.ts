import { test, expect } from "../../fixtures/test-base";
import { assertNoHorizontalOverflow } from "../../helpers/session-stream-overload";
import { waitForFiniteAnimations } from "../../helpers/animations";
import {
  capturePresentation,
  captureThreadSettings,
  seedThreadPresentation,
  startPresentationThread,
} from "./threads-presentation-helpers";

// @covers AC-UI-THREADS-SAVED-VIEWS-005.5, AC-UI-THREADS-SAVED-VIEWS-005.6, AC-UI-THREADS-DECK-004.6
test("edits wider-screen preferences in one touch drawer and keeps the phone single-chat", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(180_000);
  const original = await captureThreadSettings(apiClient);
  try {
    await startPresentationThread(testPage, apiClient, seedData, "A phone display");
    await startPresentationThread(testPage, apiClient, seedData, "B phone display");
    await seedThreadPresentation(apiClient, { layout: "columns" });
    await testPage.goto("/threads");
    await testPage.getByTestId("threads-mobile-view-trigger").tap();
    await testPage.getByTestId("threads-mobile-view-settings").tap();
    const drawer = testPage.getByTestId("threads-mobile-view-drawer");
    await expect(drawer).toHaveCount(1);
    const layout = drawer.getByTestId("threads-layout-select");
    await layout.scrollIntoViewIfNeeded();
    expect((await layout.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await layout.tap();
    await testPage.getByRole("option", { name: "Grid", exact: true }).tap();
    await expect(drawer.getByTestId("threads-phone-layout-hint")).toBeVisible();
    const toggle = drawer.getByRole("switch", { name: "Auto-hide composer" });
    await toggle.tap();
    expect(
      (await drawer.getByTestId("threads-auto-hide-row").boundingBox())!.height,
    ).toBeGreaterThanOrEqual(44);
    await expect(drawer.getByTestId("threads-touch-composer-hint")).toBeVisible();
    await drawer.getByLabel("Maximum chats").fill("2");
    const save = drawer.getByTestId("threads-view-save");
    await save.scrollIntoViewIfNeeded();
    expect((await save.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await save.tap();
    await expect
      .poll(async () => (await captureThreadSettings(apiClient)).thread_view_draft)
      .toBeNull();
    expect((await captureThreadSettings(apiClient)).thread_views[0]).toMatchObject({
      layout: "grid",
      auto_hide_composer: true,
      max_columns: 2,
    });
    await capturePresentation(testPage, testInfo, "phone-display-settings");
    await testPage.reload();
    const board = testPage.getByTestId("threads-board");
    await expect(board).toHaveAttribute("data-layout", "columns");
    await expect(board.getByTestId("session-chat")).toHaveCount(1);
    await expect(board.getByTestId("chat-input-editor")).toBeVisible();
    await expect(testPage.getByTestId("threads-layout-shortcut")).toHaveCount(0);
    await testPage.setViewportSize({ width: 1200, height: 1100 });
    await expect(board).toHaveAttribute("data-layout", "grid");
    await expect(board.getByTestId("session-chat")).toHaveCount(2);
    await testPage.getByTestId("threads-mobile-view-trigger").tap();
    await testPage.getByTestId("threads-mobile-view-settings").tap();
    await expect(drawer).toHaveCount(1);
    await expect(layout).toHaveText("Grid");
    await expect(testPage.getByTestId("threads-layout-shortcut")).toHaveCount(0);
    await assertNoHorizontalOverflow(testPage);

    await testPage.evaluate(() => {
      document.cookie = "kandev_locale=pseudo; path=/; max-age=31536000; SameSite=Lax";
      localStorage.setItem("theme", "dark");
    });
    await testPage.setViewportSize({ width: 320, height: 740 });
    await testPage.reload();
    await testPage.getByTestId("threads-mobile-view-trigger").tap();
    await testPage.getByTestId("threads-mobile-view-settings").tap();
    await expect(layout).toContainText(/[À-ž]/);
    await drawer.getByTestId("threads-max-columns").scrollIntoViewIfNeeded();
    await waitForFiniteAnimations(drawer);
    await capturePresentation(testPage, testInfo, "phone-display-pseudo-dark-320");
    await assertNoHorizontalOverflow(testPage);
    const scroll = drawer.getByTestId("threads-mobile-view-drawer-scroll-region");
    expect(await scroll.evaluate((el) => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1);
  } finally {
    await apiClient.saveUserSettings(original);
  }
});
