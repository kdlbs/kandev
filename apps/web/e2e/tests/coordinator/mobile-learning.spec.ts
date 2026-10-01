// AC-COORDINATOR-SHADOW-DREAM-005 on a phone: one column, 44 px touch targets,
// a full-screen report detail and no horizontal overflow.
import { test, expect } from "../../fixtures/test-base";
import { linkToCoordinatorSettings } from "../../../lib/coordinator/links";
import { DREAM_ID, ITEM_ID, seedDream } from "./learning-fixture";

const MIN_TOUCH_TARGET_PX = 44;

test.describe("Coordinator Learning section on a phone viewport", () => {
  test("targets are 44 px and nothing overflows horizontally", async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const release = await backend.useEnv({ KANDEV_FEATURES_COORDINATOR_PHASE31: "true" });
    try {
      const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
        name: "Mobile Learning",
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        task_agent_profile_id: seedData.agentProfileId,
        task_executor_profile_id: seedData.worktreeExecutorProfileId,
      });
      seedDream(backend.tmpDir, coordinator.id);
      await testPage.goto(
        `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=learning`,
      );
      const open = testPage.getByTestId(`learning-report-open-${DREAM_ID}`);
      await expect(open).toBeVisible();
      expect((await open.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
      await open.tap();
      await expect(testPage.getByTestId("learning-detail")).toBeVisible();
      const rate = testPage.getByTestId(`learning-rate-${ITEM_ID}-useful`);
      expect((await rate.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
      const fits = await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      );
      expect(fits).toBe(true);
    } finally {
      await release();
    }
  });
});
