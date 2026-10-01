// The Projects part of Watches on a phone viewport (matched by the
// `mobile-chrome` project via the mobile- filename prefix).
import { test, expect } from "../../fixtures/test-base";
import { linkToCoordinatorSettings } from "../../../lib/coordinator/links";
import { PHASE31_ENV, createLocalRepository } from "./watch-projects-fixture";

const MIN_TOUCH_TARGET_PX = 44;

test.describe("Coordinator watch projects on a phone viewport", () => {
  test("project rows are touch sized, full width and do not overflow the page", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const release = await backend.useEnv(PHASE31_ENV);
    try {
      const other = await createLocalRepository(
        apiClient,
        backend.tmpDir,
        seedData.workspaceId,
        "Phone Elsewhere",
      );
      const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
        name: "Mobile Project Watcher",
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
      });

      await testPage.goto(
        `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=watches`,
      );
      await testPage.getByTestId("watches-projects-all").click();
      const toggle = testPage.getByTestId(`watches-project-toggle-${other.id}`);
      await expect(toggle).toBeVisible();
      const box = await toggle.boundingBox();
      const viewport = testPage.viewportSize();
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
      expect(box?.width ?? 0).toBeGreaterThan((viewport?.width ?? 0) * 0.6);

      const overflow = await testPage.evaluate(() => ({
        scrollWidth: document.documentElement.scrollWidth,
        clientWidth: document.documentElement.clientWidth,
      }));
      expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);
    } finally {
      await release();
    }
  });
});
