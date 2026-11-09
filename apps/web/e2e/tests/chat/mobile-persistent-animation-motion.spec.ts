import { verifyReducedMotionFallback } from "./reduced-motion-helpers";
import { test, expect } from "../../fixtures/test-base";
import {
  expectCompositorMotionPaused,
  expectCompositorPulse,
} from "../../helpers/animation-assertions";
import { SessionPage } from "../../pages/session-page";
import { waitForSessionDone } from "../../helpers/session";
import { waitForActiveSessionForegroundActivity } from "../../helpers/session-store";

test("keeps the mobile composer pulse contained until the turn settles", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(60_000);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile persistent composer motion",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Expected a session for the motion check");
  await waitForSessionDone(
    apiClient,
    task.id,
    task.session_id,
    "initial motion test turn settled",
    30_000,
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  await session.sendMessageViaButton("/slow 8s");
  await waitForActiveSessionForegroundActivity(testPage, "generating", task.session_id);
  const glow = session.activeChat().getByTestId("chat-input-glow");
  await expectCompositorPulse(glow);

  await glow.evaluate((element) => {
    (element as HTMLElement).style.display = "none";
  });
  await expectCompositorMotionPaused(glow);
  await glow.evaluate((element) => {
    (element as HTMLElement).style.removeProperty("display");
  });
  await expectCompositorPulse(glow);

  await glow.evaluate((element) => {
    (element as HTMLElement).style.transform = "translateY(200vh)";
  });
  await expectCompositorMotionPaused(glow);
  await glow.evaluate((element) => {
    element.scrollIntoView({ block: "center", inline: "nearest" });
  });
  await expectCompositorPulse(glow);
  await glow.evaluate((element) => {
    (element as HTMLElement).style.removeProperty("transform");
  });

  await glow.evaluate((element) => {
    (element as HTMLElement).style.display = "none";
  });
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);

  await waitForSessionDone(
    apiClient,
    task.id,
    task.session_id,
    "slow motion test turn settled",
    30_000,
  );
  await expect(glow).toHaveCount(0);
});

test("keeps static status visible with reduced motion in the CSS fallback", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(60_000);
  await verifyReducedMotionFallback(testPage, apiClient, seedData, prCapture);
});
