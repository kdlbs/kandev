import { test, expect } from "../../fixtures/test-base";
import { assertNoHorizontalOverflow } from "../../helpers/session-stream-overload";
import {
  captureThreadSettings,
  seedThreadPresentation,
  startPresentationThread,
} from "./threads-presentation-helpers";
import {
  associateTerminalPR,
  expectNoticeTargets,
  expectReadableNotice,
} from "./threads-pr-notice-helpers";

// @covers AC-UI-MOBILE-TASK-ACTION-BAR-001.1, .2, .3, .4, .6
test("keeps the phone Threads notice readable and dismissible", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(90_000);
  const original = await captureThreadSettings(apiClient);
  try {
    const task = await startPresentationThread(apiClient, seedData, "Phone terminal notice");
    await associateTerminalPR(apiClient, task.id, "merged");
    await seedThreadPresentation(apiClient, { layout: "columns" });
    await testPage.goto(`/threads?taskId=${task.id}`);
    const tile = testPage.getByTestId(`thread-column-${task.id}`);
    const banner = tile.getByTestId("pr-merged-banner");
    await expect(banner).toContainText("PR #4702 merged");
    await expectReadableNotice(banner);
    await expectNoticeTargets(banner, 44);
    const noticeBox = (await banner.boundingBox())!;
    const barBox = (await tile.getByTestId("chat-status-bar").boundingBox())!;
    expect(noticeBox.width).toBeCloseTo(barBox.width, 0);
    await assertNoHorizontalOverflow(testPage, "phone Threads PR notice");
    await testInfo.attach("phone-notice", {
      body: await tile.screenshot({ path: testInfo.outputPath("phone-notice.png") }),
      contentType: "image/png",
    });
    await banner.getByTestId("pr-merged-dismiss-button").tap();
    await expect(banner).toBeHidden();
    await testPage.reload();
    await expect(tile.getByTestId("chat-status-bar")).toBeVisible();
    await expect(banner).toBeHidden();
  } finally {
    await apiClient.saveUserSettings(original);
  }
});
