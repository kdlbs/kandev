import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { attachGatewayTrafficCapture } from "../../helpers/ws-traffic";
import { routeSessionEntryRecovery } from "../../helpers/session-entry-recovery";
import { assertDeliveryRecoveryGeometry } from "../../helpers/delivery-recovery-controls";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

async function seedUncertainDeliverySession(apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTask(seedData.workspaceId, "Mobile durable recovery", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    agent_profile_id: seedData.agentProfileId,
    repository_ids: [seedData.repositoryId],
  });
  const sessionId = `mobile-durable-recovery-${task.id}`;
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

test("keeps uncertain delivery recovery usable on a phone", async ({
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

  const layout = testPage.locator("[data-testid='mobile-task-layout']:visible");
  await expect(layout).toBeVisible({ timeout: 30_000 });
  const banner = testPage.getByTestId("failed-session-banner");
  await expect(banner).toBeVisible({ timeout: 30_000 });
  await expect(banner).toContainText("Delivery was interrupted. The prompt outcome is uncertain.");
  await expect(banner.getByTestId("recovery-retry-connection-button")).toBeVisible();
  await expect(banner.getByTestId("recovery-stop-button")).toBeVisible();
  await expect(banner.getByTestId("recovery-resume-button")).toHaveCount(0);
  await expect(testPage.getByTestId("interrupted-sessions-notice")).toHaveCount(0);
  await expect(testPage.getByTestId("interrupted-session-continuation")).toHaveCount(0);

  await banner.getByTestId("recovery-retry-connection-button").tap();
  await expect.poll(() => proxy.heldResponseCount("session.recover")).toBe(1);
  await assertDeliveryRecoveryGeometry(testPage);
  await banner.getByTestId("recovery-stop-button").tap();
  await expect(banner.getByTestId("delivery-stop-failed")).toBeVisible();
  await assertDeliveryRecoveryGeometry(testPage);
  proxy.releaseHeldResponses("session.recover");
  await expect(banner.getByTestId("delivery-recovery-result")).toBeVisible({ timeout: 30_000 });
  await expect(testPage.getByTestId("interrupted-session-continuation")).toHaveCount(0);
  for (const id of ["recovery-retry-connection-button", "recovery-stop-button"]) {
    const box = await banner.getByTestId(id).boundingBox();
    expect(box?.height).toBeGreaterThanOrEqual(44);
  }
  await testPage.evaluate(() => {
    document.cookie = "kandev_locale=pseudo; path=/; SameSite=Lax";
  });
  await testPage.reload();
  await assertDeliveryRecoveryGeometry(testPage);
  await testPage.setViewportSize({ width: 1024, height: 900 });
  expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
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
      { timeout: 10_000, message: "mobile Stop must remain reachable during recovery" },
    )
    .toBe(true);

  await assertNoDocumentHorizontalOverflow(testPage, "mobile durable stream recovery");
});
