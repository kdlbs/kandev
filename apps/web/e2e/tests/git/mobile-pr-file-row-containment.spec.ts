import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  expandPRChanges,
  expectLongPRRowContained,
  expectNoHorizontalOverflow,
  longPRFileRow,
  seedLongPRFileTask,
  waitForDiffMarker,
} from "./pr-file-row-containment-helpers";

test.describe("Mobile PR file row containment", () => {
  // @covers AC-UI-CHANGES-FILE-ROW-CONTAINMENT-001.1 through .4
  test("keeps long PR paths clear of metadata and opens the diff", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await seedLongPRFileTask(apiClient, seedData, "Mobile PR Row Containment");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    await testPage.getByRole("button", { name: "Changes" }).tap();

    const changes = testPage.getByTestId("mobile-changes-panel");
    await expect(changes).toBeVisible();
    await expandPRChanges(changes, "tap");
    const row = longPRFileRow(changes);

    await expectLongPRRowContained(row);
    await expectNoHorizontalOverflow(row);
    await expectNoHorizontalOverflow(testPage.locator("html"));

    await row.tap();
    await expect(testPage.getByTestId("mobile-diff-sheet-close")).toBeVisible();
    await waitForDiffMarker(testPage);
  });
});
