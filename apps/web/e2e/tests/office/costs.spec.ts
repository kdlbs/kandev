import { test, expect } from "../../fixtures/office-fixture";
import { officeTopbarTitle } from "../../helpers/office-topbar";

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
    await expect(officeTopbarTitle(testPage)).toHaveText(/Costs/i, {
      timeout: 10_000,
    });
  });
});
