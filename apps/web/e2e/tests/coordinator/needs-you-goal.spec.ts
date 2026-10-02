// The goal note above Needs you (docs/plans/workspace-coordinator-p2/task-11-orders-goal-sections.md).
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  linkToCoordinatorNeedsYou,
  linkToCoordinatorSettings,
} from "../../../lib/coordinator/links";

const NOTE = "goal-note";
const GOAL_NAME = "Ship the billing beta";

type SeedData = { workspaceId: string; agentProfileId: string; worktreeExecutorProfileId: string };

async function seedCoordinator(apiClient: ApiClient, seedData: SeedData) {
  return apiClient.createCoordinator(seedData.workspaceId, {
    name: "Goal Note Coordinator",
    agent_profile_id: seedData.agentProfileId,
    executor_profile_id: seedData.worktreeExecutorProfileId,
    task_agent_profile_id: seedData.agentProfileId,
    task_executor_profile_id: seedData.worktreeExecutorProfileId,
  });
}

async function putGoal(
  apiClient: ApiClient,
  workspaceId: string,
  coordinatorId: string,
  body: Record<string, unknown>,
) {
  const response = await apiClient.rawRequest(
    "PUT",
    `/api/v1/workspaces/${workspaceId}/coordinators/${coordinatorId}/goal`,
    body,
  );
  expect(response.ok, await response.text()).toBeTruthy();
}

test.describe("Goal note on Needs you", () => {
  test("walks no goal, active, overdue and met, each after a reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const coordinator = await seedCoordinator(apiClient, seedData);
    const needsYou = linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id);
    const base = `/api/v1/workspaces/${seedData.workspaceId}/coordinators/${coordinator.id}`;

    await testPage.goto(needsYou);
    const note = testPage.getByTestId(NOTE);
    await expect(note).toContainText("No goal is set, so this list is ordered by urgency alone.");
    await expect(testPage.getByTestId("goal-note-action")).toHaveText("Set a goal");
    await testPage.getByTestId("goal-note-action").click();
    await expect(testPage).toHaveURL(
      new RegExp(
        `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}\\?section=goal`,
      ),
    );

    await putGoal(apiClient, seedData.workspaceId, coordinator.id, {
      name: GOAL_NAME,
      due_on: "2999-10-31",
      criteria: [{ text: "Docs" }, { text: "Tests" }],
    });
    await testPage.goto(needsYou);
    await expect(testPage.getByTestId(NOTE)).toContainText(GOAL_NAME);
    await expect(testPage.getByTestId(NOTE)).toContainText("Due ");
    await expect(testPage.getByTestId(NOTE)).toContainText("0 of 2 criteria met");
    await expect(testPage.getByTestId("goal-note-action")).toHaveCount(0);

    await putGoal(apiClient, seedData.workspaceId, coordinator.id, {
      name: GOAL_NAME,
      due_on: "2020-01-05",
      criteria: [{ text: "Docs" }, { text: "Tests" }],
    });
    await testPage.reload();
    await expect(testPage.getByTestId(NOTE)).toContainText("Overdue since ");

    const met = await apiClient.rawRequest("POST", `${base}/goal/met`, {});
    expect(met.ok, await met.text()).toBeTruthy();
    await testPage.reload();
    await expect(testPage.getByTestId(NOTE)).toContainText(`"${GOAL_NAME}" was met on `);
    await expect(testPage.getByTestId("goal-note-action")).toHaveText("Set the next goal");
  });

  test("shows no goal note while the phase-2 flag is off", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const release = await backend.useEnv({ KANDEV_FEATURES_COORDINATOR_PHASE2: "false" });
    try {
      const coordinator = await seedCoordinator(apiClient, seedData);
      await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
      await expect(testPage.getByTestId("count-needs-you")).toBeVisible();
      await expect(testPage.getByTestId(NOTE)).toHaveCount(0);
    } finally {
      await release();
    }
  });
});
