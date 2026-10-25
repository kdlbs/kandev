// A task on an unwatched board never appears on Needs you or in its count
// (docs/specs/coordinator/requirements/permissions.md, Watches).
import { test, expect } from "../../fixtures/test-base";
import { waitForSessionState } from "../../helpers/session";
import { waitForHttp } from "../../helpers/causal-waits";
import type { ApiClient } from "../../helpers/api-client";
import {
  linkToCoordinatorNeedsYou,
  linkToCoordinatorSettings,
} from "../../../lib/coordinator/links";

const SETTINGS_PATH = /\/coordinators\/[^/]+\/settings$/;

type Seed = {
  workspaceId: string;
  agentProfileId: string;
  repositoryId: string;
  worktreeExecutorProfileId: string;
};

async function blockedTask(
  apiClient: ApiClient,
  seed: Seed,
  title: string,
  workflowId: string,
  stepId: string,
) {
  const task = await apiClient.createTaskWithAgent(seed.workspaceId, title, seed.agentProfileId, {
    description: "/e2e:clarification",
    workflow_id: workflowId,
    workflow_step_id: stepId,
    repository_ids: [seed.repositoryId],
  });
  if (!task.session_id) throw new Error("expected an active session for the clarification task");
  await waitForSessionState(apiClient, {
    taskId: task.id,
    sessionId: task.session_id,
    expectedState: "WAITING_FOR_INPUT",
    message: "clarification session should block before Needs you is opened",
    timeout: 60_000,
  });
  return task;
}

test.describe("Coordinator watches on Needs you", () => {
  test("narrowing the watch set hides the other board's task and its count", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const other = await apiClient.createWorkflow(seedData.workspaceId, "Unwatched board");
    const otherStart = (await apiClient.createWorkflowStep(other.id, "Backlog", 0)).id;

    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Watcher",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      task_agent_profile_id: seedData.agentProfileId,
      task_executor_profile_id: seedData.worktreeExecutorProfileId,
    });
    const watched = await blockedTask(
      apiClient,
      seedData,
      "Watched board task",
      seedData.workflowId,
      seedData.startStepId,
    );
    const unwatched = await blockedTask(
      apiClient,
      seedData,
      "Unwatched board task",
      other.id,
      otherStart,
    );

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    await expect(testPage.getByTestId(`needs-you-item-${watched.id}`)).toBeVisible();
    await expect(testPage.getByTestId(`needs-you-item-${unwatched.id}`)).toBeVisible();
    await expect(testPage.getByTestId("count-needs-you")).toContainText("2");

    await testPage.goto(
      `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=watches`,
    );
    await testPage.getByRole("switch").click();
    await testPage.getByTestId(`watches-toggle-${other.id}`).click();
    const put = waitForHttp(testPage, "PUT", SETTINGS_PATH);
    await testPage.getByRole("button", { name: "Save changes" }).click();
    await put;

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    await expect(testPage.getByTestId(`needs-you-item-${watched.id}`)).toBeVisible();
    await expect(testPage.getByTestId(`needs-you-item-${unwatched.id}`)).toHaveCount(0);
    await expect(testPage.getByTestId("count-needs-you")).toContainText("1");
    await expect(testPage.getByTestId("watches-none-notice")).toHaveCount(0);
  });
});
