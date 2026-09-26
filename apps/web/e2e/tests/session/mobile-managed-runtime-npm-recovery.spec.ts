// Filename starts with "mobile-" so this runs on the mobile-chrome project.
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
  const task = await apiClient.createTask(seedData.workspaceId, "Mobile managed npm recovery", {
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

test("keeps managed npm recovery touch-safe on mobile", async ({
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

  const recovery = session.activeChat().getByTestId("managed-runtime-npm-recovery");
  await expect(recovery).toBeVisible();
  await expect(recovery.getByRole("button")).toHaveCount(1);
  const retry = recovery.getByTestId("managed-runtime-npm-retry-button");
  await expect(retry).toBeInViewport();
  const box = await retry.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.height).toBeGreaterThanOrEqual(44);
  await expect(recovery.locator("details")).not.toHaveAttribute("open");
  await expect(testPage.getByRole("dialog")).toHaveCount(0);

  await retry.tap();
  await expect
    .poll(() => sentFrames.find((frame) => frame.includes('"action":"session.recover"')) ?? "")
    .toContain('"action":"runtime_retry"');
  await expect(testPage.getByRole("dialog")).toHaveCount(0);
  await assertNoDocumentHorizontalOverflow(testPage, "mobile managed npm recovery");

  await testPage.screenshot({
    path: testInfo.outputPath("managed-runtime-npm-recovery-mobile.png"),
    fullPage: true,
  });
});

test("keeps release-age policy guidance touch-safe on mobile", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const task = await seedManagedRuntimeFailure(apiClient, seedData, "managed_runtime_npm_policy");
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  const recovery = session.activeChat().getByTestId("managed-runtime-npm-recovery");
  await expect(recovery).toBeVisible();
  await expect(
    recovery.getByRole("heading", { name: "npm blocked this runtime version" }),
  ).toBeVisible();
  await expect(recovery).toContainText(
    "Check npm's min-release-age or before setting. Wait until this version is eligible or select an older version, then retry.",
  );
  await expect(recovery.locator("details")).not.toHaveAttribute("open");
  const retry = recovery.getByTestId("managed-runtime-npm-retry-button");
  await expect(retry).toBeVisible();
  await prCapture.screenshot("managed-runtime-npm-release-age-policy-mobile", {
    caption: "Mobile recovery guidance for an npm release-age policy block",
    fullPage: true,
  });
  const box = await retry.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.height).toBeGreaterThanOrEqual(44);
  await retry.tap();
  await expect(testPage.getByRole("dialog")).toHaveCount(0);
});
