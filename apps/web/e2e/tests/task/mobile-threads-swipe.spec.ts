import { expect, test } from "../../fixtures/test-base";
import { createStandardProfile, openTaskSession } from "../../helpers/git-helper";
import { waitForLatestSessionDone } from "../../helpers/session";
import { swipeDeckLeft } from "./mobile-threads-swipe-helpers";

test("keeps the thread picker closed after leaving and returning to phone layout", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const profile = await createStandardProfile(apiClient, "mobile-picker-resize");
  const title = "Picker resize conversation";
  const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, profile.id, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  await openTaskSession(testPage, title);
  await waitForLatestSessionDone(apiClient, task.id, 1, "picker resize agent turn");
  await testPage.goto("/threads");
  const trigger = testPage.getByTestId("thread-picker-trigger");
  const picker = testPage.getByTestId("thread-picker-sheet");
  for (const width of [820, 1280]) {
    await trigger.tap();
    await expect(picker).toBeVisible();
    await testPage.setViewportSize({ width, height: 900 });
    await expect(trigger).toHaveCount(0);
    await expect(picker).toHaveCount(0);
    await testPage.setViewportSize({ width: 393, height: 851 });
    await expect(trigger).toBeVisible();
    await expect(picker).toBeHidden();
  }
  await trigger.tap();
  await expect(picker).toBeVisible();
});

test("restores picker focus to a surviving thread when its opener is archived", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const profile = await createStandardProfile(apiClient, "mobile-picker-removal");
  for (const title of ["Archived picker opener", "Surviving picker thread"]) {
    const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await openTaskSession(testPage, title);
    await waitForLatestSessionDone(apiClient, task.id, 1, `agent turn for ${title}`);
  }
  await testPage.goto("/threads");
  const board = testPage.getByTestId("threads-board");
  const columns = board.locator("[data-thread-column-id]");
  await expect(columns).toHaveCount(2);
  const openerId = await columns.first().getAttribute("data-thread-column-id");
  if (!openerId) throw new Error("Opening thread has no task ID");
  await columns.first().getByTestId("thread-picker-trigger").tap();
  const picker = testPage.getByTestId("thread-picker-sheet");
  await expect(picker).toBeVisible();
  await apiClient.archiveTask(openerId);
  await expect(columns).toHaveCount(1);
  await expect(picker.getByTestId(/^thread-picker-row-/)).toHaveCount(1);
  await testPage.keyboard.press("Escape");
  await expect(picker).toBeHidden();
  const remainingTrigger = columns.getByTestId("thread-picker-trigger");
  await expect(remainingTrigger).toBeFocused();
  const boardBox = await board.boundingBox();
  expect((await remainingTrigger.boundingBox())!.x).toBeGreaterThanOrEqual(boardBox!.x);
  await remainingTrigger.tap();
  await expect(picker).toBeVisible();
});

// @covers AC-UI-THREADS-DECK-003.13
test("updates inline pagination during a held swipe before destination detail loads", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const profile = await createStandardProfile(apiClient, "mobile-swipe-position");
  for (const title of ["First swipe conversation", "Second swipe conversation"]) {
    const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await openTaskSession(testPage, title);
    await waitForLatestSessionDone(apiClient, task.id, 1, `agent turn for ${title}`);
  }

  let release!: () => void;
  const membershipGate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let heldRequests = 0;
  await testPage.route("**/api/v1/tasks/*/sessions", async (route) => {
    heldRequests++;
    await membershipGate;
    await route.continue();
  });
  try {
    await testPage.goto(`/threads?workspace=${seedData.workspaceId}`);
    const board = testPage.getByTestId("threads-board");
    const cue = testPage.getByTestId("thread-swipe-cue");
    await expect(board.locator("[data-thread-column-id]")).toHaveCount(2);
    await expect.poll(() => heldRequests).toBeGreaterThan(0);
    await expect(cue).toHaveText("1/2");
    await expect(board.getByTestId("session-chat")).toHaveCount(0);

    await swipeDeckLeft(testPage, async () => {
      const progress = await board.evaluate((element) => element.scrollLeft / element.clientWidth);
      expect(progress).toBeGreaterThan(0.5);
      expect(progress).toBeLessThan(0.95);
      await expect(cue).toHaveText("2/2");
      await expect(cue.getByTestId("thread-page-dot").nth(1)).toHaveAttribute(
        "data-active",
        "true",
      );
      await expect(board.getByTestId("session-chat")).toHaveCount(0);
    });

    release();
    await expect(board.getByTestId("session-chat")).toHaveCount(1);
    await expect(cue).toHaveText("2/2");
    await expect
      .poll(() => board.evaluate((element) => element.scrollLeft / element.clientWidth))
      .toBeCloseTo(1, 2);
  } finally {
    release();
    await testPage.unrouteAll({ behavior: "wait" });
  }
});
