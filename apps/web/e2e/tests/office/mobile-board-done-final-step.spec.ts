import { test, expect } from "../../fixtures/office-fixture";
import type { ApiClient } from "../../helpers/api-client";

/**
 * Phone parity for REQ-OFFICE-HUMAN-DONE-001/-002: the status picker's Done
 * on a task before its final workflow step moves it to that step, and a
 * pending approver's refusal is shown as visible text.
 */

async function firstAndFinalSteps(apiClient: ApiClient, workflowId: string) {
  const { steps } = await apiClient.listWorkflowSteps(workflowId);
  const sorted = [...steps].sort((a, b) => a.position - b.position);
  return { first: sorted[0], final: sorted[sorted.length - 1] };
}

test.describe("Office status picker Done on mobile", () => {
  test("Done before the final step moves the task there and completes it", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const { first, final } = await firstAndFinalSteps(apiClient, officeSeed.workflowId);
    const task = await apiClient.createTask(officeSeed.workspaceId, "Mobile Done Move", {
      workflow_id: officeSeed.workflowId,
      workflow_step_id: first.id,
    });

    await testPage.goto(`/office/tasks/${task.id}`);
    await testPage.getByTestId("status-picker-trigger").click();
    await testPage.getByTestId("status-picker-option-done").click();

    await expect(testPage.getByTestId("status-picker-trigger")).toContainText(/Done/i);
    await expect
      .poll(async () => {
        const persisted = await apiClient.getTask(task.id);
        return { state: persisted.state, stepId: persisted.workflow_step_id };
      })
      .toEqual({ state: "COMPLETED", stepId: final.id });
  });

  test("a pending approver refusal is shown and nothing changes", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const approver = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Mobile Done Approver",
      role: "worker",
    })) as { id: string };
    const { first } = await firstAndFinalSteps(apiClient, officeSeed.workflowId);
    const task = await apiClient.createTask(officeSeed.workspaceId, "Mobile Done Gated", {
      workflow_id: officeSeed.workflowId,
      workflow_step_id: first.id,
    });
    const attach = await apiClient.rawRequest("POST", `/api/v1/office/tasks/${task.id}/approvers`, {
      agent_profile_id: approver.id,
    });
    expect(attach.ok).toBe(true);
    const before = await apiClient.getTask(task.id);

    await testPage.goto(`/office/tasks/${task.id}`);
    const trigger = testPage.getByTestId("status-picker-trigger");
    const initialLabel = (await trigger.textContent()) ?? "";
    await trigger.click();
    await testPage.getByTestId("status-picker-option-done").click();

    await expect(
      testPage.getByText(/awaiting approval from Mobile Done Approver/i).first(),
    ).toBeVisible();
    await expect(trigger).toHaveText(initialLabel);
    const after = await apiClient.getTask(task.id);
    expect({ state: after.state, stepId: after.workflow_step_id }).toEqual({
      state: before.state,
      stepId: before.workflow_step_id,
    });
  });
});
