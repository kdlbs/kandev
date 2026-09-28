import { expect, test } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";

test("task details omit native coordination controls for ordinary and configured tasks", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const ordinaryTask = await apiClient.createTask(seedData.workspaceId, "Ordinary task detail", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const configuredTask = await apiClient.createTask(
    seedData.workspaceId,
    "Configured task detail",
    {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    },
  );
  const claimResponse = await apiClient.rawRequest(
    "POST",
    `/api/v1/_test/tasks/${configuredTask.id}/management-claim`,
    { installation_id: "e2e-coordinator", instance_key: "configured-task" },
  );
  expect(claimResponse.ok).toBe(true);
  const criteriaResponse = await apiClient.rawRequest(
    "PUT",
    `/api/v1/tasks/${configuredTask.id}/completion-gate/criteria`,
    {
      expected_revision: 0,
      criteria: [
        {
          id: "required-review",
          description: "Review the task before completion",
          evidence_subject: { kind: "task_revision", id: configuredTask.id },
        },
      ],
    },
  );
  expect(criteriaResponse.ok).toBe(true);

  const coordinationReads: string[] = [];
  testPage.on("request", (request) => {
    const pathname = new URL(request.url()).pathname;
    for (const task of [ordinaryTask, configuredTask]) {
      if (
        pathname.startsWith(`/api/v1/tasks/${task.id}/management-claim`) ||
        pathname.startsWith(`/api/v1/tasks/${task.id}/completion-gate`)
      ) {
        coordinationReads.push(pathname);
      }
    }
  });

  try {
    for (const task of [ordinaryTask, configuredTask]) {
      await testPage.goto(`/t/${task.id}`);
      await expect(testPage.getByTestId("task-topbar")).toBeVisible();
      const workbench = testPage.getByTestId("dockview-task-layout");
      await expect(workbench).toBeVisible();
      await expect(testPage.getByTestId("task-management-claim-row")).toHaveCount(0);
      await expect(testPage.getByTestId("task-completion-gate-row")).toHaveCount(0);

      const [topbarBox, workbenchBox] = await Promise.all([
        testPage.getByTestId("task-topbar").boundingBox(),
        workbench.boundingBox(),
      ]);
      expect(topbarBox).not.toBeNull();
      expect(workbenchBox).not.toBeNull();
      if (!topbarBox || !workbenchBox) throw new Error("task layout geometry is unavailable");
      expect(workbenchBox.y - (topbarBox.y + topbarBox.height)).toBeLessThan(8);
      await dwell(
        testPage,
        500,
        "negative-assertion",
        "removed task detail controls must not request claim or completion-gate data",
      );
    }

    const completingStep = seedData.steps.find((step) => step.complete_task_on_enter);
    if (!completingStep) throw new Error("seed workflow has no completing step");
    const stepButton = testPage.getByTestId(`workflow-step-${completingStep.name}`);
    await stepButton.hover();
    const movePopover = testPage.getByTestId("workflow-step-popover");
    await expect(movePopover).toBeVisible();
    const blockedMove = testPage.waitForResponse(
      (response) =>
        response.url().endsWith(`/api/v1/tasks/${configuredTask.id}/move`) &&
        response.status() === 409,
    );
    await movePopover.getByTestId("workflow-step-move-here").click();
    await blockedMove;
    await expect(testPage.getByTestId("task-move-error-banner")).toBeVisible();
    expect(coordinationReads).toEqual([]);
  } finally {
    await apiClient.deleteTask(ordinaryTask.id).catch(() => undefined);
    await apiClient.deleteTask(configuredTask.id).catch(() => undefined);
  }
});
