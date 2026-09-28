import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

type Claim = {
  task_id: string;
  installation_id: string;
  instance_key: string;
  generation: number;
  resource_version: string;
};

async function seedUnavailableManager(apiClient: ApiClient, taskId: string) {
  const response = await apiClient.rawRequest(
    "POST",
    `/api/v1/_test/tasks/${taskId}/management-claim`,
    { installation_id: "removed-coordinator", instance_key: "manager-instance" },
  );
  if (!response.ok) throw new Error(`seed management claim failed: ${response.status}`);
  return (await response.json()) as Claim;
}

test("claim conflict and human takeover", async ({ testPage, apiClient, seedData }) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Management claim recovery", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const claim = await seedUnavailableManager(apiClient, task.id);

  await testPage.goto(`/t/${task.id}`);
  const row = testPage.getByTestId("task-management-claim-row");
  await expect(row).toBeVisible();
  await expect(row.getByTestId("task-management-claim-owner")).toContainText("removed-coordinator");
  await row.getByTestId("task-management-claim-open").click();

  const surface = testPage.getByTestId("task-management-claim-surface");
  await expect(surface).toBeVisible();
  await expect(surface.getByTestId("task-management-claim-history")).toContainText(
    "E2E unavailable manager",
  );

  const otherRelease = await apiClient.rawRequest(
    "POST",
    `/api/v1/tasks/${task.id}/management-claim`,
    {
      action: "release",
      expected_task_resource_version: task.updated_at,
      expected_claim_resource_version: claim.resource_version,
      reason: "Concurrent human release",
    },
  );
  expect(otherRelease.ok).toBe(true);

  await surface
    .getByTestId("task-management-claim-reason")
    .fill("Take over the unavailable manager");
  await surface.getByTestId("task-management-claim-transfer").click();
  await expect(surface.getByTestId("task-management-claim-error")).toBeVisible();

  const reacquired = await seedUnavailableManager(apiClient, task.id);
  expect(reacquired.generation).toBeGreaterThan(claim.generation);
  await surface.getByTestId("task-management-claim-refresh").click();
  await expect(surface.getByTestId("task-management-claim-owner")).toContainText(
    "removed-coordinator",
  );
  await surface
    .getByTestId("task-management-claim-reason")
    .fill("Take over the unavailable manager");
  await surface.getByTestId("task-management-claim-transfer").click();

  await expect(row.getByTestId("task-management-claim-owner")).toContainText(/human/i);
  await expect(surface.getByTestId("task-management-claim-history")).toContainText(
    "Take over the unavailable manager",
  );
});
