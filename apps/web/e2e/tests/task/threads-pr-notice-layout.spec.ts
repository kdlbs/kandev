import { test, expect } from "../../fixtures/test-base";
import {
  captureThreadSettings,
  seedThreadPresentation,
  startPresentationThread,
} from "./threads-presentation-helpers";
import {
  associateTerminalPR,
  expectNoticeTargets,
  expectReadableNotice,
  setDesktopTileWidth,
} from "./threads-pr-notice-helpers";

for (const layout of ["columns", "grid"] as const) {
  for (const state of ["merged", "closed"] as const) {
    // @covers AC-UI-MOBILE-TASK-ACTION-BAR-001.4, .6, .7, .9
    test(`keeps ${state} PR text readable in narrow desktop ${layout}`, async ({
      testPage,
      apiClient,
      seedData,
    }, testInfo) => {
      test.setTimeout(180_000);
      const original = await captureThreadSettings(apiClient);
      const { settings } = await apiClient.getUserSettings();
      try {
        await apiClient.saveUserSettings({
          show_transcript_auto_scroll_control: true,
          show_scroll_to_start: true,
          show_scroll_to_last_prompt: true,
        });
        const response = Array.from({ length: 60 }, (_, i) => `Transcript line ${i + 1}`).join(
          "\n\n",
        );
        const task = await startPresentationThread(
          apiClient,
          seedData,
          "A terminal notice",
          `e2e:message(${JSON.stringify(response)})`,
        );
        await startPresentationThread(apiClient, seedData, "B adjacent conversation");
        await associateTerminalPR(apiClient, task.id, state);
        await seedThreadPresentation(apiClient, { layout });
        await testPage.setViewportSize({ width: 1440, height: 1200 });
        await testPage.goto(`/threads?taskId=${task.id}`);
        const board = testPage.getByTestId("threads-board");
        const tile = testPage.getByTestId(`thread-column-${task.id}`);
        const bar = tile.getByTestId("chat-status-bar");
        const banner = tile.getByTestId(`pr-${state}-banner`);
        await expect(board).toHaveAttribute("data-layout", layout);
        await expect(bar.getByTestId("proceed-next-step")).toBeVisible();
        await expect(bar.getByTestId("auto-scroll-toggle-button")).toBeVisible();
        await expect(bar.getByTestId("share-task-button")).toBeVisible();
        const actionGroups = [
          bar.getByTestId("chat-status-bar-right-controls"),
          bar.getByTestId("proceed-next-step"),
        ];
        const columns = layout === "columns" ? 2 : 1;
        for (const width of [360, 384, 480]) {
          await setDesktopTileWidth(board, tile, width, columns);
          await expectReadableNotice(banner);
          await expectNoticeTargets(banner, 24);
          const noticeBox = (await banner.boundingBox())!;
          const barBox = (await bar.boundingBox())!;
          for (const group of actionGroups) {
            const actionsBox = (await group.boundingBox())!;
            expect(
              actionsBox.y >= noticeBox.y + noticeBox.height ||
                actionsBox.x >= noticeBox.x + noticeBox.width,
              "actions must fit beside or below the readable notice",
            ).toBe(true);
            expect(actionsBox.x + actionsBox.width).toBeLessThanOrEqual(barBox.x + barBox.width);
          }
        }
        await testInfo.attach("narrow-desktop-notice", {
          body: await tile.screenshot({ path: testInfo.outputPath("narrow-desktop-notice.png") }),
          contentType: "image/png",
        });
        await testPage.setViewportSize({ width: 2400, height: 1200 });
        await setDesktopTileWidth(board, tile, 900, columns);
        await expectReadableNotice(banner);
        const notice = (await banner.boundingBox())!;
        for (const group of actionGroups) {
          const actions = (await group.boundingBox())!;
          expect(actions.y + actions.height / 2).toBeCloseTo(notice.y + notice.height / 2, 0);
        }
        await banner.getByTestId(`pr-${state}-archive-button`).click();
        await expect(testPage.getByTestId(`pr-${state}-archive-confirm`)).toBeEnabled();
        await testPage.getByRole("button", { name: "Cancel", exact: true }).click();
        await expectReadableNotice(banner);
        await banner.getByTestId(`pr-${state}-dismiss-button`).click();
        await expect(banner).toBeHidden();
        await expect(bar.getByTestId("proceed-next-step")).toBeVisible();
      } finally {
        await apiClient.saveUserSettings({
          ...original,
          show_transcript_auto_scroll_control: settings.show_transcript_auto_scroll_control,
          show_scroll_to_start: settings.show_scroll_to_start,
          show_scroll_to_last_prompt: settings.show_scroll_to_last_prompt,
        });
      }
    });
  }
}

// @covers AC-UI-MOBILE-TASK-ACTION-BAR-001.7, .9
test("contains a translated closed notice in a narrow desktop tile", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(90_000);
  const original = await captureThreadSettings(apiClient);
  try {
    const task = await startPresentationThread(apiClient, seedData, "Translated terminal notice");
    await associateTerminalPR(apiClient, task.id, "closed");
    await seedThreadPresentation(apiClient, { layout: "columns" });
    await testPage
      .context()
      .addCookies([{ name: "kandev_locale", value: "pt-pt", url: backend.baseUrl }]);
    await testPage.setViewportSize({ width: 1440, height: 1000 });
    await testPage.goto(`/threads?taskId=${task.id}`);
    const tile = testPage.getByTestId(`thread-column-${task.id}`);
    const banner = tile.getByTestId("pr-closed-banner");
    await expect(banner).toContainText("fechado sem integração");
    await setDesktopTileWidth(testPage.getByTestId("threads-board"), tile, 360, 1);
    await expectReadableNotice(banner);
    await expectNoticeTargets(banner, 24);
  } finally {
    await apiClient.saveUserSettings(original);
  }
});
