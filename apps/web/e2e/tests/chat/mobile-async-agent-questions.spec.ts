import { test, expect } from "../../fixtures/test-base";
import { openTaskSession, waitForAgentMessage, waitForSessionState } from "../../helpers/session";

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

    await row.getByTestId("async-question-row-toggle").tap();
    const surface = testPage.getByTestId("async-question-answer-surface");
    await expect(surface).toBeVisible({ timeout: 15_000 });

    const overflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(overflow).toBeLessThanOrEqual(0);

    await surface.getByTestId("async-question-option").filter({ hasText: "MongoDB" }).tap();
    await surface.getByTestId("async-question-submit").tap();

    await expect(group).toHaveCount(0, { timeout: 20_000 });
  });

  test("answers from the phone chat surface and resumes the author", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Async Chat",
      seedData.agentProfileId,
      {
        description: "/e2e:async-question-wait",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected an active session for the phone chat flow");

    await waitForAgentMessage(apiClient, task.session_id, "ASYNC_WAIT_WORK_MARKER", 60_000);
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "the explicit question wait should park before its phone answer",
      timeout: 60_000,
    });

    await openTaskSession(testPage, task.id);
    const strip = testPage.getByTestId("task-async-question-strip");
    await expect(strip).toBeVisible({ timeout: 20_000 });
    await strip.getByTestId("task-async-question-toggle").tap();
    const row = strip.getByTestId("async-question-row").first();
    await row.getByTestId("async-question-row-toggle").tap();

    const surface = testPage.getByTestId("async-question-answer-surface");
    await expect(surface).toBeVisible({ timeout: 15_000 });
    const bounds = await surface.boundingBox();
    const viewport = testPage.viewportSize();
    expect(bounds).not.toBeNull();
    expect(viewport).not.toBeNull();
    expect(bounds!.height).toBeGreaterThanOrEqual(viewport!.height - 2);

    const overflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(overflow).toBeLessThanOrEqual(0);

    const option = surface.getByTestId("async-question-option").filter({ hasText: "MongoDB" });
    await expect(option).toBeVisible();
    const targetHeights = await surface
      .locator("button")
      .evaluateAll((buttons) => buttons.map((button) => button.getBoundingClientRect().height));
    expect(targetHeights.length).toBeGreaterThan(0);
    expect(Math.min(...targetHeights)).toBeGreaterThanOrEqual(44);

    await option.tap();
    await surface.getByTestId("async-question-submit").tap();
    await waitForAgentMessage(apiClient, task.session_id, "ASYNC_ANSWER_RECEIVED: MongoDB", 45_000);
  });
});
