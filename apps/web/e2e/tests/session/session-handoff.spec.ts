import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

const HANDOFF_SUMMARY = "Handoff summary: completed the prior session work.";

async function mockSummarizeUtility(testPage: import("@playwright/test").Page) {
  await testPage.route("**/api/v1/utility/execute", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        response: HANDOFF_SUMMARY,
      }),
    });
  });
}

async function createProfiles(
  apiClient: InstanceType<typeof import("../../helpers/api-client").ApiClient>,
) {
  const { agents } = await apiClient.listAgents();
  if (agents.length === 0) throw new Error("no agents available in test fixtures");
  const agentId = agents[0].id;
  const profileA = await apiClient.createAgentProfile(agentId, "Handoff Profile A", {
    model: "mock-fast",
  });
  const profileB = await apiClient.createAgentProfile(agentId, "Handoff Profile B", {
    model: "mock-slow",
  });
  return { profileA, profileB };
}

test.describe("Session handoff", () => {
  test("opens handoff dialog with target profile from session tab context menu", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { profileA, profileB } = await createProfiles(apiClient);
    await mockSummarizeUtility(testPage);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Session Handoff Task",
      profileA.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    const { sessions } = await apiClient.listTaskSessions(task.id);
    const session1Id = sessions[0].id;

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.taskCardByTitle("Session Handoff Task").click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    await session.openHandoffDialog(session1Id, profileB.id);

    await expect(session.handoffDialog()).toBeVisible({ timeout: 5_000 });
    await expect(session.handoffDialog()).toContainText("Hand off to");
    await expect(session.handoffDialog()).toContainText("Handoff Profile B");

    const prompt = session.newSessionPromptInput();
    await expect(prompt).toHaveValue(HANDOFF_SUMMARY, { timeout: 15_000 });

    await prompt.fill(`${HANDOFF_SUMMARY}\n/e2e:simple-message`);
    await session.newSessionStartButton().click();
    await expect(session.handoffDialog()).not.toBeVisible({ timeout: 15_000 });

    await expect
      .poll(
        async () => {
          const { sessions: updated } = await apiClient.listTaskSessions(task.id);
          return updated.length;
        },
        { timeout: 30_000, message: "Waiting for handoff session to be created" },
      )
      .toBe(2);
  });
});
