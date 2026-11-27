// AC-COORDINATOR-TASK-AGENT on a phone-sized viewport (mobile-chrome project).
import { expect, test } from "../../fixtures/test-base";
import { enableCoordinatorFeature, seedCoordinator } from "./coordinator-fixture";

test.describe("Coordinator agent for created tasks (mobile)", () => {
  test("the editor shows the pair without horizontal scroll", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);
    const release = await enableCoordinatorFeature(backend, apiClient, seedData.workspaceId);
    try {
      const coordinator = await seedCoordinator(apiClient, seedData.workspaceId, {
        name: "Phone Task Pair",
        agentProfileId: seedData.agentProfileId,
        executorProfileId: seedData.worktreeExecutorProfileId,
      });
      await testPage.goto(
        `/settings/workspaces/${seedData.workspaceId}/coordinators/${coordinator.id}`,
      );
      const pair = testPage.getByTestId("coordinator-task-pair");
      await expect(pair).toBeVisible({ timeout: 15_000 });
      await expect(pair).toContainText("Agent for created tasks");
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
