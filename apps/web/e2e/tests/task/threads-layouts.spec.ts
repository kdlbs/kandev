import { test, expect } from "../../fixtures/test-base";
import { attachGatewayTrafficCapture } from "../../helpers/ws-traffic";
import {
  captureThreadSettings,
  capturePresentation,
  expectTwoRows,
  seedThreadPresentation,
  startPresentationThread,
  threadBoxes,
  type ThreadPresentationSettings,
} from "./threads-presentation-helpers";

let original: ThreadPresentationSettings;
test.beforeEach(async ({ apiClient, testPage }) => {
  void testPage;
  original = await captureThreadSettings(apiClient);
});

// @covers AC-UI-THREADS-DECK-004.5, AC-UI-THREADS-DECK-004.6, AC-UI-THREADS-DECK-004.8
test("bounds both grid rows to visible streams and opens a lower-row deep link", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(300_000);
  const tasks = [];
  for (const title of ["A", "B", "C", "D", "E", "F", "G", "H"]) {
    tasks.push(
      await startPresentationThread(testPage, apiClient, seedData, `${title} windowed grid`),
    );
  }
  await seedThreadPresentation(apiClient, { layout: "grid", maxChats: 8 });
  await testPage.setViewportSize({ width: 1100, height: 1100 });
  const capture = attachGatewayTrafficCapture(testPage);
  await testPage.goto(`/threads?taskId=${tasks[7].id}`);
  const board = testPage.getByTestId("threads-board");
  const target = testPage.getByTestId(`thread-column-${tasks[7].id}`);
  await expect(target).toHaveAttribute("data-focused", "true");
  await expect(target.getByTestId("session-chat")).toBeVisible();
  const expectedSessions = async () =>
    board
      .locator('[data-testid="session-chat"]')
      .evaluateAll((chats) => chats.map((chat) => chat.getAttribute("data-session-id")!).sort());
  const subscribedSessions = () => {
    const active = new Set<string>();
    for (const frame of capture.frames) {
      if (frame.direction !== "sent" || !frame.sessionId) continue;
      if (frame.action === "session.subscribe") active.add(frame.sessionId);
      if (frame.action === "session.unsubscribe") active.delete(frame.sessionId);
    }
    return [...active].sort();
  };
  await expect
    .poll(async () => subscribedSessions().join(",") === (await expectedSessions()).join(","))
    .toBe(true);
  expect(subscribedSessions().length).toBeLessThan(8);
  expect(subscribedSessions().length).toBeGreaterThanOrEqual(2);
  const lower = await target.boundingBox();
  const above = await testPage.getByTestId(`thread-column-${tasks[6].id}`).boundingBox();
  expect(lower!.y).toBeGreaterThan(above!.y);
  await board.evaluate((element) => {
    element.scrollLeft = 0;
  });
  await expect(target.getByTestId("session-chat")).toHaveCount(0);
  await expect(
    testPage.getByTestId(`thread-column-${tasks[0].id}`).getByTestId("session-chat"),
  ).toBeVisible();
  await expect
    .poll(async () => subscribedSessions().join(",") === (await expectedSessions()).join(","))
    .toBe(true);
  await board.evaluate((element) => {
    element.scrollLeft = element.scrollWidth;
  });
  await expect(target.getByTestId("session-chat")).toBeVisible();
  await target.getByRole("button", { name: "Open task", exact: true }).click();
  await expect(testPage).toHaveURL(new RegExp(`/t/${tasks[7].id}`));
});
test.afterEach(async ({ apiClient }) => {
  await apiClient.saveUserSettings(original);
});

// @covers AC-UI-THREADS-DECK-004.1, AC-UI-THREADS-DECK-004.2, AC-UI-THREADS-DECK-004.3, AC-UI-THREADS-DECK-004.7
test("lays out stable task tiles in two rows and falls back in a short window", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(240_000);
  await testPage.setViewportSize({ width: 1440, height: 1100 });
  for (const title of ["A layout chat", "B layout chat", "C layout chat"]) {
    await startPresentationThread(testPage, apiClient, seedData, title);
  }
  await seedThreadPresentation(apiClient, { layout: "grid" });
  expect((await captureThreadSettings(apiClient)).thread_views[0].layout).toBe("grid");
  await testPage.goto("/threads");
  const board = testPage.getByTestId("threads-board");
  await expect(board.locator("[data-thread-column-id]")).toHaveCount(3);
  await expect(board).toHaveAttribute("data-layout", "grid");
  await expectTwoRows(board);
  await expect(board.getByTestId("session-chat")).toHaveCount(3);
  const grid = await threadBoxes(board);
  expect(grid[0].height).toBeGreaterThanOrEqual(300);
  expect(grid[0].width).toBeGreaterThanOrEqual(360);
  expect(grid[0].height).toBeCloseTo(grid[1].height, 1);
  await capturePresentation(testPage, testInfo, "grid-desktop");

  const contentHeight = await board.evaluate((element) => {
    const style = getComputedStyle(element);
    return element.clientHeight - parseFloat(style.paddingTop) - parseFloat(style.paddingBottom);
  });
  const thresholdHeight = 1100 - contentHeight + 612;
  await testPage.setViewportSize({ width: 1440, height: thresholdHeight + 4 });
  await expect(board).toHaveAttribute("data-layout", "grid");
  await testPage.setViewportSize({ width: 1440, height: thresholdHeight - 4 });
  await expect(board).toHaveAttribute("data-layout", "columns");
  await testPage.setViewportSize({ width: 1440, height: thresholdHeight + 4 });
  await expect(board).toHaveAttribute("data-layout", "grid");

  await testPage.setViewportSize({ width: 1440, height: 600 });
  await expect
    .poll(async () => {
      const [a, b] = await threadBoxes(board);
      return Math.abs(a.y - b.y) < 1 && b.x > a.x;
    })
    .toBe(true);
  await testPage.getByTestId("threads-view-settings").click();
  await expect(
    testPage
      .getByTestId("threads-view-settings-popover")
      .getByText("Grid needs more height. Showing Columns."),
  ).toBeVisible();
  await testPage.keyboard.press("Escape");
  expect((await captureThreadSettings(apiClient)).thread_views[0].layout).toBe("grid");

  await testPage.setViewportSize({ width: 1440, height: 1100 });
  await expectTwoRows(board);
  expect((await threadBoxes(board)).map((tile) => tile.id)).toEqual(grid.map((tile) => tile.id));
  await apiClient.archiveTask(grid[1].id!);
  await expect(board.locator("[data-thread-column-id]")).toHaveCount(2);
  await apiClient.archiveTask(grid[2].id!);
  await expect(board.locator("[data-thread-column-id]")).toHaveCount(1);
  const [single] = await threadBoxes(board);
  expect(single.height).toBeGreaterThan(grid[0].height * 1.8);
});
