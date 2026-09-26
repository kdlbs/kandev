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

  test("uses the shared profile picker and keeps route help reachable", async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING: "true",
    });

    try {
      await expect
        .poll(
          async () => {
            const { agents } = await apiClient.listAgents();
            return agents.some((agent) =>
              agent.profiles?.some((profile) => profile.id === seedData.agentProfileId),
            );
          },
          { timeout: 15_000 },
        )
        .toBe(true);
      const { agents } = await apiClient.listAgents();
      const candidate = agents
        .flatMap((agent) => agent.profiles ?? [])
        .find((profile) => profile.id === seedData.agentProfileId);
      if (!candidate) throw new Error("The E2E fixture must provide a concrete global profile");

      await testPage.goto("/settings/agents");
      await testPage.getByTestId("new-dynamic-profile").tap();
      await expect(testPage).toHaveURL(/\/settings\/agents\/dynamic\?mode=create$/);
      await expect(testPage.getByTestId("dynamic-profile-enabled-toggle")).toHaveCount(0);
      await expect(testPage.getByTestId("dynamic-profile-name")).toBeVisible({ timeout: 15_000 });
      await expect(testPage.getByTestId("dynamic-routing-policy-help")).toBeVisible();
      await expect(testPage.getByTestId("dynamic-routing-policy-help")).toContainText(
        "authentication or subscription problems, missing credentials or configuration, quota or rate limits",
      );

      const picker = testPage.getByTestId("add-dynamic-candidate");
      await picker.tap();
      const dropdown = testPage.getByTestId("add-dynamic-candidate-dropdown");
      await expect(dropdown).toBeVisible();
      await dropdown.getByPlaceholder("Search agent profiles...").fill(candidate.agentDisplayName);
      await expect(dropdown.getByTestId("agent-profile-picker-agent-icon").first()).toBeVisible();
      await dropdown.locator(`[data-value="${candidate.id}"]`).tap();

      await expect(testPage.getByTestId("dynamic-profile-candidates").locator("li")).toContainText(
        candidate.name,
      );
      await expect(testPage.getByTestId("dynamic-policy-transient")).toBeVisible();
      await expect(testPage.getByTestId("dynamic-policy-hard")).toBeVisible();
      await testPage
        .getByTestId("dynamic-policy-transient")
        .getByTestId("dynamic-policy-option-help-outcome")
        .tap();
      await expect(testPage.getByRole("tooltip")).toContainText(
        "after reset waiting and all same-candidate retries are exhausted",
      );
      await expect
        .poll(
          async () =>
            testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
          { timeout: 10_000 },
        )
        .toBe(true);
    } finally {
      await releaseFeature();
    }
  });
});
