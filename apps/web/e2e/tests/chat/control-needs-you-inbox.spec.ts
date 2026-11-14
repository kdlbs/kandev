// The integrated control view gathers Needs-you items from every workspace:
// the navigation Inbox entry carries the pending count, the summary's
// attention list raises the waiting question, and the inbox view lists it in
// the answerable section and answers it in place through the existing row.
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { waitForAgentMessage, waitForSessionState } from "../../helpers/session";

const CAPTURE_DIR = process.env.CONTROL_INBOX_CAPTURE_DIR;

test.describe.serial("Integrated control Needs-you inbox", () => {
  test.beforeAll(async ({ backend }) => {
    await backend.restart({ KANDEV_FEATURES_INTEGRATED_CONTROL: "true" });
    const response = await fetch(`${backend.baseUrl}/api/v1/features`);
    expect(response.ok).toBeTruthy();
    expect(await response.json()).toMatchObject({ integratedControl: true, needsYouInbox: true });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("lists a waiting question across workspaces and answers it in place", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "Control Inbox Answer Flow";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      title,
      seedData.agentProfileId,
      {
        description: "/e2e:clarification",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected an active session for the control inbox flow");
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "clarification session should block before the control inbox is opened",
      timeout: 60_000,
    });

    await testPage.goto("/control/overview");
    const attention = testPage
      .getByTestId("control-attention-row")
      .filter({ has: testPage.locator('[data-state="question"]') });
    await expect(attention).toHaveCount(1, { timeout: 30_000 });
    await expect(testPage.getByTestId("control-nav-inbox-count")).toHaveText("1");
    if (CAPTURE_DIR)
      await testPage.screenshot({ path: path.join(CAPTURE_DIR, "control-overview-attention.png") });

    await testPage.getByTestId("control-nav-inbox").click();
    await expect(testPage).toHaveURL(/\/control\/inbox/);
    const answerable = testPage.getByTestId("control-inbox-section-answerable");
    const row = answerable.getByTestId("needs-you-inbox-row").filter({ hasText: title });
    await expect(row).toBeVisible({ timeout: 30_000 });
    const workspaceGroup = answerable.getByTestId("control-inbox-workspace").filter({
      has: testPage.getByTestId("needs-you-inbox-row").filter({ hasText: title }),
    });
    await expect(workspaceGroup).toHaveAttribute("data-workspace-id", seedData.workspaceId);
    await expect(testPage.getByTestId("control-inbox-section-earlier")).toBeVisible();
    await expect(testPage.getByTestId("control-inbox-section-recovery")).toBeVisible();
    if (CAPTURE_DIR)
      await testPage.screenshot({
        path: path.join(CAPTURE_DIR, "control-inbox.png"),
        fullPage: true,
      });

    await row.getByTestId("needs-you-inbox-row-toggle").click();
    const overlay = row.getByTestId("clarification-overlay");
    await expect(overlay).toBeVisible({ timeout: 15_000 });
    await overlay.getByTestId("clarification-option").filter({ hasText: "PostgreSQL" }).click();

    await expect(row).toHaveCount(0, { timeout: 30_000 });
    await expect(testPage.getByTestId("control-nav-inbox-count")).toHaveCount(0, {
      timeout: 30_000,
    });
    await waitForAgentMessage(apiClient, task.session_id, "You answered:", 30_000);
  });
});
