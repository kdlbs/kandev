// The Automatic eligibility list and the raise on a phone viewport (matched by
// the `mobile-chrome` project via the mobile- filename prefix).
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { linkToCoordinatorSettings } from "../../../lib/coordinator/links";

const SETTINGS_PATH = /\/coordinators\/[^/]+\/settings$/;
const REVIEW_PATH = /\/coordinators\/[^/]+\/classes\/create_task\/reviews$/;
const MIN_TOUCH_TARGET_PX = 44;

test.describe("Coordinator automatic create_task on a phone viewport", () => {
  test("the eligibility list fits, review unlocks the raise and the raise saves", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Mobile Automatic Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      task_agent_profile_id: seedData.agentProfileId,
      task_executor_profile_id: seedData.worktreeExecutorProfileId,
    });
    const seeded = await apiClient.rawRequest(
      "POST",
      `/api/v1/_test/coordinators/${coordinator.id}/seed-create-task-history`,
      { window_rows: 20, edited_rows: 0, oldest_days_ago: 31 },
    );
    expect(seeded.status).toBe(201);
    const settings = `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=may-do`;

    await testPage.goto(settings);
    await expect(testPage.getByTestId("automatic-condition-volume")).toHaveAttribute(
      "data-met",
      "true",
    );
    const overflow = await testPage.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);
    await expect(testPage.locator("#may-do-create_task-automatic")).toBeDisabled();

    await testPage.getByTestId("may-do-review-create_task").click();
    await expect(testPage).toHaveURL(/\/queue\?class=create_task$/);
    await testPage.goto(settings);
    const mark = testPage.getByTestId("automatic-mark-reviewed");
    const box = await mark.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
    const reviewed = waitForHttp(testPage, "POST", REVIEW_PATH);
    await mark.click();
    await reviewed;

    await testPage.locator("#may-do-create_task-automatic").click();
    const saved = waitForHttp(testPage, "PUT", SETTINGS_PATH);
    await testPage.getByRole("button", { name: "Save changes" }).click();
    await saved;
    await expect(testPage.getByTestId("automatic-raised-record")).toContainText(
      "Raised to automatic",
    );
  });
});
