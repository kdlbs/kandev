import { test, expect } from "../../fixtures/test-base";
import {
  assertBoundedClientNavigation,
  expectFirstBoardSeed,
  recordWorkflowSnapshots,
  seedDemandBoards,
} from "./journey-review-loading-helpers";

test("client task navigation keeps a large workflow bounded", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const evidence = await assertBoundedClientNavigation(testPage, apiClient, seedData, false);
  await test.info().attach("review-client-navigation.json", {
    body: JSON.stringify(evidence),
    contentType: "application/json",
  });
});

test("all-workflow desktop boards demand only visible or adjacent expanded lanes", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 1280, height: 720 });
  const boards = await seedDemandBoards(apiClient, seedData);
  const last = boards[boards.length - 1];
  await testPage.addInitScript(
    (id) => sessionStorage.setItem("kanban-swimlane-collapse", JSON.stringify({ [id]: true })),
    last.id,
  );
  const requests = recordWorkflowSnapshots(testPage);
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}&workflowId=`);
  const firstId = await expectFirstBoardSeed(testPage);
  const lastLane = testPage.getByTestId(`workflow-lane-${last.id}`);
  await expect(lastLane).toHaveCount(1);
  await expect(lastLane.locator("[data-testid^=task-card-]")).toHaveCount(0);
  expect(requests).not.toContain(last.id);
  const distant = boards[boards.length - 2];
  const distantLane = testPage.getByTestId(`workflow-lane-${distant.id}`);
  const outsidePreload = await distantLane.evaluate((lane) => {
    const root = lane.closest("[data-testid=swimlane-container]");
    return !!root && lane.getBoundingClientRect().top > root.getBoundingClientRect().bottom + 300;
  });
  expect(outsidePreload).toBe(true);
  expect(requests).not.toContain(distant.id);
  expect(new Set(requests).size).toBeLessThan(boards.length);
  await lastLane.scrollIntoViewIfNeeded();
  expect(requests).not.toContain(last.id);
  await lastLane
    .getByTestId("swimlane-header")
    .getByRole("button")
    .filter({ hasText: last.name })
    .click();
  await expect(lastLane.getByTestId(`task-card-${last.task.id}`)).toBeVisible();
  expect(requests).toContain(last.id);
  await test.info().attach("review-board-demand.json", {
    body: JSON.stringify({ firstId, requests, boards: boards.map((board) => board.id) }),
    contentType: "application/json",
  });
});
