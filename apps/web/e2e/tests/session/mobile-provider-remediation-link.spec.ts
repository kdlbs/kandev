// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

const REMEDIATION_URL = "https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go";

async function seedRecoveryWithLink(apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTask(
    seedData.workspaceId,
    "Mobile OpenCode remediation link",
    {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    },
  );
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "FAILED",
    completedAt: "2026-08-07T15:15:44Z",
  });
  await apiClient.seedSessionMessage(sessionId, {
    type: "status",
    content: "OpenCode stopped responding after the provider rejected the stream.",
    metadata: {
      recovery_actions: true,
      failure_kind: "provider_quota_limited",
      provider_name: "OpenCode",
      error_output: "5-hour usage limit reached. Resets in 4hr 19min.",
      remediation_url: REMEDIATION_URL,
      actions: [
        {
          type: "archive_task",
          label: "Archive task",
          test_id: "provider-quota-archive-button",
        },
      ],
    },
  });
  return task;
}

test("keeps the OpenCode recovery link touch-safe on mobile", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}, testInfo) => {
  const task = await seedRecoveryWithLink(apiClient, seedData);

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  const recovery = session.activeChat().getByTestId("provider-quota-recovery");
  await expect(recovery).toBeVisible();

  const link = recovery.getByTestId("remediation-link");
  await expect(link).toBeVisible();
  await expect(link).toHaveAttribute("href", REMEDIATION_URL);
  await expect(link).toHaveAttribute("target", "_blank");
  await expect(link).toHaveAttribute("rel", "noopener noreferrer");

  // 44px minimum touch target on phones, like the recovery buttons beside it.
  const box = await link.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.height).toBeGreaterThanOrEqual(44);

  // Keyboard reachable via the native anchor focus path.
  await link.focus();
  await expect(link).toBeFocused();

  // The sanitized message and technical details never carry the URL.
  await expect(recovery).not.toContainText("opencode.ai");
  await expect(recovery).not.toContainText("wrk_");
  const technicalOutput = recovery.locator("details").locator("pre");
  await recovery.getByText("Technical details", { exact: true }).tap();
  await expect(technicalOutput).toBeVisible();
  await expect(technicalOutput).not.toContainText("https://opencode.ai");
  await expect(technicalOutput).not.toContainText("wrk_");
  await expect(technicalOutput).not.toContainText("ses_");

  await expect(recovery.getByTestId("provider-quota-archive-button")).toBeInViewport();

  await assertNoDocumentHorizontalOverflow(testPage, "mobile provider remediation link");
  await testPage.screenshot({
    path: testInfo.outputPath("provider-remediation-link-mobile.png"),
    fullPage: true,
  });
  await prCapture.screenshot("provider-remediation-link-mobile", {
    caption: "OpenCode recovery link stays touch-safe on mobile",
    fullPage: true,
  });
});
