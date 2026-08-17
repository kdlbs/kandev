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
      await expect(testPage.getByTestId("dynamic-profile-enabled-toggle")).toHaveCount(0);
    } finally {
      await releaseFeature();
    }
  });

  test("uses the shared searchable picker and explains route actions", async ({
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
      await testPage.getByTestId("new-dynamic-profile").click();
      await expect(testPage).toHaveURL(/\/settings\/agents\/dynamic\?mode=create$/);
      await expect(testPage.getByTestId("dynamic-profile-name")).toBeVisible({ timeout: 15_000 });
      await expect(testPage.getByTestId("dynamic-routing-policy-help")).toBeVisible();
      await expect(testPage.getByTestId("dynamic-routing-policy-help")).toContainText(
        "authentication or subscription problems, missing credentials or configuration, quota or rate limits",
      );

      const picker = testPage.getByTestId("add-dynamic-candidate");
      await picker.click();
      const dropdown = testPage.getByTestId("add-dynamic-candidate-dropdown");
      await expect(dropdown).toBeVisible();
      await expect(dropdown.getByTestId("agent-profile-picker-agent-icon").first()).toBeVisible();
      await dropdown.getByPlaceholder("Search agent profiles...").fill(candidate.agentDisplayName);
      await expect(dropdown.locator(`[data-value="${candidate.id}"]`)).toBeVisible();
      await dropdown.locator(`[data-value="${candidate.id}"]`).click();

      const candidateRow = testPage.getByTestId("dynamic-profile-candidates").locator("li");
      await expect(candidateRow).toContainText(candidate.name);
      await expect(testPage.getByTestId("dynamic-policy-transient")).toBeVisible();
      await expect(testPage.getByTestId("dynamic-policy-hard")).toBeVisible();
      const help = testPage
        .getByTestId("dynamic-policy-transient")
        .getByTestId("dynamic-policy-option-help-outcome");
      await help.hover();
      await expect(testPage.getByRole("tooltip")).toContainText(
        "after reset waiting and all same-candidate retries are exhausted",
      );
    } finally {
      await releaseFeature();
    }
  });
});
