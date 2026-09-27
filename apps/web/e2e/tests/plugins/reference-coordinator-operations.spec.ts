import { randomUUID } from "node:crypto";
import { expect, test } from "../../fixtures/test-base";
import {
  createLinkedJiraTask,
  installAndGrantCoordinator,
  invokeCoordinatorAction,
  uninstallCoordinator,
} from "./reference-coordinator-helpers";

test("desktop coordinator adopts a linked task, writes to Jira, and reports outcomes", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(150_000);
  let taskId = "";
  let watchId = "";
  await installAndGrantCoordinator(testPage, apiClient, seedData.workspaceId);
  try {
    const instanceKey = `ops-${randomUUID().slice(0, 8)}`;
    await invokeCoordinatorAction(apiClient, seedData.workspaceId, "instance.save", {
      instance_key: instanceKey,
      name: "Operations lead",
      role: "chief-of-staff",
      agent_profile_id: seedData.agentProfileId,
      instructions: "Adopt selected work, submit versioned evidence, and report Host outcomes.",
    });
    const linkedTask = await createLinkedJiraTask(apiClient, {
      workspaceId: seedData.workspaceId,
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
      agentProfileId: seedData.agentProfileId,
    });
    taskId = linkedTask.taskId;
    watchId = linkedTask.watchId;

    await testPage.goto("/plugins/kandev-plugin-coordinator");
    await expect(testPage.getByTestId("coordinator-external-task-id")).toBeVisible();
    await testPage.getByTestId("coordinator-external-task-id").fill(taskId);
    await testPage.getByTestId("coordinator-adopt-external-task").click();

    const taskCard = testPage.locator(
      `[data-testid="coordinator-task-list"] [data-task-id="${taskId}"]`,
    );
    await expect(taskCard).toBeVisible();
    await expect(taskCard).toContainText("Last coordinator claim receipt: generation");
    const operations = taskCard.getByTestId("coordinator-task-operations");
    await operations.locator("summary").click();
    await operations.getByTestId("coordinator-read-issue").click();

    const linkedIssue = operations.getByTestId("coordinator-linked-issue");
    await expect(linkedIssue).toContainText("KCOORD-");
    await expect(linkedIssue).toContainText("Coordinator linked issue");
    await linkedIssue
      .getByTestId("coordinator-issue-comment")
      .fill("Coordinator evidence is ready for review.");
    await linkedIssue.getByTestId("coordinator-issue-comment-submit").click();
    const receipts = operations.getByTestId("coordinator-issue-write-receipts");
    await expect(receipts).toContainText("APPLIED");

    await linkedIssue.getByTestId("coordinator-issue-transition-target").selectOption("start");
    await linkedIssue.getByTestId("coordinator-issue-transition-submit").click();
    await expect(receipts).toContainText("APPLIED");

    const outcomes = testPage.getByTestId("coordinator-task-outcomes");
    await outcomes.getByTestId("coordinator-refresh-outcomes").click();
    await expect(outcomes.getByTestId("coordinator-outcome-report")).toContainText(taskId);
    await expect(outcomes.getByTestId("coordinator-outcome-report")).toContainText(
      "not verified by Host",
    );
  } finally {
    if (watchId)
      await apiClient
        .rawRequest(
          "DELETE",
          `/api/v1/jira/watches/issue/${watchId}?workspace_id=${encodeURIComponent(seedData.workspaceId)}`,
        )
        .catch(() => undefined);
    if (taskId) await apiClient.deleteTask(taskId).catch(() => undefined);
    await uninstallCoordinator(apiClient);
  }
});
