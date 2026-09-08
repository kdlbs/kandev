import { test, expect } from "../../fixtures/test-base";
import { resizeColumnViaSplitview } from "../../helpers/dockview-resize";
import { SessionPage } from "../../pages/session-page";
import {
  expandPRChanges,
  expectLongPRRowContained,
  expectNoHorizontalOverflow,
  longPRFileRow,
  seedLongPRFileTask,
  waitForDiffMarker,
} from "./pr-file-row-containment-helpers";

test.describe("PR file row containment", () => {
  // @covers AC-UI-CHANGES-FILE-ROW-CONTAINMENT-001.1 through .3
  test("keeps long PR paths clear of metadata at the desktop panel minimum", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 1366, height: 900 });
    const task = await seedLongPRFileTask(apiClient, seedData, "Desktop PR Row Containment");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForDockviewReady();
    await session.clickTab("Changes");
    await expandPRChanges(session.changes, "click");

    const row = longPRFileRow(session.changes);
    await expect(row).toBeVisible();
    const width = await resizeColumnViaSplitview(testPage, "right", 180);
    expect(width).toBe(180);
    await expect
      .poll(async () => Math.round((await session.changes.boundingBox())?.width ?? 0))
      .toBe(180);

    await expectLongPRRowContained(row);
    await expectNoHorizontalOverflow(row);
    await expectNoHorizontalOverflow(testPage.locator("html"));

    await row.click();
    await waitForDiffMarker(testPage);
  });
});
