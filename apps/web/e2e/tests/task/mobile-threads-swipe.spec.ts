import { expect, test } from "../../fixtures/test-base";
import { createStandardProfile, openTaskSession } from "../../helpers/git-helper";
import { waitForLatestSessionDone } from "../../helpers/session";
import { swipeDeckLeft } from "./mobile-threads-swipe-helpers";

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
