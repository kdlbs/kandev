// AC-COORDINATOR-SHADOW-DREAM-005.x / -006.x: the Learning section shows the
// shadow switch, health, measures and reports; a report opens to its items and
// a rating is saved. Dream rows are inserted straight into the e2e database
// because a dream cannot be created over HTTP.
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { linkToCoordinatorSettings } from "../../../lib/coordinator/links";
import { DREAM_ID, ITEM_ID, ITEM_TEXT, seedDream } from "./learning-fixture";

test.describe("Coordinator Learning section", () => {
  test("lists reports, opens one and rates an item", async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const release = await backend.useEnv({ KANDEV_FEATURES_COORDINATOR_PHASE31: "true" });
    try {
      const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
        name: "Learning Coordinator",
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        task_agent_profile_id: seedData.agentProfileId,
        task_executor_profile_id: seedData.worktreeExecutorProfileId,
      });
      seedDream(backend.tmpDir, coordinator.id);

      await testPage.goto(
        `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=learning`,
      );
      await expect(testPage.getByTestId("learning-section")).toBeVisible();
      await expect(testPage.getByTestId("learning-health")).toBeVisible();
      await expect(testPage.getByTestId("learning-measure-approval_without_edit")).toBeVisible();
      await expect(testPage.getByTestId(`learning-report-${DREAM_ID}`)).toBeVisible();

      await testPage.getByTestId(`learning-report-open-${DREAM_ID}`).click();
      await expect(testPage.getByTestId("learning-detail")).toBeVisible();
      await expect(testPage.getByTestId(`learning-item-${ITEM_ID}`)).toContainText(ITEM_TEXT);
      await expect(testPage.getByTestId("learning-considered")).toContainText("Raise the ceiling");

      const put = waitForHttp(testPage, "PUT", /\/dreams\/[^/]+\/items\/[^/]+\/rating$/);
      await testPage.getByTestId(`learning-rate-${ITEM_ID}-useful`).click();
      expect((await put).status()).toBe(204);
      await expect(testPage.getByTestId(`learning-rate-${ITEM_ID}-useful`)).toHaveAttribute(
        "aria-checked",
        "true",
      );

      await testPage.getByTestId("learning-detail-back").click();
      await expect(testPage.getByTestId(`learning-report-${DREAM_ID}`)).toBeVisible();
    } finally {
      await release();
    }
  });
});
