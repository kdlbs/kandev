// REQ-COORDINATOR-PAUSE-001 and -003: Pause and Resume on the autonomy strip
// and in the Autonomy settings section, against the real backend, with the
// phase 3.1 flag switched on for the spec and off again for the badge case.
import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import {
  linkToCoordinatorAutonomySettings,
  linkToCoordinatorNeedsYou,
} from "../../../lib/coordinator/links";

const PHASE31 = { KANDEV_FEATURES_COORDINATOR_PHASE31: "true" };
const PAUSE_PUT = /\/coordinators\/[^/]+\/pause$/;
const STRIP_STATE = "autonomy-strip-state";
const PAUSE_BUTTON = "autonomy-pause";
const RESUME_BUTTON = "autonomy-resume";

type Seed = { workspaceId: string; agentProfileId: string; worktreeExecutorProfileId: string };

async function autonomousCoordinator(
  apiClient: import("../../helpers/api-client").ApiClient,
  seed: Seed,
  name: string,
  autonomy = true,
) {
  const coordinator = await apiClient.createCoordinator(seed.workspaceId, {
    name,
    agent_profile_id: seed.agentProfileId,
    executor_profile_id: seed.worktreeExecutorProfileId,
    task_agent_profile_id: seed.agentProfileId,
    task_executor_profile_id: seed.worktreeExecutorProfileId,
  });
  if (autonomy) {
    const patched = await apiClient.rawRequest(
      "PATCH",
      `/api/v1/workspaces/${seed.workspaceId}/coordinators/${coordinator.id}`,
      { autonomy_enabled: true, cost_ceiling_usd: "10.00" },
    );
    expect(patched.status).toBe(200);
  }
  return coordinator;
}

async function click(page: Page, testId: string): Promise<void> {
  const put = waitForHttp(page, "PUT", PAUSE_PUT);
  await page.getByTestId(testId).click();
  expect((await put).status()).toBe(200);
}

test.describe("Coordinator pause on desktop", () => {
  test("Pause and Resume from the strip update the strip and the stored state", async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const release = await backend.useEnv(PHASE31);
    try {
      const coordinator = await autonomousCoordinator(apiClient, seedData, "Pause Strip");
      await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
      await expect(testPage.getByTestId(PAUSE_BUTTON)).toBeVisible();

      await click(testPage, PAUSE_BUTTON);
      await expect(testPage.getByTestId(STRIP_STATE)).toHaveAttribute("data-state", "paused");
      await expect(testPage.getByTestId(STRIP_STATE)).toContainText("Autonomy: Paused");
      await expect(testPage.getByTestId(RESUME_BUTTON)).toBeVisible();

      const stored = await apiClient.rawRequest(
        "GET",
        `/api/v1/workspaces/${seedData.workspaceId}/coordinators/${coordinator.id}/autonomy`,
      );
      expect(((await stored.json()) as { paused: boolean }).paused).toBe(true);

      await click(testPage, RESUME_BUTTON);
      await expect(testPage.getByTestId(STRIP_STATE)).not.toHaveAttribute("data-state", "paused");
      await expect(testPage.getByTestId(PAUSE_BUTTON)).toBeVisible();
    } finally {
      await release();
    }
  });

  test("the Autonomy section shows the state and the control while autonomy is off", async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const release = await backend.useEnv(PHASE31);
    try {
      const coordinator = await autonomousCoordinator(apiClient, seedData, "Pause Section", false);
      await testPage.goto(linkToCoordinatorAutonomySettings(seedData.workspaceId, coordinator.id));
      const section = testPage.getByTestId("autonomy-pause-block");
      await expect(section.getByTestId("autonomy-pause-state")).toHaveText("Not paused");
      await expect(section).toContainText("A paused coordinator keeps its queue");

      await click(testPage, "autonomy-section-pause");
      await expect(section.getByTestId("autonomy-pause-state")).toContainText("Paused since");
      await expect(testPage.getByTestId("autonomy-section-resume")).toBeVisible();
    } finally {
      await release();
    }
  });

  test("with the flag off a paused coordinator shows a read-only badge and no control", async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const release = await backend.useEnv(PHASE31);
    let released = false;
    try {
      const coordinator = await autonomousCoordinator(apiClient, seedData, "Pause Flag Off");
      await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
      await click(testPage, PAUSE_BUTTON);
      await expect(testPage.getByTestId(STRIP_STATE)).toHaveAttribute("data-state", "paused");

      await release();
      released = true;
      await testPage.reload();
      await expect(testPage.getByTestId(STRIP_STATE)).toHaveAttribute("data-state", "paused");
      await expect(testPage.getByTestId("autonomy-paused-flag-off-note")).toHaveText(
        "Resume needs the phase 3.1 features to be on",
      );
      await expect(testPage.getByTestId(RESUME_BUTTON)).toHaveCount(0);
      await expect(testPage.getByTestId(PAUSE_BUTTON)).toHaveCount(0);
    } finally {
      if (!released) await release();
    }
  });
});
