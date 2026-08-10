import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

const REMEDIATION_URL = "https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go";
const RECOVERY_MESSAGE =
  "OpenCode stopped responding after the provider rejected the stream. No URL should ever appear in this message.";

async function seedRecoveryWithLink(apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTask(seedData.workspaceId, "OpenCode remediation link", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "FAILED",
    completedAt: "2026-08-07T15:15:44Z",
  });
  await apiClient.seedSessionMessage(sessionId, {
    type: "status",
    content: RECOVERY_MESSAGE,
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

test("renders the allowlisted OpenCode recovery link without leaking it into the message", async ({
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
  await expect(link).toContainText("Open provider page");

  // The URL must never leak into the card, the sanitized message, or details.
  await expect(recovery).not.toContainText("opencode.ai");
  await expect(recovery).not.toContainText("wrk_");
  await expect(recovery).toContainText("5-hour usage limit reached");

  // The link is keyboard-reachable as a native anchor.
  await link.focus();
  await expect(link).toBeFocused();

  const technicalOutput = recovery.locator("details").locator("pre");
  await recovery.getByText("Technical details", { exact: true }).click();
  await expect(technicalOutput).toBeVisible();
  await expect(technicalOutput).toContainText("5-hour usage limit reached");
  await expect(technicalOutput).not.toContainText("https://opencode.ai");
  await expect(technicalOutput).not.toContainText("wrk_");
  await expect(technicalOutput).not.toContainText("ses_");

  // Existing recovery actions stay intact beside the link.
  await expect(recovery.getByTestId("provider-quota-archive-button")).toBeVisible();
  await expect(recovery.getByTestId("provider-quota-delete-button")).toBeVisible();

  await assertNoDocumentHorizontalOverflow(testPage, "provider remediation link");
  await testPage.screenshot({
    path: testInfo.outputPath("provider-remediation-link-desktop.png"),
    fullPage: true,
  });
  await prCapture.screenshot("provider-remediation-link-desktop", {
    caption: "OpenCode recovery link renders separately from the sanitized message",
    fullPage: true,
  });
});

test("keeps the short-error fallback when no remediation URL is present", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  // Mirrors OpenCode 1.18.5's ACP service-failure path: only the short safe
  // message arrives, so Kandev must show it without inventing a link.
  const task = await apiClient.createTask(seedData.workspaceId, "OpenCode short error fallback", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "FAILED",
    completedAt: "2026-08-07T15:15:44Z",
  });
  await apiClient.seedSessionMessage(sessionId, {
    type: "status",
    content: "AI_APICallError: provider rejected the request.",
    metadata: {
      recovery_actions: true,
      actions: [
        {
          type: "archive_task",
          label: "Archive task",
          test_id: "provider-quota-archive-button",
        },
      ],
    },
  });

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  // The generic recovery card keeps the short message and offers no link.
  await expect(session.activeChat()).toContainText("AI_APICallError");
  await expect(session.activeChat().getByTestId("remediation-link")).toHaveCount(0);
  await expect(session.activeChat().getByTestId("provider-quota-archive-button")).toBeVisible();
});

test("renders the remediation link inside the persistent last-agent-error notice", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Last agent error link", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await apiClient.seedTaskSession(task.id, {
    state: "WAITING_FOR_INPUT",
    metadata: {
      last_agent_error: {
        message: "AI_APICallError: 5-hour usage limit reached.",
        occurred_at: "2026-08-07T15:15:44Z",
        remediation_url: REMEDIATION_URL,
      },
    },
  });

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  const notice = testPage.getByTestId("last-agent-error-notice");
  await expect(notice).toBeVisible({ timeout: 15_000 });
  const link = notice.getByTestId("remediation-link");
  await expect(link).toBeVisible();
  await expect(link).toHaveAttribute("href", REMEDIATION_URL);
  await expect(link).toHaveAttribute("rel", "noopener noreferrer");
  await expect(notice).not.toContainText("opencode.ai/workspace");
});
