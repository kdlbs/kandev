import { test, expect } from "../../fixtures/test-base";
import { waitForAgentMessage, waitForSessionState } from "../../helpers/session";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

async function createAsyncQuestionTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  scenario: string,
) {
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: scenario,
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

test.describe("Async agent questions", () => {
  test("asks early, keeps working, and answers from the Inbox", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const title = "Async Keep Working";
    const task = await createAsyncQuestionTask(apiClient, seedData, title, "/e2e:async-question");
    if (!task.session_id) throw new Error("expected an active session for the async question flow");

    await waitForAgentMessage(apiClient, task.session_id, "ASYNC_WORK_MARKER", 60_000);
    await testPage.goto("/needs-you-inbox");

    const group = testPage.getByTestId("async-question-group").filter({ hasText: title });
    await expect(group).toBeVisible({ timeout: 30_000 });
    const row = group.getByTestId("async-question-row").first();
    await expect(row.getByText("Agent continues work")).toBeVisible({ timeout: 15_000 });

    await row.getByTestId("async-question-row-toggle").click();
    await row.getByTestId("async-question-option").filter({ hasText: "PostgreSQL" }).click();
    await row.getByTestId("async-question-submit").click();

    await expect(group).toHaveCount(0, { timeout: 20_000 });
  });

  test("parks an explicit dependency wait and resumes after the answer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const title = "Async Explicit Wait";
    const task = await createAsyncQuestionTask(
      apiClient,
      seedData,
      title,
      "/e2e:async-question-wait",
    );
    if (!task.session_id) throw new Error("expected an active session for the async wait flow");

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
    await row.getByTestId("async-question-option").filter({ hasText: "SQLite" }).click();
    await row.getByTestId("async-question-submit").click();

    await expect(group).toHaveCount(0, { timeout: 20_000 });
  });
});
