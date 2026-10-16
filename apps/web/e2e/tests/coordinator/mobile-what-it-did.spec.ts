// The Queue's "What it did" section on a phone-sized viewport: rows stack, and
// the filter and the empty state stay reachable (the `mobile-chrome` project
// matches this filename).
import { test, expect } from "../../fixtures/test-base";
import { linkToCoordinatorQueue } from "../../../lib/coordinator/links";

test.describe("Coordinator What it did (mobile)", () => {
  test("the section, its filter and the filtered-empty state fit a phone viewport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Mobile Activity Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      task_agent_profile_id: seedData.agentProfileId,
      task_executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorQueue(seedData.workspaceId, coordinator.id));
    const section = testPage.getByTestId("what-it-did");
    await expect(section).toBeVisible();
    await expect(testPage.getByTestId("activity-empty")).toBeVisible();

    const box = await section.boundingBox();
    const viewport = testPage.viewportSize();
    if (!box || !viewport) throw new Error("expected a rendered section and a viewport");
    expect(box.x + box.width).toBeLessThanOrEqual(viewport.width);

    await section.getByTestId("activity-filter").click();
    await testPage.getByRole("option", { name: "Stop task" }).click();
    await expect(testPage).toHaveURL(/class=stop/);
    await expect(testPage.getByTestId("activity-empty")).toHaveText("Nothing matches this filter.");
  });
});
