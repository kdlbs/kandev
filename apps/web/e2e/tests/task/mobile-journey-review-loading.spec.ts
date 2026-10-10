import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import {
  assertBoundedClientNavigation,
  expectFirstBoardSeed,
  recordWorkflowSnapshots,
  seedDemandBoards,
} from "./journey-review-loading-helpers";

test("phone task picker navigation keeps a large workflow bounded", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const evidence = await assertBoundedClientNavigation(testPage, apiClient, seedData, true);
  await test.info().attach("review-client-navigation.json", {
    body: JSON.stringify(evidence),
    contentType: "application/json",
  });
});

test("all-workflow phone reads follow the focused board", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const boards = await seedDemandBoards(apiClient, seedData);
  const last = boards[boards.length - 1];
  await testPage.addInitScript(
    (id) => sessionStorage.setItem("kanban-swimlane-collapse", JSON.stringify({ [id]: true })),
    last.id,
  );
  const requests = recordWorkflowSnapshots(testPage);
  await testPage.goto(`/?workspaceId=${seedData.workspaceId}&workflowId=`);
  const firstId = await expectFirstBoardSeed(testPage);
  const mobile = new MobileKanbanPage(testPage);
  await expect(mobile.mobileKanbanLayout()).toBeVisible();
  expect(requests.every((id) => id === firstId)).toBe(true);
  await mobile.boardNavigator.tap();
  await mobile.workflowItem(last.id).tap();
  await expect(mobile.taskCard(last.task.id)).toBeVisible();
  expect(requests).toContain(last.id);
  expect(requests.every((id) => id === firstId || id === last.id)).toBe(true);
  await test.info().attach("review-board-demand.json", {
    body: JSON.stringify({ firstId, requests, boards: boards.map((board) => board.id) }),
    contentType: "application/json",
  });
});
