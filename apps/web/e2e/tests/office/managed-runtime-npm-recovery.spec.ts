import { test, expect } from "../../fixtures/office-fixture";

test("keeps managed npm recovery visible in Office chat", async ({
  testPage,
  apiClient,
  officeSeed,
}) => {
  const sentFrames: string[] = [];
  testPage.on("websocket", (ws) => {
    if (!ws.url().endsWith("/ws")) return;
    ws.on("framesent", (event) => {
      if (typeof event.payload === "string") sentFrames.push(event.payload);
    });
  });

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
