import { expect, test } from "../../fixtures/test-base";

test.describe("Repository branch policies on mobile", () => {
  test("uses a drawer to create a policy without page overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
    const repositoryCard = testPage.locator('[data-slot="card"]', { hasText: "E2E Repo" });
    await repositoryCard.getByRole("button", { name: "Edit", exact: true }).tap();

    const policies = testPage.getByTestId(`branch-policies-${seedData.repositoryId}`);
    await policies.locator("summary").tap();
    await policies.getByRole("button", { name: "Add policy", exact: true }).tap();

    const drawer = testPage.getByRole("dialog", { name: "Add branch policy" });
    await expect(drawer).toBeVisible();
    await drawer.getByRole("button", { name: "About policy names", exact: true }).tap();
    await expect(testPage.getByRole("dialog", { name: "About policy names" })).toContainText(
      "Use a short name that helps people choose this policy in the task dialog.",
    );
    await testPage.keyboard.press("Escape");
    await drawer.getByRole("textbox", { name: "Policy name" }).fill("Mobile policy");
    await drawer.getByRole("combobox", { name: "Base branch" }).tap();
    await drawer.getByRole("option", { name: /^main local/ }).tap();
    await drawer.getByRole("combobox", { name: "Pull request target" }).tap();
    await drawer.getByRole("option", { name: /^main local/ }).tap();
    await drawer
      .getByRole("textbox", { name: "Branch name template" })
      .fill("mobile/{title}-{suffix}");
    await drawer.getByRole("button", { name: "Save", exact: true }).tap();

    await expect
      .poll(async () => {
        const response = await apiClient.listRepositoryBranchPolicies(seedData.repositoryId);
        return response.repository_branch_policies.some(
          (policy) => policy.name === "Mobile policy",
        );
      })
      .toBe(true);
    await expect(testPage.getByRole("dialog", { name: "Add branch policy" })).toHaveCount(0);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
      390,
    );
  });
});
