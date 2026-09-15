import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { attachGatewayTrafficCapture } from "../../helpers/ws-traffic";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

async function seedUncertainDeliverySession(apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTask(seedData.workspaceId, "Durable stream recovery", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const sessionId = `durable-recovery-${task.id}`;
  await apiClient.seedTaskSession(task.id, {
    state: "FAILED",
    sessionId,
    completedAt: new Date().toISOString(),
    metadata: {
      last_agent_error: {
        message: "Delivery was interrupted.",
        code: "DURABLE_DELIVERY_UNCERTAIN",
        occurred_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
      },
    },
  });
  return { task, sessionId };
}

test("surfaces uncertain delivery with state-only retry and Stop", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const capture = attachGatewayTrafficCapture(testPage);
  const { task, sessionId } = await seedUncertainDeliverySession(apiClient, seedData);

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  const banner = testPage.getByTestId("failed-session-banner");
  await expect(banner).toBeVisible({ timeout: 30_000 });
  await expect(banner).toContainText("Delivery was interrupted. The prompt outcome is uncertain.");
  await expect(banner.getByTestId("recovery-resume-button")).toHaveCount(0);
  await expect(banner.getByTestId("recovery-fresh-button")).toHaveCount(0);

  await banner.getByTestId("recovery-retry-connection-button").click();
  await expect(session.recoveryError()).toBeVisible({ timeout: 30_000 });
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

  await banner.getByTestId("recovery-stop-button").click();
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
