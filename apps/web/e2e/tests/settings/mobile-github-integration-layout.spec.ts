import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile GitHub integration settings", () => {
  test("keeps the enable toggle in the compact integration header", async ({ testPage }) => {
    await testPage.goto("/settings/integrations/github");

    const heading = testPage.locator("h3[data-testid='github-integration-heading']");
    await expect(heading).toBeVisible();
    await expect(
      testPage.locator("section").filter({ has: heading }).locator("#github-enabled"),
    ).toBeVisible();
    const accessCard = testPage.getByTestId("github-workspace-access-card");
    await expect(accessCard).toBeVisible();
    await expect(accessCard).toHaveAttribute("data-slot", "card");
    await expect(testPage.locator("#github-enabled")).toHaveAttribute("aria-checked", "true");
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  });
});
