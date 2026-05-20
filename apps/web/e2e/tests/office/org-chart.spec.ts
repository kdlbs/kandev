import { test, expect } from "../../fixtures/office-fixture";

test.describe("Org chart", () => {
  test("org chart shows CEO agent node", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/workspace/org");
    await expect(testPage.getByRole("heading", { name: /Org/i }).first()).toBeVisible({
      timeout: 10_000,
    });
    // CEO agent from onboarding should appear as a node
    await expect(testPage.getByText("CEO").first()).toBeVisible({ timeout: 15_000 });
  });
});
