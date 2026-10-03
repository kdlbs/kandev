import { test, expect } from "../../fixtures/office-fixture";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

test.describe("mobile: Office session recovery required", () => {
  test("keeps the recovery notice reachable and contained", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "mobile-parked-recovery-task", {
      workflow_id: officeSeed.workflowId,
    });
    const session = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: officeSeed.agentId,
    });
    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      status: "failed",
      reason: "task_assigned",
      taskId: task.id,
      sessionId: session.session_id,
      errorMessage: "Session recovery is required",
    });

    await testPage.route(
      `**/api/v1/office/agents/${officeSeed.agentId}/runs/${run.run_id}`,
      async (route) => {
        const response = await route.fetch();
        const detail = await response.json();
        detail.routing = {
          ...(detail.routing ?? {}),
          blocked_status: "session_recovery_required",
          session_recovery_block_id: "mobile-recovery-block-e2e",
          session_recovery_reason: "native_state_missing",
        };
        await route.fulfill({ response, json: detail });
      },
    );

    await testPage.goto(`/office/agents/${officeSeed.agentId}/runs/${run.run_id}`);

    const notice = testPage.getByTestId("run-session-recovery-required");
    const link = testPage.getByTestId("run-session-recovery-link");
    await expect(notice).toBeVisible();
    await expect(link).toBeInViewport();
    await expect(link).toHaveAttribute(
      "href",
      `/office/tasks/${task.id}?advanced&session_id=${session.session_id}`,
    );
    const box = await link.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(36);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile Office recovery notice");
  });
});
