import type { Locator } from "@playwright/test";

import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

async function expectCompositorPulse(pulse: Locator) {
  await expect(pulse).toBeVisible();
  await expect
    .poll(() =>
      pulse.evaluate((element) => {
        const animations = element.getAnimations();
        return (
          animations.length === 1 &&
          animations[0].playState === "running" &&
          animations[0].effect?.getTiming().iterations === Infinity &&
          animations[0].constructor.name === "Animation"
        );
      }),
    )
    .toBe(true);
}

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
