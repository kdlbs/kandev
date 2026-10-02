// Sections row and the goal note on a phone viewport (matched by the
// `mobile-chrome` project via the mobile- filename prefix).
import { test, expect } from "../../fixtures/test-base";
import {
  linkToCoordinatorNeedsYou,
  linkToCoordinatorSettings,
} from "../../../lib/coordinator/links";

const MIN_TOUCH_TARGET_PX = 44;

test.describe("Coordinator sections on a phone viewport", () => {
  test("the Sections row scrolls in place, an order can be added and the note button is full width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Mobile Sections Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const action = testPage.getByTestId("goal-note-action");
    await expect(action).toBeVisible();
    const viewport = testPage.viewportSize();
    const box = await action.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
    expect(box?.width ?? 0).toBeGreaterThan((viewport?.width ?? 0) * 0.6);

    await testPage.goto(linkToCoordinatorSettings(seedData.workspaceId, coordinator.id));
    const row = testPage.getByRole("tablist", { name: "Coordinator sections" });
    await expect(row).toBeVisible();
    const overflow = await testPage.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);

    await row.getByRole("tab", { name: "Standing orders" }).click();
    await testPage.getByTestId("standing-order-add").click();
    await testPage.getByTestId("standing-order-text").fill("Keep cards small.");
    await testPage.getByTestId("standing-order-save").click();
    await expect(testPage.getByTestId("standing-order-row")).toContainText("Keep cards small.");
  });

  test("May do rows and the Watches list fit the phone viewport without horizontal scroll", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Mobile Control Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });
    const base = linkToCoordinatorSettings(seedData.workspaceId, coordinator.id);
    for (const section of ["may-do", "watches"]) {
      await testPage.goto(`${base}?section=${section}`);
      await expect(testPage.getByTestId(`coordinator-section-${section}`)).toBeVisible();
      const overflow = await testPage.evaluate(() => ({
        scrollWidth: document.documentElement.scrollWidth,
        clientWidth: document.documentElement.clientWidth,
      }));
      expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);
    }
  });
});
