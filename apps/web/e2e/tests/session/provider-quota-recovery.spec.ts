import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

async function seedProviderQuotaRecovery(apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTask(seedData.workspaceId, "OpenCode quota recovery", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "FAILED",
    completedAt: "2026-08-02T15:15:44Z",
  });
  await apiClient.seedSessionMessage(sessionId, {
    type: "status",
    content: "OpenCode stopped responding after the provider rejected the stream.",
    metadata: {
      recovery_actions: true,
      failure_kind: "provider_quota_limited",
      provider_name: "OpenCode",
      model_id: "kimi-k3",
      reset_at: "2026-08-02T16:30:00Z",
      error_output: "5-hour usage limit reached. Resets in 4hr 19min.",
      actions: [
        {
          type: "archive_task",
          label: "Archive task",
          test_id: "provider-quota-archive-button",
        },
        {
          type: "delete_task",
          label: "Delete task",
          test_id: "provider-quota-delete-button",
          variant: "destructive",
        },
      ],
    },
  });
  return task;
}

test("renders a localized OpenCode quota recovery card", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}, testInfo) => {
  const task = await seedProviderQuotaRecovery(apiClient, seedData);

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  const recovery = session.activeChat().getByTestId("provider-quota-recovery");
  await expect(recovery).toHaveCount(1);
  await expect(
    recovery.getByRole("heading", { name: "OpenCode usage limit reached" }),
  ).toBeVisible();
  await expect(recovery).toContainText("kimi-k3");
  await expect(recovery).toContainText("Capacity resets");
  await expect(session.activeChat().getByRole("status", { name: /Session failed/i })).toHaveCount(
    0,
  );

  const technicalDetails = recovery.locator("details");
  const technicalOutput = technicalDetails.locator("pre");
  await expect(technicalDetails).not.toHaveAttribute("open");
  await expect(technicalOutput).not.toBeVisible();
  await recovery.getByText("Technical details", { exact: true }).click();
  await expect(technicalOutput).toBeVisible();
  await expect(technicalOutput).toContainText("5-hour usage limit reached");
  await expect(technicalOutput).not.toContainText("https://opencode.ai");
  await expect(technicalOutput).not.toContainText("wrk_");
  await expect(technicalOutput).not.toContainText("ses_");

  for (const button of [
    recovery.getByTestId("provider-quota-archive-button"),
    recovery.getByTestId("provider-quota-delete-button"),
  ]) {
    await expect(button).toBeVisible();
    const box = await button.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThan(0);
  }

  await assertNoDocumentHorizontalOverflow(testPage, "provider quota recovery");
  await testPage.screenshot({
    path: testInfo.outputPath("provider-quota-recovery-desktop.png"),
    fullPage: true,
  });
  await prCapture.screenshot("provider-quota-recovery-desktop", {
    caption: "OpenCode quota recovery with model and reset guidance",
    fullPage: true,
  });
});
