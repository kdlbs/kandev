// Real phone browser geometry and navigation; autonomy reads are network-edge fixtures.
import { test, expect } from "../../fixtures/test-base";
import {
  linkToCoordinatorNeedsYou,
  linkToCoordinatorAutonomySettings,
} from "../../../lib/coordinator/links";
import { stubAutonomyRead } from "./autonomy-fixture";

test("phone autonomy actions span the card and coordinator selection stays tappable", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const coordinators = [];
  for (const name of ["Phone Autonomy A", "Phone Autonomy B"]) {
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      task_agent_profile_id: seedData.agentProfileId,
      task_executor_profile_id: seedData.worktreeExecutorProfileId,
    });
    coordinators.push(coordinator);
    await stubAutonomyRead(testPage, coordinator.id);
  }
  await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinators[0].id));
  const selector = testPage.getByTestId("coordinator-selector");
  await expect(selector).toBeVisible();
  const selectorBox = await selector.boundingBox();
  expect(selectorBox).not.toBeNull();
  expect(selectorBox!.height).toBeGreaterThanOrEqual(44);
  expect(selectorBox!.width).toBeGreaterThanOrEqual(44);
  await selector.tap();
  await testPage.getByRole("option", { name: "Phone Autonomy B", exact: true }).tap();
  await expect(testPage).toHaveURL(
    linkToCoordinatorNeedsYou(seedData.workspaceId, coordinators[1].id),
  );

  const action = testPage.getByTestId("autonomy-item-open-settings");
  await expect(action).toBeVisible();
  const geometry = await action.evaluate((element) => {
    const footer = element.parentElement!;
    const style = getComputedStyle(footer);
    return {
      height: element.getBoundingClientRect().height,
      width: element.getBoundingClientRect().width,
      available:
        footer.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight),
    };
  });
  expect(geometry.height).toBeGreaterThanOrEqual(44);
  expect(Math.abs(geometry.width - geometry.available)).toBeLessThanOrEqual(1);
  await action.tap();
  await expect(testPage).toHaveURL(
    linkToCoordinatorAutonomySettings(seedData.workspaceId, coordinators[1].id),
  );
  await expect(testPage.getByTestId("autonomy-section")).toBeVisible();
});
