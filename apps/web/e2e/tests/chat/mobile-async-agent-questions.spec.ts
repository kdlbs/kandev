import { test, expect } from "../../fixtures/test-base";
import { waitForAgentMessage, waitForSessionState } from "../../helpers/session";

test.describe("Mobile async agent questions", () => {
  test("answers a waiting async request in one full-height surface", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const title = "Mobile Async Waiting";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      title,
      seedData.agentProfileId,
      {
        description: "/e2e:async-question-wait",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected an active session for the mobile async flow");

    await waitForAgentMessage(apiClient, task.session_id, "ASYNC_WAIT_WORK_MARKER", 60_000);
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "the explicit question wait should park the captured turn",
      timeout: 60_000,
    });

    await testPage.goto("/needs-you-inbox");
    const group = testPage.getByTestId("async-question-group").filter({ hasText: title });
    await expect(group).toBeVisible({ timeout: 30_000 });
    const row = group.getByTestId("async-question-row").first();
    await expect(row.getByText("Waiting for answer")).toBeVisible({ timeout: 15_000 });

    await row.getByTestId("async-question-row-toggle").click();
    const surface = testPage.getByTestId("async-question-answer-surface");
    await expect(surface).toBeVisible({ timeout: 15_000 });

    const overflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(overflow).toBeLessThanOrEqual(0);

    await surface.getByTestId("async-question-option").filter({ hasText: "MongoDB" }).click();
    await surface.getByTestId("async-question-submit").click();

    await expect(group).toHaveCount(0, { timeout: 20_000 });
  });
});
