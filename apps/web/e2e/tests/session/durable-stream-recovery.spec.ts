import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { attachGatewayTrafficCapture } from "../../helpers/ws-traffic";
import { routeSessionEntryRecovery } from "../../helpers/session-entry-recovery";
import { assertDeliveryRecoveryGeometry } from "../../helpers/delivery-recovery-controls";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

async function seedUncertainDeliverySession(apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTask(seedData.workspaceId, "Durable stream recovery", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    agent_profile_id: seedData.agentProfileId,
    repository_ids: [seedData.repositoryId],
  });
  const sessionId = `durable-recovery-${task.id}`;
  await apiClient.seedTaskSession(task.id, {
    state: "FAILED",
    sessionId,
    agentProfileId: seedData.agentProfileId,
    repositoryId: seedData.repositoryId,
    completedAt: new Date().toISOString(),
    metadata: {
      last_agent_error: {
        message: "Delivery was interrupted.",
        code: "DURABLE_DELIVERY_UNCERTAIN",
        occurred_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
      },
    },
  });
  await expect
    .poll(async () => {
      const { sessions } = await apiClient.listTaskSessions(task.id);
      const session = sessions.find((item) => item.id === sessionId);
      const error = session?.metadata?.last_agent_error as { code?: string } | undefined;
      return error?.code ?? "";
    })
    .toBe("DURABLE_DELIVERY_UNCERTAIN");
  return { task, sessionId };
}

test("surfaces uncertain delivery with state-only retry and Stop", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const capture = attachGatewayTrafficCapture(testPage);
  const { task, sessionId } = await seedUncertainDeliverySession(apiClient, seedData);
  const proxy = await routeSessionEntryRecovery(testPage);
  proxy.holdResponses("session.recover", { sessionId });
  proxy.rejectResponsesUntilReleased("session.stop", "Stop was refused", { sessionId });

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  const banner = testPage.getByTestId("failed-session-banner");
  await expect(banner).toBeVisible({ timeout: 30_000 });
  await expect(banner).toContainText("Delivery was interrupted. The prompt outcome is uncertain.");
  await expect(banner.getByTestId("recovery-resume-button")).toHaveCount(0);
  await expect(banner.getByTestId("recovery-fresh-button")).toHaveCount(0);

  const retryBox = await banner.getByTestId("recovery-retry-connection-button").boundingBox();
  const stopBox = await banner.getByTestId("recovery-stop-button").boundingBox();
  expect(retryBox).not.toBeNull();
  expect(stopBox).not.toBeNull();
  expect(Math.abs(retryBox!.y - stopBox!.y)).toBeLessThanOrEqual(1);
  expect(Math.abs(retryBox!.height - stopBox!.height)).toBeLessThanOrEqual(1);
  await banner.getByTestId("recovery-retry-connection-button").click();
  await expect.poll(() => proxy.heldResponseCount("session.recover")).toBe(1);
  for (const width of [767, 768, 1280]) {
    await testPage.setViewportSize({ width, height: 900 });
    await assertDeliveryRecoveryGeometry(testPage);
  }
  await expect(banner.getByTestId("recovery-stop-button")).toBeEnabled();
  await banner.getByTestId("recovery-stop-button").click();
  await expect(banner.getByTestId("delivery-stop-failed")).toBeVisible();
  await assertDeliveryRecoveryGeometry(testPage);
  proxy.releaseHeldResponses("session.recover");
  await banner.getByTestId("recovery-retry-connection-button").click();
  await expect(banner.getByTestId("delivery-recovery-result")).toBeVisible({ timeout: 30_000 });
  await expect
    .poll(
      () =>
        capture.frames.some(
          (frame) =>
            frame.direction === "sent" &&
            frame.action === "session.recover" &&
            frame.sessionId === sessionId,
        ),
      { timeout: 10_000, message: "retry must use session.recover for the original session" },
    )
    .toBe(true);

  await testPage.evaluate(() => {
    document.cookie = "kandev_locale=pseudo; path=/; SameSite=Lax";
  });
  await testPage.reload();
  await assertDeliveryRecoveryGeometry(testPage);
  await expect
    .poll(
      () =>
        capture.frames.some(
          (frame) =>
            frame.direction === "sent" &&
            frame.action === "session.stop" &&
            frame.sessionId === sessionId,
        ),
      { timeout: 10_000, message: "Stop must remain reachable after retry failure" },
    )
    .toBe(true);
});
