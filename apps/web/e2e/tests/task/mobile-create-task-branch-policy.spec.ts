import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { expectPolicyOptionContentNotToOverlap } from "./create-task-branch-policy-helpers";

useRegularMode();

test.describe("Task branch policy selection on mobile", () => {
  test("keeps the policy marker and fresh-branch state visible", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const policy = await apiClient.createRepositoryBranchPolicy(seedData.repositoryId, {
      name: `Mobile policy ${Date.now()}`,
      base_branch: "main",
      branch_template: "mobile/{title}-{suffix}",
      pull_request_target: "main",
    });
    const { executors } = await apiClient.listExecutors();
    const localExecutor = executors.find((executor) => executor.type === "local");
    if (!localExecutor) {
      test.skip(true, "No local executor available");
      return;
    }
    const localProfile = await apiClient.createExecutorProfile(
      localExecutor.id,
      `E2E Mobile Branch Policy Local ${Date.now()}`,
    );

    try {
      await testPage.setViewportSize({ width: 390, height: 844 });
      const mobile = new MobileKanbanPage(testPage);
      await mobile.goto();
      await mobile.mobileFab.tap();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await dialog.getByTestId("executor-profile-selector").tap();
      await testPage.getByRole("option", { name: new RegExp(localProfile.name) }).tap();
      await dialog.getByTestId("branch-chip-trigger").tap();
      const option = testPage.getByRole("option", { name: new RegExp(policy.name) });
      await expect(option).toContainText("Policy");
      await expectPolicyOptionContentNotToOverlap(option, policy.name);
      await option.tap({ force: true });
      await expect(dialog.getByTestId("branch-chip-trigger")).toContainText(policy.name);
      await expect(dialog.getByTestId("fresh-branch-toggle")).toHaveAttribute(
        "aria-pressed",
        "true",
      );
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth),
      ).toBeLessThanOrEqual(390);
    } finally {
      await apiClient.deleteExecutorProfile(localProfile.id).catch(() => {});
    }
  });
});
