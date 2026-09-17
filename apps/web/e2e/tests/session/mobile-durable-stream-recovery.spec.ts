import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { attachGatewayTrafficCapture } from "../../helpers/ws-traffic";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

async function seedUncertainDeliverySession(apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTask(seedData.workspaceId, "Mobile durable recovery", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const sessionId = `mobile-durable-recovery-${task.id}`;
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

test("keeps uncertain delivery recovery usable on a phone", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const capture = attachGatewayTrafficCapture(testPage);
  const { task, sessionId } = await seedUncertainDeliverySession(apiClient, seedData);

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

  await banner.getByTestId("recovery-retry-connection-button").tap();
  await expect(session.recoveryError()).toBeVisible({ timeout: 30_000 });
  await banner.getByTestId("recovery-stop-button").tap();
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
