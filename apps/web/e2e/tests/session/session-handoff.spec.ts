import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

const HANDOFF_SUMMARY = "Handoff summary: completed the prior session work.";

async function mockSummarizeUtility(testPage: import("@playwright/test").Page) {
  let summarizeRequestCount = 0;
  await testPage.route("**/api/v1/utility/execute", async (route) => {
    const request = route.request().postDataJSON() as { utility_agent_id?: string };
    if (request.utility_agent_id === "builtin-summarize-session") summarizeRequestCount += 1;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        response: HANDOFF_SUMMARY,
      }),
    });
  });
  return () => summarizeRequestCount;
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
    const getSummarizeRequestCount = await mockSummarizeUtility(testPage);

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

    const handoffDialog = session.handoffDialog();
    await expect(handoffDialog).toBeVisible({ timeout: 5_000 });
    await expect(handoffDialog).toContainText("Hand off to");
    await expect(handoffDialog).toContainText("Handoff Profile B");

    const prompt = session.newSessionPromptInput();
    const contextTrigger = handoffDialog.locator("button").filter({ hasText: "Blank" });
    await expect(contextTrigger).toBeVisible();
    await expect(prompt).toHaveValue("");
    await expect(session.newSessionStartButton()).toBeDisabled();
    await dwell(
      testPage,
      500,
      "negative-assertion",
      "prove opening a handoff does not invoke the summary utility",
    );
    expect(getSummarizeRequestCount()).toBe(0);

    await contextTrigger.click();
    const summaryOption = testPage.getByRole("option", {
      name: /Handoff Profile A/,
    });
    await expect(summaryOption).toBeVisible();
    await summaryOption.click();
    await expect(prompt).toHaveValue(HANDOFF_SUMMARY, { timeout: 15_000 });
    await expect.poll(getSummarizeRequestCount).toBe(1);

    await handoffDialog.getByRole("button", { name: "Cancel" }).click();
    await expect(handoffDialog).not.toBeVisible({ timeout: 5_000 });

    await session.openHandoffDialog(session1Id, profileB.id);
    await expect(session.handoffDialog()).toBeVisible({ timeout: 5_000 });
    await expect(session.newSessionPromptInput()).toHaveValue("");
    await expect(
      session.handoffDialog().locator("button").filter({ hasText: "Blank" }),
    ).toBeVisible();
    expect(getSummarizeRequestCount()).toBe(1);

    await prompt.fill("/e2e:simple-message");
    await session.newSessionStartButton().click();
    await expect(session.handoffDialog()).not.toBeVisible({ timeout: 15_000 });

    let handoffSessionId: string | undefined;
    await expect
      .poll(
        async () => {
          const { sessions: updated } = await apiClient.listTaskSessions(task.id);
          const handoffSession = updated.find(({ id }) => id !== session1Id);
          handoffSessionId = handoffSession?.id;
          if (!handoffSession) return null;

          const { messages } = await apiClient.listSessionMessages(handoffSession.id);
          return messages.find((message) => message.author_type === "user")?.content ?? null;
        },
        { timeout: 30_000, message: "Waiting for handoff session to be created" },
      )
      .toBe("/e2e:simple-message");

    const { sessions: updated } = await apiClient.listTaskSessions(task.id);
    const handoffSession = updated.find(({ id }) => id !== session1Id);
    expect(handoffSession?.id).toBe(handoffSessionId);
    expect(handoffSession?.agent_profile_id).toBe(profileB.id);
    expect(getSummarizeRequestCount()).toBe(1);
  });
});
