import { test } from "../../fixtures/test-base";
import { waitForSessionDone } from "../../helpers/session";
import { expectTouchControl } from "../../helpers/control-sizing";
import { SessionPage } from "../../pages/session-page";

test("completed-session New Agent remains a touch target on mobile", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile control sizing completed session task",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for first session");
  await apiClient.seedTaskSession(task.id, {
    state: "COMPLETED",
    sessionId: task.session_id,
    agentProfileId: seedData.agentProfileId,
    completedAt: new Date().toISOString(),
  });

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expectTouchControl(session.completedSessionNewAgentButton());
});
