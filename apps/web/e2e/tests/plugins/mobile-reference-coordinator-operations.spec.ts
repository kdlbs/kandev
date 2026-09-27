import { randomUUID } from "node:crypto";
import { expect, test } from "../../fixtures/test-base";
import {
  installAndGrantCoordinator,
  invokeCoordinatorAction,
  uninstallCoordinator,
} from "./reference-coordinator-helpers";

test("phone coordinator adopts a task, submits evidence, and shows its outcome", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  let taskId = "";
  await installAndGrantCoordinator(testPage, apiClient, seedData.workspaceId);
  try {
    const instanceKey = `phone-ops-${randomUUID().slice(0, 8)}`;
    const task = await apiClient.createTask(
      seedData.workspaceId,
      `Phone evidence ${randomUUID().slice(0, 6)}`,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        agent_profile_id: seedData.agentProfileId,
        prepare_session: true,
      },
    );
    taskId = task.id;
    await invokeCoordinatorAction(apiClient, seedData.workspaceId, "instance.save", {
      instance_key: instanceKey,
      name: "Phone operations lead",
      role: "delivery-lead",
      agent_profile_id: seedData.agentProfileId,
      instructions: "Claim selected tasks and report current evidence.",
    });

    await testPage.goto("/plugins/kandev-plugin-coordinator-template");
    await testPage.getByRole("tab", { name: "Tasks" }).tap();
    await testPage.getByTestId("coordinator-external-task-id").fill(taskId);
    await testPage.getByTestId("coordinator-adopt-external-task").tap();

    const taskCard = testPage.locator(
      `[data-testid="coordinator-task-list"] [data-task-id="${taskId}"]`,
    );
    await expect(taskCard).toBeVisible();
    await expect(taskCard).toContainText("Last coordinator claim receipt: generation");
    const operations = taskCard.getByTestId("coordinator-task-operations");
    await operations.locator("summary").tap();
    await operations.getByTestId("coordinator-criterion-id").fill("phone-checks");
    await operations
      .getByTestId("coordinator-criterion-description")
      .fill("Mobile release checks pass");
    await operations.getByTestId("coordinator-evidence-kind").fill("ci_run");
    await operations.getByTestId("coordinator-evidence-id").fill("phone-run-42");
    await operations.getByTestId("coordinator-evidence-revision").fill("head-phone-42");
    const saveCriterion = operations.getByTestId("coordinator-save-criterion");
    await expect(saveCriterion).toHaveCSS("min-height", "44px");
    await saveCriterion.tap();

    await operations
      .getByTestId("coordinator-evidence-summary")
      .fill("Phone CI passed on head-phone-42.");
    await operations
      .getByTestId("coordinator-evidence-reference")
      .fill("https://ci.example/phone-run-42");
    const submitEvidence = operations.getByTestId("coordinator-submit-evidence");
    await expect(submitEvidence).toHaveCSS("min-height", "44px");
    await submitEvidence.tap();

    await testPage.getByRole("tab", { name: "Outcomes" }).tap();
    const outcomes = testPage.getByTestId("coordinator-task-outcomes");
    await outcomes.getByTestId("coordinator-refresh-outcomes").tap();
    const report = outcomes.getByTestId("coordinator-outcome-report");
    await expect(report).toContainText(taskId);
    await expect(report).toContainText("not verified by Host");
    await expect(report).toContainText("unknown");

    const geometry = await testPage.getByTestId("coordinator-page").evaluate((element) => ({
      clientWidth: element.clientWidth,
      scrollWidth: element.scrollWidth,
    }));
    expect(geometry.scrollWidth).toBeLessThanOrEqual(geometry.clientWidth + 1);
  } finally {
    if (taskId) await apiClient.deleteTask(taskId).catch(() => undefined);
    await uninstallCoordinator(apiClient);
  }
});
