import { test, expect } from "../../fixtures/office-fixture";

test.describe("Activity page", () => {
  test("activity page renders", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/workspace/activity");
    await expect(testPage.getByRole("heading", { name: /Activity/i }).first()).toBeVisible({
      timeout: 10_000,
    });
  });

  test("activity API returns data", async ({ officeApi, officeSeed }) => {
    const result = await officeApi.listActivity(officeSeed.workspaceId);
    expect(result).toBeDefined();
  });
});
