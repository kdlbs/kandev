// AC-COORDINATOR-RELAY-001, -002: a manager answers a waiting agent's
// question or permission request on the Needs you item, through the same
// resolvers the Inbox and the task chat use
// (docs/specs/coordinator/requirements/relay.md).
import { test, expect } from "../../fixtures/test-base";
import { waitForAgentMessage, waitForSessionState } from "../../helpers/session";
import { linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";

test.describe("Coordinator answer in place: question", () => {
  test("answering a clarification on the item resumes the task and clears the Inbox row (AC .001.3)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Answering Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });
    const title = "Answer In Place Question";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      title,
      seedData.agentProfileId,
      {
        description: "/e2e:clarification",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected an active session for the question task");
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "clarification session should block before the Needs you screen is opened",
      timeout: 60_000,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const card = testPage.getByTestId(`needs-you-item-${task.id}`);
    await expect(card).toBeVisible({ timeout: 30_000 });
    await expect(card.getByRole("link", { name: "Open task" })).toBeVisible();

    await card.getByTestId("needs-you-answer-here").click();
    const overlay = card.getByTestId("clarification-overlay");
    await expect(overlay).toBeVisible({ timeout: 15_000 });
    await overlay.getByTestId("clarification-option").filter({ hasText: "PostgreSQL" }).click();

    await expect(card).toHaveCount(0, { timeout: 30_000 });
    await waitForAgentMessage(apiClient, task.session_id, "You answered:", 30_000);

    await testPage.goto("/needs-you-inbox");
    await expect(testPage.getByTestId("needs-you-inbox-empty")).toBeVisible({ timeout: 30_000 });
  });
});

test.describe("Coordinator answer in place: permission", () => {
  test.beforeAll(async ({ backend }) => {
    await backend.restart({ AGENTCTL_AUTO_APPROVE_PERMISSIONS: "false" });
  });
  test.afterAll(async ({ backend }) => {
    await backend.restart({ AGENTCTL_AUTO_APPROVE_PERMISSIONS: "true" });
  });

  test("approving a permission on the item resolves it through the chat's path (AC .002.2)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Permission Answering Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Answer In Place Permission",
      seedData.agentProfileId,
      {
        description: "/e2e:permission-flow",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected an active session for the permission task");

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const card = testPage.getByTestId(`needs-you-item-${task.id}`);
    await expect(card).toBeVisible({ timeout: 45_000 });

    await card.getByTestId("needs-you-answer-here").click();
    await expect(card.getByTestId("permission-answer")).toBeVisible({ timeout: 15_000 });
    await card.getByTestId("permission-approve").click();

    await waitForAgentMessage(
      apiClient,
      task.session_id,
      "Permission was granted and command executed.",
      30_000,
    );
    await expect(card).toHaveCount(0, { timeout: 30_000 });
  });
});
