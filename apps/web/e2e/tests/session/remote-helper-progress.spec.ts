import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

async function seedRemoteHelperProgress(
  apiClient: ApiClient,
  seedData: SeedData,
  status: "running" | "failed",
) {
  const task = await apiClient.createTask(seedData.workspaceId, `Remote helper ${status}`, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const failed = status === "failed";
  await apiClient.seedTaskSession(task.id, {
    state: failed ? "FAILED" : "RUNNING",
    startedAt: "2026-09-25T10:00:00Z",
    completedAt: failed ? "2026-09-25T10:01:30Z" : undefined,
    metadata: {
      prepare_result: {
        status: failed ? "failed" : "preparing",
        steps: [
          {
            name: "",
            kind: "remote_helper_download",
            remote_platform: "linux/amd64",
            failure_code: failed ? "timeout" : undefined,
            status,
            error: failed ? "context deadline exceeded" : undefined,
          },
        ],
      },
    },
  });
  return task;
}

test("shows typed remote helper download progress on desktop", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await seedRemoteHelperProgress(apiClient, seedData, "running");
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  const panel = session.activeChat().getByTestId("prepare-progress-panel");
  await expect(panel).toBeVisible();
  await expect(panel).toContainText("Downloading remote helper (linux/amd64)");
});

test("shows localized helper timeout recovery on desktop", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await seedRemoteHelperProgress(apiClient, seedData, "failed");
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  const panel = session.activeChat().getByTestId("prepare-progress-panel");
  await expect(panel).toBeVisible();
  await panel.getByRole("button", { name: "Show preparation details" }).click();
  await expect(panel).toContainText("Remote helper download timed out (linux/amd64)");
  await expect(panel).toContainText(
    "Use the full offline runtime archive or configure an explicit helper path.",
  );
  await expect(panel).toContainText("context deadline exceeded");
});
