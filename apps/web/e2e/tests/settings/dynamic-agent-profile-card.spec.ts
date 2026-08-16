import { test, expect } from "../../fixtures/test-base";

test.describe("Dynamic Agents settings card", () => {
  test("renders first and opens dynamic profile creation", async ({ testPage, backend }) => {
    test.setTimeout(60_000);
    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING: "true",
    });

    try {
      await testPage.goto("/settings/agents");

      const agentCards = testPage.locator(
        '[data-testid="dynamic-agents-card"], [data-testid^="agent-group-"]',
      );
      await expect(agentCards.first()).toHaveAttribute("data-testid", "dynamic-agents-card");

      const createProfile = testPage.getByTestId("new-dynamic-profile");
      await expect(createProfile).toBeVisible({ timeout: 15_000 });
      await expect(createProfile).toBeEnabled();
      await expect(createProfile).toHaveAttribute("href", "/settings/agents/dynamic?mode=create");

      await createProfile.click();
      await expect(testPage).toHaveURL(/\/settings\/agents\/dynamic\?mode=create$/);
    } finally {
      await releaseFeature();
    }
  });
});
