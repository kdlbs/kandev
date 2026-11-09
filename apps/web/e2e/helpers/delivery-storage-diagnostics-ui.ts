import type { Page } from "@playwright/test";
import { expect, test } from "../fixtures/test-base";

export function deliveryStorageFailureScenario() {
  test("shows the storage cause, eligible settings, and history after reload", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }, testInfo) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Storage recovery", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    const stamp = "delivery-storage-fixture";
    const occurredAt = new Date().toISOString();
    const message = "The agent could not continue because the disk is full.";
    const causes = [
      {
        operation: "delivery",
        code: "delivery_storage",
        storage: {
          reason: "disk_space_low",
          location: "local_runtime",
          observed_at: occurredAt,
          disk_available_bytes: 1024,
          disk_required_bytes: 2048,
        },
      },
    ];
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "FAILED",
      completedAt: occurredAt,
      agentProfileId: seedData.agentProfileId,
      errorMessage: message,
      metadata: {
        last_agent_error: {
          message,
          stamp,
          scope: "session",
          code: "STORAGE_FULL",
          occurred_at: occurredAt,
          causes,
        },
      },
    });
    await apiClient.seedSessionMessage(sessionId, {
      type: "status",
      content: message,
      createdAt: occurredAt,
      metadata: {
        recovery_actions: true,
        scope: "session",
        error_stamp: stamp,
        causes,
        actions: [
          {
            type: "ws_request",
            label: "Resume session",
            params: {
              method: "session.recover",
              payload: { task_id: task.id, session_id: sessionId, action: "resume" },
            },
          },
        ],
      },
    });

    await testPage.goto(`/t/${task.id}`);
    await expectStorageFailureCard(testPage);
    if (testInfo.project.name === "mobile-chrome") {
      const settings = testPage.getByTestId("recovery-storage-settings-link");
      expect((await settings.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    }
    await testPage.reload();
    await expectStorageFailureCard(testPage);
    await prCapture.screenshot(`delivery-storage-failure-${testInfo.project.name}`, {
      caption: "Low disk failure keeps its cause and recovery action after reload.",
    });
  });
}

async function expectStorageFailureCard(page: Page) {
  const card = page.getByTestId("session-recovery-card");
  await expect(card).toHaveCount(1);
  await expect(
    card.getByRole("heading", { name: "Session interrupted by low disk space" }),
  ).toBeVisible();
  await expect(card).toContainText("Free space on this host");
  await expect(card.getByTestId("recovery-storage-settings-link")).toHaveAttribute(
    "href",
    "/settings/system/storage",
  );
  await expect(card.locator("pre")).toBeHidden();
}
