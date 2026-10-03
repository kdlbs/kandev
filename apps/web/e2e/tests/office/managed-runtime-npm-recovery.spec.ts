import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/office-fixture";

async function interceptSessionRecovery(page: Page): Promise<string[]> {
  const sentFrames: string[] = [];
  await page.routeWebSocket(/\/ws$/, (ws) => {
    const server = ws.connectToServer();

    ws.onMessage((message) => {
      if (typeof message !== "string") {
        server.send(message);
        return;
      }

      for (const part of message.split("\n")) {
        const frameText = part.trim();
        if (!frameText) continue;

        let frame: { action?: unknown } | null = null;
        try {
          frame = JSON.parse(frameText) as { action?: unknown };
        } catch {
          server.send(frameText);
          continue;
        }

        if (frame?.action === "session.recover") {
          // This spec verifies the recovery request shape; forwarding it would
          // start an agent run that outlives the test's seeded failed session.
          sentFrames.push(frameText);
          continue;
        }

        server.send(frameText);
      }
    });

    server.onMessage((message) => ws.send(message));
  });
  return sentFrames;
}

test("keeps managed npm recovery visible in Office chat", async ({
  testPage,
  apiClient,
  officeSeed,
}) => {
  const sentFrames = await interceptSessionRecovery(testPage);

  const task = await apiClient.createTask(officeSeed.workspaceId, "Office managed npm recovery", {
    workflow_id: officeSeed.workflowId,
  });
  await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
    assignee_agent_profile_id: officeSeed.agentId,
  });
  await apiClient.seedTaskSession(task.id, {
    state: "FAILED",
    agentProfileId: officeSeed.agentId,
    completedAt: "2026-08-16T09:15:44Z",
    metadata: {
      last_agent_error: {
        message: "managed npm runtime failed to prepare",
        code: "managed_runtime_npm_resolution",
        details: "npm error code ETARGET\nnpm error notarget No matching version found",
      },
    },
  });

  await testPage.goto(`/office/tasks/${task.id}`);
  await expect(
    testPage.getByRole("heading", { name: "Office managed npm recovery" }),
  ).toBeVisible();

  const recovery = testPage.getByTestId("run-error-managed-runtime-npm-recovery");
  await expect(recovery).toBeVisible();
  await expect(recovery.getByText("npm could not prepare the runtime")).toBeVisible();
  await expect(recovery.getByTestId("run-error-managed-runtime-retry-button")).toHaveCount(1);
  await expect(recovery.getByTestId("run-error-resume-button")).toHaveCount(0);
  await expect(recovery.getByTestId("run-error-fresh-button")).toHaveCount(0);
  await expect(recovery.getByRole("button", { name: "Technical details" })).toHaveAttribute(
    "aria-expanded",
    "false",
  );

  await recovery.getByTestId("run-error-managed-runtime-retry-button").click();
  await expect
    .poll(() => sentFrames.find((frame) => frame.includes('"action":"session.recover"')) ?? "")
    .toContain('"action":"runtime_retry"');
});

test("explains release-age policy failures in Office chat", async ({
  testPage,
  apiClient,
  officeSeed,
}) => {
  const sentFrames = await interceptSessionRecovery(testPage);

  const task = await apiClient.createTask(officeSeed.workspaceId, "Office npm policy recovery", {
    workflow_id: officeSeed.workflowId,
  });
  await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
    assignee_agent_profile_id: officeSeed.agentId,
  });
  await apiClient.seedTaskSession(task.id, {
    state: "FAILED",
    agentProfileId: officeSeed.agentId,
    completedAt: "2026-08-16T09:15:44Z",
    metadata: {
      last_agent_error: {
        message: "managed npm runtime is blocked by release-age policy",
        code: "managed_runtime_npm_policy",
        details:
          "npm error notarget No matching version found. Minimum release age policy applies.\n  @example/agent@1.2.3 release date: <release-date>",
      },
    },
  });

  await testPage.goto(`/office/tasks/${task.id}`);
  await expect(testPage.getByRole("heading", { name: "Office npm policy recovery" })).toBeVisible();

  const recovery = testPage.getByTestId("run-error-managed-runtime-npm-recovery");
  await expect(recovery).toBeVisible();
  await expect(recovery).toContainText("npm blocked this runtime version");
  await expect(recovery).toContainText(
    "Check npm's min-release-age or before setting. Wait until this version is eligible or select an older version, then retry.",
  );
  await expect(recovery.getByTestId("run-error-managed-runtime-retry-button")).toHaveCount(1);
  await expect(recovery.getByTestId("run-error-resume-button")).toHaveCount(0);
  await expect(recovery.getByTestId("run-error-fresh-button")).toHaveCount(0);
  await expect(recovery.getByRole("button", { name: "Technical details" })).toHaveAttribute(
    "aria-expanded",
    "false",
  );

  await recovery.getByTestId("run-error-managed-runtime-retry-button").click();
  await expect
    .poll(() => sentFrames.find((frame) => frame.includes('"action":"session.recover"')) ?? "")
    .toContain('"action":"runtime_retry"');
});
