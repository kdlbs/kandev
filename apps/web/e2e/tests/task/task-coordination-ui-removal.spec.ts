import { expect, test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
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
  const criteriaSnapshot = (await criteriaResponse.json()) as {
    blocked: boolean;
    criteria: Array<{ id: string }>;
  };
  expect(criteriaSnapshot.blocked).toBe(true);
  expect(criteriaSnapshot.criteria.map((criterion) => criterion.id)).toEqual(["required-review"]);

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
      await testPage.goto(`/t/${task.id}`, { waitUntil: "domcontentloaded" });
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
    const [currentTask, persistedGateResponse] = await Promise.all([
      apiClient.getTask(configuredTask.id),
      apiClient.rawRequest("GET", `/api/v1/tasks/${configuredTask.id}/completion-gate`),
    ]);
    expect(currentTask.workflow_step_id).toBe(seedData.startStepId);
    expect(currentTask.state).not.toBe("COMPLETED");
    expect(persistedGateResponse.ok).toBe(true);
    const persistedGate = (await persistedGateResponse.json()) as { blocked: boolean };
    expect(persistedGate.blocked).toBe(true);
    const stepButton = testPage.getByTestId(`workflow-step-${completingStep.name}`);
    const moveResponse = testPage.waitForResponse(
      (response) =>
        new URL(response.url()).pathname === `/api/v1/tasks/${configuredTask.id}/move` &&
        response.request().method() === "POST",
    );
    if (await stepButton.isVisible()) {
      await stepButton.hover();
      const movePopover = testPage.getByTestId("workflow-step-popover");
      await expect(movePopover).toBeVisible();
      await waitForFiniteAnimations(movePopover);
      await movePopover.getByTestId("workflow-step-move-here").click();
    } else {
      // The responsive top bar replaces the full step list with a disclosure
      // when its center region is constrained.
      await testPage.getByTestId("workflow-stepper-minimal").hover();
      const disclosure = testPage.getByTestId("workflow-step-disclosure");
      await expect(disclosure).toBeVisible();
      await waitForFiniteAnimations(disclosure);
      const targetStep = disclosure.getByTestId(
        `workflow-step-disclosure-row-${completingStep.id}`,
      );
      await expect(targetStep).toBeVisible();
      const moveButton = targetStep.getByTestId(
        `workflow-step-disclosure-move-${completingStep.id}`,
      );
      await expect(moveButton).toBeVisible();
      await waitForFiniteAnimations(disclosure);
      await moveButton.click();
    }
    const move = await moveResponse;
    expect(move.status()).toBe(409);
    await expect(testPage.getByTestId("task-move-error-banner")).toBeVisible();
    expect(coordinationReads).toEqual([]);
  } finally {
    await apiClient.deleteTask(ordinaryTask.id).catch(() => undefined);
    await apiClient.deleteTask(configuredTask.id).catch(() => undefined);
  }
});
