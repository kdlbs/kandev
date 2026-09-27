import { expect, test } from "../../fixtures/test-base";
import {
  attemptBlockedCompletion,
  makeCompletionEvidenceStale,
  readCompletionGateHistory,
  readTaskState,
  seedStaleCompletionEvidence,
} from "./task-completion-gate-e2e-helpers";

test("phone can inspect stale completion evidence and record a one-move override", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const seed = await seedStaleCompletionEvidence(
    apiClient,
    seedData.workspaceId,
    "Phone completion evidence",
  );
  try {
    await makeCompletionEvidenceStale(apiClient, seed.taskId);
    const blockedMove = await attemptBlockedCompletion(
      apiClient,
      seed.taskId,
      seed.workflowId,
      seed.completionStepId,
    );
    expect(blockedMove.status).toBe(409);

    await testPage.goto(`/t/${seed.taskId}`);
    const row = testPage.getByTestId("task-completion-gate-row");
    await expect(row).toBeVisible();
    const open = row.getByTestId("task-completion-gate-open");
    expect((await open.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await open.tap();

    const surface = testPage.getByTestId("task-completion-gate-surface");
    await expect(surface.getByTestId("task-completion-gate-blockers")).toBeVisible();
    await expect(surface.getByTestId("task-completion-gate-blocker")).toContainText(
      "Evidence is out of date",
    );
    await surface
      .getByTestId("task-completion-gate-override-step")
      .selectOption(seed.completionStepId);
    await surface
      .getByTestId("task-completion-gate-override-reason")
      .fill("Accepted by the release owner on phone");
    const override = surface.getByTestId("task-completion-gate-override");
    expect((await override.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await override.tap();

    await expect.poll(() => readTaskState(apiClient, seed.taskId)).toBe("COMPLETED");
    expect((await readCompletionGateHistory(apiClient, seed.taskId)).items).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          action: "completion_overridden",
          reason: "Accepted by the release owner on phone",
        }),
      ]),
    );
    await row.getByTestId("task-completion-gate-open").tap();
    await expect(testPage.getByTestId("task-completion-gate-history")).toContainText(
      "Accepted by the release owner on phone",
    );
    const dimensions = await testPage.evaluate(() => ({
      clientWidth: document.documentElement.clientWidth,
      scrollWidth: document.documentElement.scrollWidth,
    }));
    expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth);
  } finally {
    await apiClient.deleteTask(seed.taskId).catch(() => undefined);
    await apiClient.deleteWorkflow(seed.workflowId).catch(() => undefined);
  }
});
