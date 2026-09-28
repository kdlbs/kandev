import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { seedTaskTreeActivityScenario } from "./sidebar-task-tree-activity-sort-helpers";

// @covers AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-002.1, .2, .5, .6
test("desktop sidebar ranks a parent by its active child and keeps row-local times", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const { parent, child, peer, navigationTaskId } = await seedTaskTreeActivityScenario(
    apiClient,
    seedData,
    "Desktop tree sort",
  );
  await testPage.goto(`/t/${navigationTaskId}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  const treeAndPeerIds = [parent.id, peer.id];
  await expect
    .poll(async () => {
      const rows = await session.sidebar
        .locator("[data-task-row-id]")
        .evaluateAll((elements) =>
          elements.map((element) => element.getAttribute("data-task-row-id")),
        );
      return rows.filter((id): id is string => id !== null && treeAndPeerIds.includes(id));
    })
    .toEqual([parent.id, peer.id]);

  const parentTime = await session.sidebar
    .locator(`[data-task-row-id="${parent.id}"]`)
    .getByTestId("sidebar-task-time")
    .getAttribute("data-time-value");
  const childTime = await session.sidebar
    .locator(`[data-task-row-id="${child.id}"]`)
    .getByTestId("sidebar-task-time")
    .getAttribute("data-time-value");
  expect(parentTime).toBeTruthy();
  expect(childTime).toBeTruthy();
  expect(Date.parse(childTime!)).toBeGreaterThan(Date.parse(parentTime!));
});
