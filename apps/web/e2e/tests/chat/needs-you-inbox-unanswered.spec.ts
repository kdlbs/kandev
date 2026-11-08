// AC-TASKS-CLARIFICATION-UNANSWERED-001.1/.4/.5/.7: with the workspace's
// unanswered inbox on, a question a newer turn superseded appears in the
// Needs-you "unanswered" section; a late answer sends an ordinary message and
// closes only that question, and Close removes one without sending anything.
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForHttp } from "../../helpers/causal-waits";

const OPTION_LABEL = "PostgreSQL";

async function seedSupersededQuestion(
  apiClient: ApiClient,
  seed: { workspaceId: string; agentProfileId: string },
  title: string,
  pendingId: string,
) {
  const task = await apiClient.createTask(seed.workspaceId, title);
  // A running session admits the late answer through its ordinary queue.
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "RUNNING",
    agentProfileId: seed.agentProfileId,
  });
  const askedAt = new Date(Date.now() - 120_000).toISOString();
  await apiClient.seedSessionMessage(sessionId, {
    type: "clarification_request",
    newTurn: true,
    turnStartedAt: askedAt,
    turnCompletedAt: askedAt,
    metadata: {
      pending_id: pendingId,
      session_id: sessionId,
      question_id: "db-choice",
      question_index: 0,
      question_total: 1,
      status: "pending",
      question: {
        id: "db-choice",
        title: `${title} question`,
        prompt: "Which database should this use?",
        options: [
          { option_id: "opt-pg", label: OPTION_LABEL, description: "" },
          { option_id: "opt-sqlite", label: "SQLite", description: "" },
        ],
      },
    },
  });
  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: "Moving on with other work.",
    newTurn: true,
    turnStartedAt: new Date(Date.now() - 60_000).toISOString(),
  });
  return { taskId: task.id, sessionId };
}

test.describe("Needs-you inbox unanswered section", () => {
  test.beforeEach(async ({ apiClient, seedData }) => {
    await apiClient.updateWorkspace(seedData.workspaceId, {
      clarification_unanswered_inbox_enabled: true,
    });
  });

  test.afterEach(async ({ apiClient, seedData }) => {
    await apiClient.updateWorkspace(seedData.workspaceId, {
      clarification_unanswered_inbox_enabled: false,
    });
  });

  test("answers a superseded question as a new message and closes only that question", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const answered = await seedSupersededQuestion(
      apiClient,
      seedData,
      "Unanswered Late Answer",
      `pend-unanswered-late-${Date.now()}`,
    );
    await seedSupersededQuestion(
      apiClient,
      seedData,
      "Unanswered Bystander",
      `pend-unanswered-other-${Date.now()}`,
    );

    await testPage.goto("/needs-you-inbox");
    const section = testPage.getByTestId("needs-you-unanswered-section");
    const row = section
      .getByTestId("needs-you-unanswered-row")
      .filter({ hasText: "Unanswered Late Answer" });
    const bystander = section
      .getByTestId("needs-you-unanswered-row")
      .filter({ hasText: "Unanswered Bystander" });
    await expect(row).toBeVisible();
    await expect(bystander).toBeVisible();

    await row.getByTestId("needs-you-unanswered-row-toggle").click();
    await row.getByTestId("clarification-option").filter({ hasText: OPTION_LABEL }).click();
    const lateAnswerClose = waitForHttp(
      testPage,
      "POST",
      /\/api\/v1\/clarification-inbox\/unanswered\/[^/]+\/late-answer$/,
    );
    await row.getByTestId("clarification-late-submit").click();

    const closeResponse = await lateAnswerClose;
    expect(closeResponse.status()).toBe(204);
    expect(closeResponse.request().postDataJSON()).toMatchObject({
      session_id: answered.sessionId,
    });
    await expect(row).toHaveCount(0);
    await expect(bystander).toBeVisible();
  });

  test("closes a question without sending a message", async ({ testPage, apiClient, seedData }) => {
    const closed = await seedSupersededQuestion(
      apiClient,
      seedData,
      "Unanswered Close",
      `pend-unanswered-close-${Date.now()}`,
    );

    await testPage.goto("/needs-you-inbox");
    const row = testPage
      .getByTestId("needs-you-unanswered-row")
      .filter({ hasText: "Unanswered Close" });
    await expect(row).toBeVisible();
    await row.getByTestId("needs-you-unanswered-close").click();
    await expect(row).toHaveCount(0);

    const { messages } = await apiClient.listSessionMessages(closed.sessionId);
    expect(messages.some((message) => message.author_type === "user")).toBe(false);
  });

  test("shows no unanswered section while the setting is off", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.updateWorkspace(seedData.workspaceId, {
      clarification_unanswered_inbox_enabled: false,
    });
    await seedSupersededQuestion(
      apiClient,
      seedData,
      "Unanswered Setting Off",
      `pend-unanswered-off-${Date.now()}`,
    );

    await testPage.goto("/needs-you-inbox");
    await testPage.getByTestId("inbox-tab-history").click();
    await expect(
      testPage.getByTestId("inbox-history-row").filter({ hasText: "Unanswered Setting Off" }),
    ).toBeVisible();
    await testPage.getByRole("tab", { name: /needs you/i }).click();
    await expect(testPage.getByTestId("needs-you-unanswered-section")).toHaveCount(0);
  });
});
