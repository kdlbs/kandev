import { test, expect } from "../../fixtures/test-base";
import { expectCompositorPulse } from "../../helpers/animation-assertions";
import { SessionPage } from "../../pages/session-page";

test("keeps the busy task composer glow animated until the turn settles", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(60_000);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Persistent composer motion",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  await session.sendMessage("/slow 8s");
  const glow = session.activeChat().getByTestId("chat-input-glow");
  await expectCompositorPulse(glow);

  await session.waitForChatIdle({ timeout: 30_000 });
  await expect(glow).toHaveCount(0);
});
