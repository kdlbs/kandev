import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

async function seedManagedRuntimeFailure(
  apiClient: ApiClient,
  seedData: SeedData,
  failureKind:
    | "managed_runtime_npm_resolution"
    | "managed_runtime_npm_policy" = "managed_runtime_npm_resolution",
) {
  const task = await apiClient.createTask(seedData.workspaceId, "Managed npm runtime recovery", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "FAILED",
    completedAt: "2026-08-16T09:15:44Z",
  });
  await apiClient.seedSessionMessage(sessionId, {
    type: "status",
    content: "managed npm runtime failed to prepare",
    metadata: {
      recovery_actions: true,
      failure_kind: failureKind,
      error_output:
        failureKind === "managed_runtime_npm_policy"
          ? "npm error notarget No matching version found. Minimum release age policy applies.\n  @example/agent@1.2.3 release date: <release-date>"
          : "npm error code ETARGET\nnpm error notarget No matching version found",
      actions: [
        {
          type: "ws_request",
          label: "Retry runtime",
          test_id: "managed-runtime-npm-retry-button",
          params: {
            method: "session.recover",
            payload: { task_id: task.id, session_id: sessionId, action: "runtime_retry" },
          },
        },
      ],
    },
  });
  return task;
}

test("renders managed npm recovery with one retry action", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  const sentFrames: string[] = [];
  testPage.on("websocket", (ws) => {
    if (!ws.url().endsWith("/ws")) return;
    ws.on("framesent", (event) => {
      if (typeof event.payload === "string") sentFrames.push(event.payload);
    });
  });

  const task = await seedManagedRuntimeFailure(apiClient, seedData);
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  const recovery = session.activeChat().getByTestId("session-recovery-card");
  await expect(recovery).toBeVisible();
  await expect(
    recovery.getByRole("heading", { name: "npm could not prepare the runtime" }),
  ).toBeVisible();
  await expect(recovery).toContainText(
    "Kandev refreshed package data and retried the same runtime once.",
  );
  await expect(recovery).not.toContainText("ACP");
  await expect(recovery.locator("details")).not.toHaveAttribute("open");
  await expect(recovery.getByTestId("managed-runtime-npm-retry-button")).toHaveCount(1);
  await expect(recovery.getByTestId("managed-runtime-npm-retry-button")).toHaveCount(1);

  await assertNoDocumentHorizontalOverflow(testPage, "managed npm recovery");
  await testPage.screenshot({
    path: testInfo.outputPath("managed-runtime-npm-recovery-desktop.png"),
    fullPage: true,
  });
  await recovery.getByText("Technical details", { exact: true }).click();
  await expect(recovery.locator("pre")).toBeVisible();
  await expect(recovery.locator("pre")).toContainText("ETARGET");

  await recovery.getByTestId("managed-runtime-npm-retry-button").click();
  await expect
    .poll(() => sentFrames.find((frame) => frame.includes('"action":"session.recover"')) ?? "")
    .toContain('"action":"runtime_retry"');
});

test("explains release-age policy failures with one retry action", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const task = await seedManagedRuntimeFailure(apiClient, seedData, "managed_runtime_npm_policy");
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  const recovery = session.activeChat().getByTestId("session-recovery-card");
  await expect(recovery).toBeVisible();
  await expect(
    recovery.getByRole("heading", { name: "npm blocked this runtime version" }),
  ).toBeVisible();
  await expect(recovery).toContainText(
    "Check npm's min-release-age or before setting. Wait until this version is eligible or select an older version, then retry.",
  );
  await expect(recovery).not.toContainText("refreshed package data");
  await expect(recovery.locator("details")).not.toHaveAttribute("open");
  await expect(recovery.getByRole("button")).toHaveCount(1);
  await expect(recovery.getByTestId("managed-runtime-npm-retry-button")).toBeVisible();
  await prCapture.screenshot("managed-runtime-npm-release-age-policy-desktop", {
    caption: "The recovery card explains an npm release-age policy block",
    fullPage: true,
  });
});
