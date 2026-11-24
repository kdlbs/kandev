import { expect, type Page } from "@playwright/test";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { firstSessionId } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

export async function expectLiveJobSurvivesSnapshot(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<void> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Background snapshot ordering",
    seedData.agentProfileId,
    {
      description: "background snapshot ordering",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  const sessionId = await firstSessionId(apiClient, task.id);
  let snapshotStarted!: () => void;
  let releaseSnapshot!: () => void;
  const started = new Promise<void>((resolve) => (snapshotStarted = resolve));
  const released = new Promise<void>((resolve) => (releaseSnapshot = resolve));
  await page.route(`**/api/v1/task-sessions/${sessionId}/background-work`, async (route) => {
    const response = await route.fetch();
    expect((await response.json()).workloads).toEqual([]);
    snapshotStarted();
    await released;
    await route.fulfill({ response });
  });
  const ws = watchWs(page);
  try {
    await page.goto(`/t/${task.id}`);
    await new SessionPage(page).waitForLoad();
    await started;
    const update = ws.waitForEvent("session.background_work.updated");
    await apiClient.recordBackgroundWorkload(sessionId, {
      work_id: "snapshot-race-job",
      run_id: "snapshot-race-run",
      kind: "shell",
      title: "Live background job",
      state: "running",
      revision: 1,
    });
    await update;
    const chip = page.getByTestId("background-work-chip");
    await expect(chip).toBeVisible();
    await expect(chip).toContainText("Background Job");

    releaseSnapshot();
    // The store bridge observes the production HTTP publication settling, so
    // the assertion cannot pass before the stale response has been applied.
    await page.waitForFunction((id) => {
      const store = (window as Window & { __KANDEV_E2E_STORE__: StoreApi<AppState> })
        .__KANDEV_E2E_STORE__;
      return store.getState().backgroundWork.loadingBySessionId[id] === false;
    }, sessionId);
    await expect(chip).toBeVisible();
    await expect(chip).toContainText("Background Job");
  } finally {
    releaseSnapshot();
  }
}
