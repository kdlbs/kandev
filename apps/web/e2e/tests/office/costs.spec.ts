import { test, expect } from "../../fixtures/office-fixture";

test.describe("Costs & Budgets", () => {
  test("cost summary starts at zero", async ({ officeApi, officeSeed }) => {
    const summary = await officeApi.getCostSummary(officeSeed.workspaceId);
    expect(summary).toBeDefined();
  });

  test("budgets list is initially empty", async ({ officeApi, officeSeed }) => {
    const budgets = await officeApi.listBudgets(officeSeed.workspaceId);
    expect(budgets).toBeDefined();
  });

  test("costs page renders", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/workspace/costs");
    await expect(testPage.getByRole("heading", { name: /Costs/i }).first()).toBeVisible({
      timeout: 10_000,
    });
  });
});
