import { expect, test } from "../../fixtures/office-fixture";

test("mobile run observation keeps identity visible without document overflow", async ({
  testPage,
  apiClient,
  officeSeed,
}) => {
  const run = await apiClient.seedRun({
    agentProfileId: officeSeed.agentId,
    status: "finished",
    reason: "routine_dispatch_manual",
  });

  await testPage.goto(`/office/agents/${officeSeed.agentId}/runs/${run.run_id}`);
  await expect(testPage.getByTestId("run-header")).toBeVisible();
  await expect(testPage.getByTestId("run-agent-name")).toHaveText("CEO");
  expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
    await testPage.evaluate(() => document.documentElement.clientWidth),
  );
});
