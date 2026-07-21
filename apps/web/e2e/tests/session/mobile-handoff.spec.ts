import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

async function createProfiles(
  apiClient: InstanceType<typeof import("../../helpers/api-client").ApiClient>,
) {
  const { agents } = await apiClient.listAgents();
  if (agents.length === 0) throw new Error("no agents available in test fixtures");
  const agentId = agents[0].id;
  const profileA = await apiClient.createAgentProfile(agentId, "Mobile Handoff A", {
    model: "mock-fast",
  });
  const profileB = await apiClient.createAgentProfile(agentId, "Mobile Handoff B", {
    model: "mock-slow",
  });
  return { profileA, profileB, agentName: agents[0].name };
}

test.describe("Session handoff on mobile", () => {
  test("opens handoff dialog from mobile session actions menu", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { profileA, profileB, agentName } = await createProfiles(apiClient);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Handoff Task",
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
        { timeout: 30_000 },
      )
      .toBe(true);

    const { sessions } = await apiClient.listTaskSessions(task.id);
    const session1Id = sessions[0].id;

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const activeSessionPill = testPage.getByTestId("mobile-sessions-pill");
    await expect(activeSessionPill.getByTestId("mobile-session-agent-icon")).toHaveAttribute(
      "data-agent-name",
      agentName,
    );

    await session.openMobileHandoffDialog(session1Id, profileB.id);

    await expect(session.handoffDialog()).toBeVisible({ timeout: 5_000 });
    await expect(session.handoffDialog()).toContainText("Hand off to");
    await expect(session.handoffDialog()).toContainText("Mobile Handoff B");
  });
});
