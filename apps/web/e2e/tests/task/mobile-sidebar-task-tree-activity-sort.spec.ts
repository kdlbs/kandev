import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { seedTaskTreeActivityScenario } from "./sidebar-task-tree-activity-sort-helpers";

// @covers AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-002.1, .2, .5, .6
test("phone task drawer ranks a parent by its active child and navigates to that child", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const { parent, child, peer, navigationTaskId } = await seedTaskTreeActivityScenario(
    apiClient,
    seedData,
    "Phone tree sort",
  );
  await testPage.goto(`/t/${navigationTaskId}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  await testPage.getByTestId("mobile-task-picker-trigger").tap();
  const sheet = testPage.getByRole("dialog", { name: "Tasks" });
  await expect(sheet).toBeVisible();

  const treeAndPeerIds = [parent.id, peer.id];
  await expect
    .poll(async () => {
      const rows = await sheet
        .locator("[data-task-row-id]")
        .evaluateAll((elements) =>
          elements.map((element) => element.getAttribute("data-task-row-id")),
        );
      return rows.filter((id): id is string => id !== null && treeAndPeerIds.includes(id));
    })
    .toEqual([parent.id, peer.id]);

  const parentTime = await sheet
    .locator(`[data-task-row-id="${parent.id}"]`)
    .getByTestId("sidebar-task-time")
    .getAttribute("data-time-value");
  const childTime = await sheet
    .locator(`[data-task-row-id="${child.id}"]`)
    .getByTestId("sidebar-task-time")
    .getAttribute("data-time-value");
  expect(parentTime).toBeTruthy();
  expect(childTime).toBeTruthy();
  expect(Date.parse(childTime!)).toBeGreaterThan(Date.parse(parentTime!));

  await sheet.locator(`[data-task-row-id="${child.id}"]`).tap();
  await expect(testPage).toHaveURL((url) => url.pathname === `/t/${child.id}`);
});
