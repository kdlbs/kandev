// AC-COORDINATOR-PAUSE-003.4: the Pause control on a phone is a full-width
// button with a touch target of at least 44 px on the strip's second line.
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";

const MIN_TOUCH_TARGET_PX = 44;

test.describe("Coordinator pause on a phone viewport", () => {
  test("the control is a full-width 44 px button and Pause works by touch", async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const release = await backend.useEnv({ KANDEV_FEATURES_COORDINATOR_PHASE31: "true" });
    try {
      const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
        name: "Mobile Pause",
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        task_agent_profile_id: seedData.agentProfileId,
        task_executor_profile_id: seedData.worktreeExecutorProfileId,
      });
      const patched = await apiClient.rawRequest(
        "PATCH",
        `/api/v1/workspaces/${seedData.workspaceId}/coordinators/${coordinator.id}`,
        { autonomy_enabled: true, cost_ceiling_usd: "10.00" },
      );
      expect(patched.status).toBe(200);

      await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
      const button = testPage.getByTestId("autonomy-pause");
      await expect(button).toBeVisible();
      const box = await button.boundingBox();
      const strip = await testPage.getByTestId("autonomy-strip").boundingBox();
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
      expect(box?.width ?? 0).toBeGreaterThan((strip?.width ?? 0) * 0.8);
      const overflow = await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      );
      expect(overflow).toBe(true);

      const put = waitForHttp(testPage, "PUT", /\/coordinators\/[^/]+\/pause$/);
      await button.tap();
      expect((await put).status()).toBe(200);
      await expect(testPage.getByTestId("autonomy-strip-state")).toHaveAttribute(
        "data-state",
        "paused",
      );
      await expect(testPage.getByTestId("autonomy-resume")).toBeVisible();
    } finally {
      await release();
    }
  });
});
