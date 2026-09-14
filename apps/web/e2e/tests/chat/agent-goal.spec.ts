import { expect, test } from "../../fixtures/test-base";
import { openTaskSession } from "../../helpers/session";
import { openQuickChatWithAgent, sendQuickChatMessage } from "./quick-chat-helpers";

const GOAL_OBJECTIVE_FRAGMENT = "Coordinate contributor PR reviews";
const GOAL_CONTINUATION_COPY = "The agent may continue automatically between replies.";

test.describe("agent goal visibility", () => {
  test("keeps an active goal visible in task chat across idle and reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `Agent goal task ${Date.now()}`,
      seedData.agentProfileId,
      {
        description: "/e2e:goal-active",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("goal task did not return a session id");

    let session = await openTaskSession(testPage, task.id);
    const chip = () => session.activeChat().getByTestId("agent-goal-chip");
    await expect(chip()).toBeVisible({ timeout: 30_000 });

    await chip().hover();
    const popover = testPage.getByTestId("agent-goal-popover");
    await expect(popover).toBeVisible();
    await expect(popover).toContainText(GOAL_OBJECTIVE_FRAGMENT);
    await expect(popover).toContainText(GOAL_CONTINUATION_COPY);

    await testPage.keyboard.press("Escape");
    await expect(popover).toBeHidden();
    await chip().focus();
    await expect(popover).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expect(popover).toBeHidden();

    await testPage.reload();
    session = await openTaskSession(testPage, task.id);
    await expect(session.activeChat().getByTestId("agent-goal-chip")).toBeVisible({
      timeout: 30_000,
    });

    await session.sendMessageViaButton("/e2e:goal-complete");
    await session.expectChatResponseVisible("The provider goal is complete.", 0, {
      timeout: 30_000,
    });
    await expect(session.activeChat().getByTestId("agent-goal-chip")).toBeHidden({
      timeout: 15_000,
    });

    await session.sendMessageViaButton("/e2e:goal-active");
    await expect(session.activeChat().getByTestId("agent-goal-chip")).toBeVisible({
      timeout: 30_000,
    });
    await session.sendMessageViaButton("/e2e:goal-clear");
    await session.expectChatResponseVisible("The provider goal was cleared.", 0, {
      timeout: 30_000,
    });
    await expect(session.activeChat().getByTestId("agent-goal-chip")).toBeHidden({
      timeout: 15_000,
    });
  });

  test("contains a long objective in the desktop disclosure", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `Long agent goal ${Date.now()}`,
      seedData.agentProfileId,
      {
        description: "/e2e:goal-long",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await openTaskSession(testPage, task.id);
    const chip = testPage.getByTestId("agent-goal-chip");
    await expect(chip).toBeVisible({ timeout: 30_000 });
    await chip.hover();

    const objective = testPage.getByTestId("agent-goal-objective");
    await expect(objective).toBeVisible();
    await expect(objective).toContainText(GOAL_OBJECTIVE_FRAGMENT);
    await expect
      .poll(() => objective.evaluate((element) => element.scrollHeight > element.clientHeight))
      .toBe(true);
    const bounds = await objective.boundingBox();
    expect(bounds).not.toBeNull();
    if (!bounds) throw new Error("goal objective bounds unavailable");
    expect(bounds.width).toBeLessThanOrEqual(testPage.viewportSize()?.width ?? bounds.width);
  });

  test("uses the same goal disclosure in Quick Chat", async ({ testPage }) => {
    const dialog = await openQuickChatWithAgent(testPage);
    await sendQuickChatMessage(dialog, testPage, "/e2e:goal-active");

    const chip = dialog.getByTestId("agent-goal-chip");
    await expect(chip).toBeVisible({ timeout: 30_000 });
    await chip.click();
    await expect(testPage.getByTestId("agent-goal-popover")).toBeVisible();
    await expect(testPage.getByTestId("agent-goal-popover")).toContainText(GOAL_CONTINUATION_COPY);
  });
});
