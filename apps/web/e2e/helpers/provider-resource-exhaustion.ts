import { test, expect } from "../fixtures/test-base";
import { waitForSessionDone } from "./session";
import { SessionPage } from "../pages/session-page";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

export function providerResourceExhaustionScenario() {
  test("resource exhaustion keeps a specific reason and hidden continuation on reload", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }, testInfo) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Compare completed inspection results",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("fixture session missing");
    try {
      await waitForSessionDone(
        apiClient,
        task.id,
        task.session_id,
        "resource exhaustion fixture settled",
      );
      await apiClient.seedSessionMessage(task.session_id, {
        type: "status",
        content: "Provider resources exhausted",
        metadata: {
          variant: "warning",
          retrying: true,
          recovery_mode: "continue",
          recovery_phase: "waiting",
          failure_code: "provider_resource_exhausted",
          provider_name: "Cursor",
          attempt: 1,
          max_attempts: 5,
          retry_at: new Date(Date.now() + 300_000).toISOString(),
          actions: [
            {
              type: "ws_request",
              label: "Cancel",
              icon: "x",
              test_id: "recovery-cancel-retry-button",
              params: {
                method: "session.recover",
                payload: { task_id: task.id, session_id: task.session_id, action: "cancel_retry" },
              },
            },
          ],
        },
      });
      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      const notice = session.transientRetryCard();
      await expect(notice).toContainText("Provider resources exhausted");
      await expect(notice).toContainText("Continuing in");
      await expect(session.activeChat().getByText("continue", { exact: true })).toHaveCount(0);
      await testPage.reload();
      await session.waitForLoad();
      await expect(notice).toContainText("Provider resources exhausted");
      await expect(session.recoveryCancelRetryButton()).toBeVisible();
      await assertNoDocumentHorizontalOverflow(testPage);
      if (testInfo.project.name === "mobile-chrome") {
        const box = await session.recoveryCancelRetryButton().boundingBox();
        expect(box?.height).toBeGreaterThanOrEqual(44);
      }
      await prCapture.screenshot("resource-exhaustion", {
        caption: "Resource exhaustion retains its specific reason and continuation countdown.",
      });
    } finally {
      await apiClient.deleteTask(task.id);
    }
  });
}
