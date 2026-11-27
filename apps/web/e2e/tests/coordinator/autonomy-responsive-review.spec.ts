import { test, expect } from "../../fixtures/test-base";
import { linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";
import { stubAutonomyRead } from "./autonomy-fixture";

test("narrow fine-pointer autonomy action fills its footer with a 44px target", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.setViewportSize({ width: 700, height: 900 });
  expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(false);
  const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
    name: "Narrow Fine Pointer Autonomy",
    agent_profile_id: seedData.agentProfileId,
    executor_profile_id: seedData.worktreeExecutorProfileId,
    task_agent_profile_id: seedData.agentProfileId,
    task_executor_profile_id: seedData.worktreeExecutorProfileId,
  });
  await stubAutonomyRead(testPage, coordinator.id);
  await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
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
  const configure = testPage.getByRole("link", { name: "Configure", exact: true });
  await expect(configure).toBeVisible();
  const phoneConfigure = await configure.boundingBox();
  expect(phoneConfigure).not.toBeNull();
  expect(phoneConfigure!.height).toBeGreaterThanOrEqual(44);
  expect(phoneConfigure!.width).toBeGreaterThanOrEqual(44);
  await testPage.setViewportSize({ width: 1280, height: 900 });
  await expect.poll(async () => (await configure.boundingBox())?.height).toBeLessThan(44);
});
