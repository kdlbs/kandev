import { test, expect } from "../../fixtures/test-base";

test.describe("Dynamic Agents settings card on mobile", () => {
  test("keeps the first-card creation path reachable by touch", async ({ testPage, backend }) => {
    test.setTimeout(60_000);
    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING: "true",
    });

    try {
      await testPage.goto("/settings/agents");

      await expect
        .poll(
          async () =>
            testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
          { timeout: 15_000 },
        )
        .toBe(true);

      const agentCards = testPage.locator(
        '[data-testid="dynamic-agents-card"], [data-testid^="agent-group-"]',
      );
      await expect(agentCards.first()).toHaveAttribute("data-testid", "dynamic-agents-card");

      const createProfile = testPage.getByTestId("new-dynamic-profile");
      await expect(createProfile).toBeVisible({ timeout: 15_000 });
      await expect
        .poll(
          async () => {
            const box = await createProfile.boundingBox();
            return box ? Math.min(box.width, box.height) : null;
          },
          { timeout: 10_000 },
        )
        .toBeGreaterThanOrEqual(44);

      await createProfile.tap();
      await expect(testPage).toHaveURL(/\/settings\/agents\/dynamic\?mode=create$/);
    } finally {
      await releaseFeature();
    }
  });
});
