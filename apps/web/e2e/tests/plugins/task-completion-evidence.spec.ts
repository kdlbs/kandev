import { expect, test } from "../../fixtures/test-base";
import {
  attemptBlockedCompletion,
  makeCompletionEvidenceStale,
  readCompletionGate,
  readCompletionGateHistory,
  readTaskState,
  seedStaleCompletionEvidence,
} from "./task-completion-gate-e2e-helpers";

test("stale evidence blocks completion and the native override applies to one move", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const seed = await seedStaleCompletionEvidence(
    apiClient,
    seedData.workspaceId,
    "Desktop completion evidence",
  );
  try {
    const staleTask = await makeCompletionEvidenceStale(apiClient, seed.taskId);
    expect(staleTask.updated_at).not.toBe("");
    const blockedMove = await attemptBlockedCompletion(
      apiClient,
      seed.taskId,
      seed.workflowId,
      seed.completionStepId,
    );
    expect(blockedMove.status).toBe(409);
    expect(await readCompletionGate(apiClient, seed.taskId)).toMatchObject({
      blocked: true,
      blockers: [{ criterion_id: "task-review", reason: "evidence_stale" }],
    });

    await testPage.goto(`/t/${seed.taskId}`);
    const row = testPage.getByTestId("task-completion-gate-row");
    await expect(row).toBeVisible();
    await row.getByTestId("task-completion-gate-open").click();
    const surface = testPage.getByTestId("task-completion-gate-surface");
    await expect(surface.getByTestId("task-completion-gate-blocker")).toContainText(
      "Evidence is out of date",
    );

    await surface
      .getByTestId("task-completion-gate-override-step")
      .selectOption(seed.completionStepId);
    await surface
      .getByTestId("task-completion-gate-override-reason")
      .fill("The release owner accepted the remaining evidence risk");
    await surface.getByTestId("task-completion-gate-override").click();

    await expect.poll(() => readTaskState(apiClient, seed.taskId)).toBe("COMPLETED");
    const history = await readCompletionGateHistory(apiClient, seed.taskId);
    expect(history.items).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          action: "completion_overridden",
          reason: "The release owner accepted the remaining evidence risk",
        }),
      ]),
    );
    await row.getByTestId("task-completion-gate-open").click();
    await expect(testPage.getByTestId("task-completion-gate-history")).toContainText(
      "The release owner accepted the remaining evidence risk",
    );
  } finally {
    await apiClient.deleteTask(seed.taskId).catch(() => undefined);
    await apiClient.deleteWorkflow(seed.workflowId).catch(() => undefined);
  }
});
